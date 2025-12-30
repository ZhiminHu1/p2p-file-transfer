package centralserver

import (
	"fmt"
	"sync"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/transport"
	"tarun-kavipurapu/p2p-transfer/pkg/transport/tcp"
)

func init() {
}

type CentralServer struct {
	mu        sync.Mutex
	peers     map[string]transport.Node
	Transport transport.Transport
	files     map[string]*protocol.FileMetaData
	quitCh    chan struct{}
}

func NewCentralServer(optsStr string) *CentralServer {
	trans := tcp.NewTCPTransport(optsStr)

	centralServer := CentralServer{
		peers:     make(map[string]transport.Node),
		Transport: trans,
		files:     make(map[string]*protocol.FileMetaData),
		quitCh:    make(chan struct{}),
	}
	trans.SetOnPeer(centralServer.OnPeer)

	return &centralServer

}

func (c *CentralServer) Start() error {
	logger.Sugar.Infof("[PeerServer] [%s] starting CentralServer...", c.Transport.Addr())

	err := c.Transport.ListenAndAccept()
	if err != nil {
		return err
	}
	c.loop()
	return nil
}

func (c *CentralServer) loop() {

	defer func() {
		logger.Sugar.Info("[PeerServer] Central Serevr has stopped due to error or quit acction")

		c.Transport.Close()
	}()
	for {
		select {
		case msg := <-c.Transport.Consume():
			if err := c.handleMessage(msg.From, msg); err != nil {
				logger.Sugar.Error("handle message error:", err)
			}
		case <-c.quitCh:
			return
		}
	}
}

func (c *CentralServer) handleMessage(from string, msg protocol.RPC) error {
	switch v := msg.Payload.(type) {

	case protocol.FileMetaData:
		return c.handleRegisterFile(from, v)
	case protocol.RequestChunkData:
		return c.handleRequestChunkData(from, v)

	case protocol.RegisterSeeder:
		return c.registerSeeder(v)

	case protocol.DataMessage:
		logger.Sugar.Warn("[PeerServer] Received DataMessage wrapper, this shouldn't happen with new transport logic")
		return nil

	default:
		logger.Sugar.Errorf("[PeerServer] Unknown message type: %T", v)
	}
	return nil
}

func (c *CentralServer) registerSeeder(msg protocol.RegisterSeeder) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fileMetadata, exists := c.files[msg.FileId]
	if !exists {
		return fmt.Errorf("file with ID %s not found", msg.FileId)
	}

	// Correct loop:
	for k, v := range fileMetadata.ChunkInfo {
		peerExists := false
		for _, p := range v.Owners {
			if p == msg.PeerAddr {
				peerExists = true
				break
			}
		}
		if !peerExists {
			v.Owners = append(v.Owners, msg.PeerAddr)
			fileMetadata.ChunkInfo[k] = v // Update map
		}
	}

	logger.Sugar.Infof("Registered peer %s as a new seeder for file %s", msg.PeerAddr, msg.FileId)
	return nil
}
func (c *CentralServer) handleRequestChunkData(from string, msg protocol.RequestChunkData) error {
	fileId := msg.FileId
	c.mu.Lock()
	fileMetadata := c.files[fileId]
	peer := c.peers[from]
	c.mu.Unlock()

	if fileMetadata == nil {
		logger.Sugar.Errorf("[PeerServer] No file metadata found for file ID: %s", fileId)
		return fmt.Errorf("file metadata not found")
	}

	logger.Sugar.Info(msg)

	// Send protocol object
	logger.Sugar.Infof("[PeerServer] Sending metadata to peer %s", peer.Addr())

	err := peer.Send(*fileMetadata) // Send the struct directly
	if err != nil {
		logger.Sugar.Errorf("[PeerServer] Failed to send data to peer %s: %v", peer.Addr(), err)
		return err
	}

	logger.Sugar.Info("[PeerServer] Data sent successfully to peer", peer.Addr())
	return nil
}

func (c *CentralServer) handleRegisterFile(from string, msg protocol.FileMetaData) error {

	c.mu.Lock()
	defer c.mu.Unlock()
	// msg is a value.
	c.files[msg.FileId] = &msg
	logger.Sugar.Infof("[PeerServer] File ID: %s", msg.FileId)

	return nil
}

func (c *CentralServer) OnPeer(peer transport.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Always accept
	c.peers[peer.Addr()] = peer
	logger.Sugar.Infof("[PeerServer] listener central server peer connected with  %s", peer.Addr())

	return nil
}
