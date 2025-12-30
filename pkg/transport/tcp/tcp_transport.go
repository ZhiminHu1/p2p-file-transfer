package tcp

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"sync"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/transport"
)

// TCPNode implements transport.Node
type TCPNode struct {
	conn net.Conn
	enc  *gob.Encoder
	lock sync.Mutex
}

func NewTCPNode(conn net.Conn) *TCPNode {
	return &TCPNode{
		conn: conn,
		enc:  gob.NewEncoder(conn),
	}
}

func (n *TCPNode) Send(msg any) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	// Wrap in DataMessage as expected by current logic,
	// or directly send if the receiving side expects the raw struct.
	// Based on legacy logic: dataMsg := pkg.DataMessage{Payload: msg}
	// We will send the payload wrapped in DataMessage to maintain compatibility with the message loop logic
	// effectively, the Transport now handles the wrapping.

	wrapper := protocol.DataMessage{
		Payload: msg,
	}

	return n.enc.Encode(wrapper)
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
				fmt.Printf("TCP accept error: %s\n", err)
				continue
			}
		}
		node := NewTCPNode(conn)
		go t.handleConn(conn, node)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn, node transport.Node) {
	defer conn.Close()

	if t.onPeer != nil {
		if err := t.onPeer(node); err != nil {
			return
		}
	}

	dec := gob.NewDecoder(conn)

	for {
		var msg protocol.DataMessage
		if err := dec.Decode(&msg); err != nil {
			if err.Error() != "EOF" {
				log.Printf("TCP read error from %s: %s", conn.RemoteAddr(), err)
			}
			return
		}

		t.rpcCh <- protocol.RPC{
			From:    conn.RemoteAddr().String(),
			Payload: msg.Payload,
		}
	}
}

func (t *TCPTransport) Dial(addr string) (transport.Node, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	node := NewTCPNode(conn)
	go t.handleConn(conn, node)

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
