package centralserver

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"tarun-kavipurapu/p2p-transfer/pkg/discovery"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/transport"
	"tarun-kavipurapu/p2p-transfer/pkg/transport/tcp"
)

func init() {
}

type CentralServer struct {
	mu            sync.Mutex
	peersByListen map[string]*peerInfo // ListenAddr -> peerInfo
	peersByRemote map[string]*peerInfo // RemoteAddr -> peerInfo
	Transport     transport.Transport
	files         map[string]*protocol.FileMetaData
	quitCh        chan struct{}
	advertiser    *discovery.Advertiser
}
type peerInfo struct {
	Node       transport.Node
	RemoteAddr string
	ListenAddr string
	lastSeen   time.Time
	Priority   int
}

func NewCentralServer(addr string) *CentralServer {
	trans := tcp.NewTCPTransport(addr)

	centralServer := CentralServer{
		peersByListen: make(map[string]*peerInfo),
		peersByRemote: make(map[string]*peerInfo),
		Transport:     trans,
		files:         make(map[string]*protocol.FileMetaData),
		quitCh:        make(chan struct{}),
		advertiser:    discovery.NewAdvertiser(),
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

	// Start mDNS advertisement
	_, portStr, err := net.SplitHostPort(c.Transport.Addr())
	if err != nil {
		logger.Sugar.Errorf("[CentralServer] Failed to parse address: %v", err)
		return nil
	}
	port, err := strconv.Atoi(portStr)
	if err == nil && port > 0 {
		meta := map[string]string{
			"version": "1.0.0",
			"type":    "central-server",
		}
		if err := c.advertiser.Start("p2p-central", port, meta); err != nil {
			logger.Sugar.Errorf("[CentralServer] Failed to start mDNS advertisement: %v", err)
		} else {
			logger.Sugar.Infof("[CentralServer] mDNS advertisement started on port %d", port)
		}
	}

	go c.monitorPeers()
	c.loop()
	return nil
}

func (c *CentralServer) loop() {

	defer func() {
		logger.Sugar.Info("[CentralServer] stopped (error or quit)")

		c.Transport.Close()
	}()
	for {
		select {
		case msg := <-c.Transport.Consume():
			if err := c.handleMessage(msg.From, msg); err != nil {
				logger.Sugar.Errorf("[CentralServer] handle message failed: from=%s type=%T err=%v", msg.From, msg.Payload, err)
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
		// Heartbeat is frequent; keep it quiet unless debugging.
		c.mu.Lock()
		if pi, ok := c.peersByRemote[from]; ok {
			pi.lastSeen = time.Now()
		}
		c.mu.Unlock()
		return nil
	case protocol.PeerRegistration:
		return c.handlePeerRegister(from, v)

	default:
		logger.Sugar.Errorf("[CentralServer] Unknown message type from=%s type=%T", from, v)
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

	logger.Sugar.Infof("[CentralServer] registered seeder: peer=%s fileId=%s", msg.PeerAddr, msg.FileId)
	return nil
}
func (c *CentralServer) handleRequestChunkData(from string, msg protocol.RequestChunkData) error {
	fileId := msg.FileId
	c.mu.Lock()
	fileMetadata := c.files[fileId]
	peerByRemote := c.peersByRemote[from]
	if fileMetadata == nil {
		c.mu.Unlock()
		logger.Sugar.Errorf("[CentralServer] file metadata not found: fileId=%s from=%s", fileId, from)
		return fmt.Errorf("file metadata not found")
	}
	if peerByRemote == nil || peerByRemote.Node == nil {
		c.mu.Unlock()
		logger.Sugar.Errorf("[CentralServer] peer session not found: from=%s fileId=%s", from, fileId)
		return fmt.Errorf("peer session not found")
	}
	node := peerByRemote.Node

	//logger.Sugar.Info(msg)
	// 检查chunks的活跃owner
	// todo BGU待修复
	for chunkId, chunkInfo := range fileMetadata.ChunkInfo {
		var activeOwners []string
		for _, ownerAddr := range chunkInfo.Owners {
			// 检查owner是否在活跃列表中
			if _, exist := c.peersByListen[ownerAddr]; exist {
				activeOwners = append(activeOwners, ownerAddr)
			}
		}
		chunkInfo.Owners = activeOwners
		fileMetadata.ChunkInfo[chunkId] = chunkInfo
	}
	c.mu.Unlock()

	// Send protocol object
	logger.Sugar.Debugf("[CentralServer] sending metadata: to=%s from=%s fileId=%s", node.Addr(), from, fileId)

	err := node.Send(*fileMetadata) // Send the struct directly
	if err != nil {
		logger.Sugar.Errorf("[CentralServer] failed to send metadata: to=%s from=%s fileId=%s err=%v", node.Addr(), from, fileId, err)
		return err
	}

	logger.Sugar.Debugf("[CentralServer] metadata sent: to=%s fileId=%s", node.Addr(), fileId)
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
	status += fmt.Sprintf("Connected Peers: %d\n", len(c.peersByListen))
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
	for addr := range c.peersByListen {
		list = append(list, addr)
	}
	return list
}

func (c *CentralServer) Stop() {
	c.advertiser.Stop()
	close(c.quitCh)
	c.Transport.Close()
}

func (c *CentralServer) OnPeer(peer transport.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Always accept
	if _, exist := c.peersByRemote[peer.Addr()]; !exist {
		c.peersByRemote[peer.Addr()] = &peerInfo{
			Node:       peer,
			RemoteAddr: peer.Addr(),
			lastSeen:   time.Now(),
		}
	}
	logger.Sugar.Infof("[CentralServer] peer connected: remote=%s", peer.Addr())

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
			for remoteAddr, peer := range c.peersByRemote {
				if now.Sub(peer.lastSeen) > 15*time.Second {
					logger.Sugar.Warnf("[CentralServer] peer timed out: remote=%s listen=%s", remoteAddr, peer.ListenAddr)
					if peer.Node != nil {
						_ = peer.Node.Close()
					}
					delete(c.peersByRemote, remoteAddr)
					if _, exists := c.peersByListen[peer.ListenAddr]; exists {
						delete(c.peersByListen, peer.ListenAddr)
					}
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *CentralServer) handlePeerRegister(from string, v protocol.PeerRegistration) error {
	// 更新 peer的 listenAddr
	c.mu.Lock()
	defer c.mu.Unlock()

	pi, ok := c.peersByRemote[from]
	if !ok || pi == nil {
		logger.Sugar.Errorf("[CentralServer] No peer registered for %s", from)
		return fmt.Errorf("peer %s not found (remote)", from)
	}

	if v.ListenAddr != "" && pi.ListenAddr != v.ListenAddr {
		// 清理旧的 地址
		if cur := c.peersByListen[pi.ListenAddr]; cur == pi {
			delete(c.peersByListen, pi.ListenAddr)
		}
	}
	pi.ListenAddr = v.ListenAddr
	c.peersByListen[v.ListenAddr] = pi
	logger.Sugar.Infof("[CentralServer] peer registered: remote=%s listen=%s", from, v.ListenAddr)

	return nil
}
