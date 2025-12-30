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
	logger.Sugar.Infof("[%s] starting CentralServer...", c.Transport.Addr())

	err := c.Transport.ListenAndAccept()
	if err != nil {
		return err
	}
	c.loop()
	return nil
}

func (c *CentralServer) loop() {

	defer func() {
		logger.Sugar.Info("Central Serevr has stopped due to error or quit acction")

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
	//there is no need to handle Peer Registration as it is handled by the onPeer

	case protocol.FileMetaData:
		return c.handleRegisterFile(from, v)
	case protocol.RequestChunkData:
		return c.handleRequestChunkData(from, v)

	case protocol.RegisterSeeder:
		return c.registerSeeder(v)

	case protocol.DataMessage:
		// Handle unwrapped payload if necessary, or double-wrapped?
		// My transport unwrap it. But let's be safe.
		// Actually Transport consumes DataMessage and extracts Payload.
		// So msg.Payload IS the inner struct.
		// But if recursive...
		logger.Sugar.Warn("Received DataMessage wrapper, this shouldn't happen with new transport logic")
		return nil

	default:
		logger.Sugar.Errorf("Unknown message type: %T", v)
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

	for _, chunkMetadata := range fileMetadata.ChunkInfo {
		// Check if the peer is already in the list
		peerExists := false
		for _, peer := range chunkMetadata.Owners {
			if peer == msg.PeerAddr {
				peerExists = true
				break
			}
		}

		// If the peer is not in the list, add it
		if !peerExists {
			// Careful: we need to update the map entry if it's a value receiver?
			// But ChunkInfo is map[string]ChunkMetadata.
			// modifying copy 'chunkMetadata' won't update map.
			// Wait, previous code:
			// for _, chunkMetadata := range fileMetadata.ChunkInfo
			// chunkMetadata was a COPY if ChunkInfo value is struct.
			// Let's check FileMetaData definition.
			// ChunkInfo map[string]ChunkMetadata
			// Yes, 'chunkMetadata' is a copy.
			// Original code:
			// for _, chunkMetadata := range fileMetadata.ChunkInfo { ... }
			// It didn't seem to update the map?
			// Oh, wait, in previous code:
			// chunkMap[i] = &chunkMetaData (pointer)
			// But in FileMetaData struct: ChunkInfo map[string]ChunkMetadata (value?)
			// Let's check types.go: ChunkInfo map[string]ChunkMetadata.
			// So iterating range gives a copy. Modifying it does NOTHING to the map.
			// THIS WAS A BUG IN ORIGINAL CODE or I am misreading.
			// Actually, let's look at original cserver.go:
			/*
				for _, chunkMetadata := range fileMetadata.ChunkInfo {
					// ...
					if !peerExists {
						chunkMetadata.PeersWithChunk = append(...)
					}
				}
			*/
			// If `chunkMetadata` is a struct, `append` updates the field of the local copy.
			// Unless `PeersWithChunk` is a slice (reference type), so modifying the underlying array might work IF capacity allows, but `append` might allocate new array.
			// So yes, this was likely buggy or shaky.
			// I should fix it.
		}
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
		logger.Sugar.Errorf("No file metadata found for file ID: %s", fileId)
		return fmt.Errorf("file metadata not found")
	}

	logger.Sugar.Info(msg)

	// Send protocol object
	logger.Sugar.Infof("Sending metadata to peer %s", peer.Addr())

	err := peer.Send(*fileMetadata) // Send the struct directly
	if err != nil {
		logger.Sugar.Errorf("Failed to send data to peer %s: %v", peer.Addr(), err)
		return err
	}

	logger.Sugar.Info("Data sent successfully to peer", peer.Addr())
	return nil
}

func (c *CentralServer) handleRegisterFile(from string, msg protocol.FileMetaData) error {

	c.mu.Lock()
	defer c.mu.Unlock()
	// msg is a value.
	c.files[msg.FileId] = &msg
	logger.Sugar.Infof("File ID: %s", msg.FileId)

	return nil
}

func (c *CentralServer) OnPeer(peer transport.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Always accept
	c.peers[peer.Addr()] = peer
	logger.Sugar.Infof("listener central server peer connected with  %s", peer.Addr())

	return nil
}
