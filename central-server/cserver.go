package centralserver

import (
	"fmt"
	"sync"
	"time"

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
	lastSeen  map[string]time.Time
	Transport transport.Transport
	files     map[string]*protocol.FileMetaData
	quitCh    chan struct{}
}

func NewCentralServer(optsStr string) *CentralServer {
	trans := tcp.NewTCPTransport(optsStr)

	centralServer := CentralServer{
		peers:     make(map[string]transport.Node),
		lastSeen:  make(map[string]time.Time),
		Transport: trans,
		files:     make(map[string]*protocol.FileMetaData),
		quitCh:    make(chan struct{}),
	}
	trans.SetOnPeer(centralServer.OnPeer)

	return &centralServer

}

func (c *CentralServer) Start() error {
	logger.Sugar.Infof("[CentralServer] [%s] starting CentralServer...", c.Transport.Addr())

	err := c.Transport.ListenAndAccept()
	if err != nil {
		return err
	}
	go c.monitorPeers()
	c.loop()
	return nil
}

func (c *CentralServer) loop() {

	defer func() {
		logger.Sugar.Info("[CentralServer] Central Serevr has stopped due to error or quit acction")

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
		logger.Sugar.Warn("[CentralServer] Received DataMessage wrapper, this shouldn't happen with new transport logic")
		return nil

	case protocol.Heartbeat:
		logger.Sugar.Infof("[CentralServer] Received Heartbeat, from [%s]", from)
		c.mu.Lock()
		c.lastSeen[from] = time.Now()
		c.mu.Unlock()
		return nil

	default:
		logger.Sugar.Errorf("[CentralServer] Unknown message type: %T", v)
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

	if fileMetadata == nil {
		logger.Sugar.Errorf("[CentralServer] No file metadata found for file ID: %s", fileId)
		return fmt.Errorf("file metadata not found")
	}

	//logger.Sugar.Info(msg)
	// 检查chunks的活跃owner
	for chunkId, chunkInfo := range fileMetadata.ChunkInfo {
		var activeOwners []string
		for _, ownerAddr := range chunkInfo.Owners {
			// 检查owner是否在活跃列表中
			if _, exist := c.peers[ownerAddr]; exist {
				activeOwners = append(activeOwners, ownerAddr)
			}
		}
		chunkInfo.Owners = activeOwners
		fileMetadata.ChunkInfo[chunkId] = chunkInfo
	}
	c.mu.Unlock()

	// Send protocol object
	logger.Sugar.Infof("[CentralServer] Sending metadata to peer %s", peer.Addr())

	err := peer.Send(*fileMetadata) // Send the struct directly
	if err != nil {
		logger.Sugar.Errorf("[CentralServer] Failed to send data to peer %s: %v", peer.Addr(), err)
		return err
	}

	logger.Sugar.Info("[CentralServer] Data sent successfully to peer", peer.Addr())
	return nil
}

func (c *CentralServer) handleRegisterFile(from string, msg protocol.FileMetaData) error {

	c.mu.Lock()
	defer c.mu.Unlock()
	// msg is a value.
	c.files[msg.FileId] = &msg
	logger.Sugar.Infof("[CentralServer] File ID: %s", msg.FileId)

	return nil
}

func (c *CentralServer) GetStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	status := fmt.Sprintf("Central Server Running on: %s\n", c.Transport.Addr())
	status += fmt.Sprintf("Connected Peers: %d\n", len(c.peers))
	status += fmt.Sprintf("Registered Files: %d\n", len(c.files))

	for id, meta := range c.files {
		status += fmt.Sprintf(" - File: %s.%s (ID: %s) Size: %d bytes\n", meta.FileName, meta.FileExtension, id, meta.FileSize)
	}
	return status
}

func (c *CentralServer) GetPeersList() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var list []string
	for addr := range c.peers {
		list = append(list, addr)
	}
	return list
}

func (c *CentralServer) Stop() {
	close(c.quitCh)
	c.Transport.Close()
}

func (c *CentralServer) OnPeer(peer transport.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Always accept
	c.peers[peer.Addr()] = peer
	c.lastSeen[peer.Addr()] = time.Now()
	logger.Sugar.Infof("[CentralServer] listener central server peer connected with  %s", peer.Addr())

	return nil
}

func (c *CentralServer) monitorPeers() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.quitCh:
			return
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for addr, lastTime := range c.lastSeen {
				if now.Sub(lastTime) > 15*time.Second {
					logger.Sugar.Warnf("[CentralServer] Peer %s timed out, removing...", addr)
					if node, exists := c.peers[addr]; exists {
						node.Close()
						delete(c.peers, addr)
					}
					delete(c.lastSeen, addr)
					// 我们在别处实现掉线peer的文件清理逻辑
				}
			}
			c.mu.Unlock()
		}
	}
}
