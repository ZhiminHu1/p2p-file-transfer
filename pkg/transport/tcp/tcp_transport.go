package tcp

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"sync"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/transport"
)

// TCPNode implements transport.Node
type TCPNode struct {
	conn net.Conn
	lock sync.Mutex
	// TCP主动连接 outbound -> true 否则 outbound -> false
	outbound bool
}

func NewTCPNode(conn net.Conn, outbound bool) *TCPNode {
	return &TCPNode{
		conn:     conn,
		outbound: outbound,
	}
}

func (n *TCPNode) Send(msg any) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	// 1. Encode payload to memory buffer to know its size
	dataMessage := protocol.DataMessage{
		Incoming: protocol.IncomingMessageType,
		Msg:      msg,
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(dataMessage); err != nil {
		return err
	}
	payloadBytes := buf.Bytes()

	// 2. Write Header (Control Frame)
	if err := writeFrameHeader(n.conn, FrameTypeControl, uint32(len(payloadBytes))); err != nil {
		return err
	}

	// 3. Write Payload
	_, err := n.conn.Write(payloadBytes)
	return err
}

func (n *TCPNode) SendStream(meta any, data io.Reader, length int64) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	// Step 1: Send Metadata Wrapped in StreamMetaWrapper (Control Frame)
	// 发送一个 控制消息，代表准备进行流式传输数据
	dataMessage := protocol.DataMessage{Incoming: protocol.IncomingStreamType, Msg: meta}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(dataMessage); err != nil {
		return err
	}
	metaBytes := buf.Bytes()

	if err := writeFrameHeader(n.conn, FrameTypeControl, uint32(len(metaBytes))); err != nil {
		return fmt.Errorf("failed to write meta header: %w", err)
	}
	if _, err := n.conn.Write(metaBytes); err != nil {
		return fmt.Errorf("failed to write meta payload: %w", err)
	}

	// Step 2: Send Data Stream (Stream Frame)
	// Note: We use uint32 for length in header, so max stream size per frame is 4GB.
	// For larger streams, we might need a 64-bit length header or multiple frames.
	// Assuming chunk size < 4GB for now.
	if err := writeFrameHeader(n.conn, FrameTypeStream, uint32(length)); err != nil {
		return fmt.Errorf("failed to write stream header: %w", err)
	}

	// Step 3: Copy data directly to connection
	written, err := io.CopyN(n.conn, data, length)
	if err != nil {
		return fmt.Errorf("failed to write stream data: %w", err)
	}
	if written != length {
		return fmt.Errorf("stream write incomplete: expected %d, wrote %d", length, written)
	}

	return nil
}

func (n *TCPNode) Close() error {
	return n.conn.Close()
}

func (n *TCPNode) Addr() string {
	return n.conn.RemoteAddr().String()
}

// TCPTransport implements transport.Transport
type TCPTransport struct {
	listenAddr string
	listener   net.Listener
	rpcCh      chan protocol.RPC
	onPeer     func(transport.Node) error
}

func NewTCPTransport(addr string) *TCPTransport {
	return &TCPTransport{
		listenAddr: addr,
		rpcCh:      make(chan protocol.RPC, 1024),
	}
}

func (t *TCPTransport) SetOnPeer(f func(transport.Node) error) {
	t.onPeer = f
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error
	t.listener, err = net.Listen("tcp", t.listenAddr)
	if err != nil {
		return err
	}

	go t.acceptLoop()
	return nil
}

func (t *TCPTransport) acceptLoop() {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			// Check if closed
			select {
			case <-t.rpcCh: // just a check, not robust, but standard net error check is better
				return
			default:
				logger.Sugar.Errorf("[TCPTransport] accept error: listen=%s err=%v", t.listenAddr, err)
				continue
			}
		}
		node := NewTCPNode(conn, false)
		go t.handleConn(conn, node, node.outbound)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn, node transport.Node, outbound bool) {
	defer conn.Close()

	if !outbound && t.onPeer != nil {
		if err := t.onPeer(node); err != nil {
			return
		}
	}

	// No global decoder anymore, we decode per frame
	var pendingMeta any // Store metadata for the next stream

	for {
		// 1. Read Header
		msgType, length, err := readFrameHeader(conn)
		if err != nil {
			if err != io.EOF {
				logger.Sugar.Errorf("[TCPTransport] read header error: remote=%s err=%v", conn.RemoteAddr(), err)
			}
			return
		}

		if msgType == FrameTypeControl {
			// 2a. Control Frame: Read full payload into memory and decode
			payload := make([]byte, length)
			if _, err := io.ReadFull(conn, payload); err != nil {
				logger.Sugar.Errorf("[TCPTransport] read control payload error: %v", err)
				return
			}

			// Decode Gob
			var dataMessage protocol.DataMessage
			buf := bytes.NewReader(payload)
			if err := gob.NewDecoder(buf).Decode(&dataMessage); err != nil {
				logger.Sugar.Errorf("[TCPTransport] gob decode error: %v", err)
				continue
			}

			if dataMessage.Incoming == protocol.IncomingMessageType {
				// Standard message
				t.rpcCh <- protocol.RPC{
					From:    conn.RemoteAddr().String(),
					Payload: dataMessage.Msg,
				}
			} else if dataMessage.Incoming == protocol.IncomingStreamType {
				pendingMeta = dataMessage.Msg
				continue
				// 准备读取流式数据
			}

		} else if msgType == FrameTypeStream {
			// 2b. Stream Frame: Pass the reader to upper layer
			// We MUST block until the stream is consumed.
			streamReader := io.LimitReader(conn, int64(length))
			doneCh := make(chan struct{})

			// Use the pending metadata if available
			var meta any
			if pendingMeta != nil {
				meta = pendingMeta
				pendingMeta = nil // Reset
			}

			t.rpcCh <- protocol.RPC{
				From: conn.RemoteAddr().String(),
				Payload: protocol.IncomingStream{
					Stream: streamReader,
					Length: int64(length),
					Meta:   meta,
					Done:   doneCh,
				},
			}

			// BLOCK here until upper layer finishes reading
			<-doneCh
		} else {
			logger.Sugar.Errorf("[TCPTransport] unknown frame type: %d", msgType)
			return
		}
	}
}

func (t *TCPTransport) Dial(addr string) (transport.Node, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	node := NewTCPNode(conn, true)
	go t.handleConn(conn, node, node.outbound)

	return node, nil
}

func (t *TCPTransport) Consume() <-chan protocol.RPC {
	return t.rpcCh
}

func (t *TCPTransport) Close() error {
	close(t.rpcCh)
	return t.listener.Close()
}

func (t *TCPTransport) Addr() string {
	return t.listenAddr
}
