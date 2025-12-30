package transport

import "tarun-kavipurapu/p2p-transfer/pkg/protocol"

// Node represents a remote peer that we can send messages to
type Node interface {
	Send(msg any) error
	Close() error
	Addr() string
}

// Transport handles the network layer
type Transport interface {
	ListenAndAccept() error
	Dial(addr string) (Node, error)
	Consume() <-chan protocol.RPC
	Close() error
	Addr() string
	SetOnPeer(func(Node) error)
}
