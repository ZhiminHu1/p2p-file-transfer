package tcp

import (
	"encoding/gob"
	"net"
	"sync"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/transport"
)

// TCPNode implements transport.Node
type TCPNode struct {
	conn net.Conn
	enc  *gob.Encoder
	lock sync.Mutex
	// TCP主动连接 outbound -> true 否则 outbound -> false
	outbound bool
}

func NewTCPNode(conn net.Conn, outbound bool) *TCPNode {
	return &TCPNode{
		conn:     conn,
		enc:      gob.NewEncoder(conn),
		outbound: outbound,
	}
}

func (n *TCPNode) Send(msg any) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	return n.enc.Encode(msg)
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

	dec := gob.NewDecoder(conn)

	for {
		var msg any
		if err := dec.Decode(&msg); err != nil {
			if err.Error() != "EOF" {
				logger.Sugar.Errorf("[TCPTransport] read error: remote=%s outbound=%t err=%v", conn.RemoteAddr(), outbound, err)
			}
			return
		}
		t.rpcCh <- protocol.RPC{
			From:    conn.RemoteAddr().String(),
			Payload: msg,
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
