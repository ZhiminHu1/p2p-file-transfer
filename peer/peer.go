package peer

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "net/http/pprof" // Register pprof HTTP handlers

	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"tarun-kavipurapu/p2p-transfer/pkg/monitor"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/storage"
	"tarun-kavipurapu/p2p-transfer/pkg/transport"
	"tarun-kavipurapu/p2p-transfer/pkg/transport/tcp"
)

// PeerServer manages peer-to-peer file transfers
type PeerServer struct {
	peerLock           sync.RWMutex
	peers              map[string]transport.Node
	centralServerPeer  transport.Node
	fileMetadataInfo   map[string]*protocol.FileMetadata
	Transport          transport.Transport
	peerServerAddr     string
	centralServerAddr  string
	store              storage.Store
	completedDownloads map[string]bool
	downloadsMutex     sync.RWMutex

	pendingChunksLock sync.Mutex
	pendingChunks     map[string]chan protocol.ChunkMetaDataResponse
	quitCh            chan struct{}
}

func init() {
}

func NewPeerServer(addr string, centralServerAddr string) *PeerServer {
	trans := tcp.NewTCPTransport(addr)
	peerServer := &PeerServer{
		peers:              make(map[string]transport.Node),
		peerServerAddr:     addr,
		peerLock:           sync.RWMutex{},
		Transport:          trans,
		centralServerAddr:  centralServerAddr,
		store:              storage.Store{},
		fileMetadataInfo:   make(map[string]*protocol.FileMetadata),
		completedDownloads: make(map[string]bool),
		downloadsMutex:     sync.RWMutex{},
		pendingChunks:      make(map[string]chan protocol.ChunkMetaDataResponse),
		quitCh:             make(chan struct{}),
	}
	trans.SetOnPeer(peerServer.OnPeer)

	logger.Sugar.Infof("[PeerServer] initialized: listen=%s central=%s", peerServer.peerServerAddr, peerServer.centralServerAddr)
	return peerServer
}

func (p *PeerServer) messageLoop() {
	defer func() {
		logger.Sugar.Infof("[PeerServer] Stopping due to error or quit action")
		p.Transport.Close()
	}()
	logger.Sugar.Infof("[PeerServer] Starting main loop")
	for {
		select {
		case msg := <-p.Transport.Consume():
			if err := p.handleMessage(msg.From, msg); err != nil {
				logger.Sugar.Errorf("[PeerServer] Error handling message from %s: %v", msg.From, err)
			}
		case <-p.quitCh:
			return
		}
	}
}

func (p *PeerServer) handleMessage(from string, msg protocol.RPC) error {
	switch v := msg.Payload.(type) {
	case protocol.FileMetadata:
		logger.Sugar.Infof("[PeerServer] Received FileMetadata for file: %s", v.FileName)
		// Handle chunks in a separate goroutine to avoid blocking the message loop
		go func() {
			if err := p.handleRequestChunks(from, v); err != nil {
				logger.Sugar.Errorf("[PeerServer] Error handling request chunks: %v", err)
			}
		}()
		return nil

	case protocol.ChunkRequestToPeer:
		logger.Sugar.Infof("[PeerServer] Received ChunkRequestToPeer for file: %s, chunk: %d", v.FileId, v.ChunkId)
		return p.SendChunkData(from, v)
	case protocol.IncomingStream:
		// Handle incoming stream data
		defer close(v.Done) // Always signal completion

		if v.Meta == nil {
			io.Copy(io.Discard, v.Stream)
			logger.Sugar.Errorf("[PeerServer] Received incoming stream with no metadata")
			return fmt.Errorf("received incoming stream without metadata")
		}

		if meta, ok := v.Meta.(protocol.ChunkMetaDataResponse); ok {
			return p.handleChunkDataStream(meta, v.Stream)
		} else {
			io.Copy(io.Discard, v.Stream)
			logger.Sugar.Errorf("[PeerServer] Received incoming stream with unknown metadata type: %T", v.Meta)
			return fmt.Errorf("received incoming stream with unknown metadata type: %T", v.Meta)
		}
	default:
		return fmt.Errorf("unknown message type received from %s: %T", from, v)
	}
}

func (p *PeerServer) handleChunkDataStream(meta protocol.ChunkMetaDataResponse, stream io.Reader) error {
	logger.Sugar.Infof("[PeerServer] Receiving stream for chunk %d of file %s", meta.ChunkId, meta.FileId)

	// Start transfer timing
	monitor.StartTransfer()

	// 创建 chunk 目录并写入文件
	baseDir := fmt.Sprintf("chunks-%s", strings.Split(p.peerServerAddr, ":")[0])
	chunkDir := filepath.Join(baseDir, meta.FileId)
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		return fmt.Errorf("failed to create chunk directory: %w", err)
	}

	chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.chunk", meta.ChunkId))
	file, err := os.Create(chunkPath)
	if err != nil {
		return fmt.Errorf("failed to create chunk file: %w", err)
	}
	defer file.Close()

	// 流式写入（在这里完整消费 stream）
	written, err := io.Copy(file, stream)
	if err != nil {
		file.Close()         // 先关闭文件（Windows 需要先关闭才能删除）
		os.Remove(chunkPath) // 清理残留文件
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	// 记录传输完成
	monitor.RecordTransfer(written)

	logger.Sugar.Debugf("[PeerServer] Saved chunk %d (%d bytes)", meta.ChunkId, written)

	// 通知 logic 层：chunk 已保存
	key := fmt.Sprintf("%s:%d", meta.FileId, meta.ChunkId)
	p.pendingChunksLock.Lock()
	ch, exists := p.pendingChunks[key]
	if exists {
		delete(p.pendingChunks, key)
	}
	p.pendingChunksLock.Unlock()

	if exists {
		ch <- meta
	} else {
		logger.Sugar.Warnf("[PeerServer] Warning: Received chunk data for %s but no pending request found.", key)
	}
	return nil
}

func (p *PeerServer) SendChunkData(from string, v protocol.ChunkRequestToPeer) error {
	// Logic to read file and send data
	// Use relative path matching how chunks are created
	baseDir := fmt.Sprintf("chunks-%s", strings.ReplaceAll(p.peerServerAddr, ":", "-"))
	filePath := filepath.Join(baseDir, v.FileId, v.ChunkName)
	logger.Sugar.Infof("[PeerServer] Sending chunk %d of file %s to %s from path %s", v.ChunkId, v.FileId, from, filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open chunk file: %w", err)
	}
	defer file.Close()

	// Read entire file into byte slice
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	meta := protocol.ChunkMetaDataResponse{
		FileId:  v.FileId,
		ChunkId: v.ChunkId,
	}

	p.peerLock.Lock()
	peer, ok := p.peers[from]
	p.peerLock.Unlock()

	if !ok {
		return fmt.Errorf("peer %s not found in map", from)
	}

	if err := peer.SendStream(meta, file, fileInfo.Size()); err != nil {
		return fmt.Errorf("failed to send chunk stream: %w", err)
	}

	logger.Sugar.Infof("[PeerServer] Sent %d bytes of chunk %d to %s", fileInfo.Size(), v.ChunkId, from)
	return nil
}

func (p *PeerServer) handleRequestChunks(from string, msg protocol.FileMetadata) error {
	logger.Sugar.Infof("[PeerServer] Handling request for chunks of file: %s", msg.FileName)
	err := p.handleChunks(msg)
	if err != nil {
		logger.Sugar.Errorf("[PeerServer] Error handling chunks for file %s: %v", msg.FileName, err)
		return err
	}
	return nil
}

func (p *PeerServer) GetOrDialPeer(addr string) (transport.Node, error) {
	// Fast path: read lock check
	p.peerLock.RLock()
	if node, exists := p.peers[addr]; exists {
		p.peerLock.RUnlock()
		return node, nil
	}
	p.peerLock.RUnlock()

	// Slow path: write lock with double check
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	// Double check after acquiring write lock
	if node, exists := p.peers[addr]; exists {
		return node, nil
	}

	node, err := p.Transport.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial peer %s: %w", addr, err)
	}

	p.peers[addr] = node
	logger.Sugar.Debugf("[PeerServer] Dialed and cached peer: %s", addr)
	return node, nil
}

func (p *PeerServer) OnPeer(peer transport.Node) error {
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	if peer.Addr() == p.centralServerAddr {
		p.centralServerPeer = peer
		logger.Sugar.Infof("[PeerServer] Connected to central server: %s --> %s", peer.Addr(), p.centralServerAddr)
	} else {
		p.peers[peer.Addr()] = peer
		logger.Sugar.Infof("[PeerServer] New peer connected: %s --> %s", peer.Addr(), p.peerServerAddr)
	}

	return nil
}

func (p *PeerServer) RegisterPeer() error {
	logger.Sugar.Infof("[PeerServer] connecting to central server: %s", p.centralServerAddr)
	// Dial returns the Node, and HandleConn starts loop (which calls OnPeer)
	// We don't need to manually assign p.centralServerPeer here if OnPeer does it.
	// But Dial returns the node immediately.
	node, err := p.Transport.Dial(p.centralServerAddr)
	if err != nil {
		return fmt.Errorf("failed to dial central server: %w", err)
	}

	// Ensure OnPeer logic ran or manual assignment
	p.peerLock.Lock()
	p.centralServerPeer = node
	p.peerLock.Unlock()

	// 向服务器发送自己监听的地址
	if err := p.centralServerPeer.Send(protocol.PeerRegistration{ListenAddr: p.peerServerAddr}); err != nil {
		return fmt.Errorf("failed to send peer registration (listen=%s): %w", p.peerServerAddr, err)
	}

	logger.Sugar.Infof("[PeerServer] registered with central server: listen=%s", p.peerServerAddr)
	go p.startHeartbeat()
	return nil
}

func (p *PeerServer) startHeartbeat() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.quitCh:
			return
		case <-ticker.C:
			p.peerLock.Lock()
			centralServer := p.centralServerPeer
			p.peerLock.Unlock()

			if centralServer != nil {
				err := centralServer.Send(protocol.Heartbeat{Timestamp: time.Now().Unix()})
				if err != nil {
					logger.Sugar.Errorf("[PeerServer] failed to send heartbeat: err=%v", err)
				}
			}
		}
	}
}

func (p *PeerServer) RegisterFile(path string) error {
	logger.Sugar.Infof("[PeerServer] Registering file: %s", path)
	file, err := os.Open(path)
	if err != nil {
		logger.Sugar.Errorf("[PeerServer] Registering file error: %s", path)
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	fileName := strings.Split(fileInfo.Name(), ".")
	hashString, err := storage.HashFile(file)
	if err != nil {
		return fmt.Errorf("failed to hash file: %w", err)
	}

	validDir := fmt.Sprintf("chunks-%s", strings.ReplaceAll(p.peerServerAddr, ":", "-"))
	fileDirectory, err := p.store.CreateChunkDirectory(validDir, hashString)
	if err != nil {
		return fmt.Errorf("failed to create chunk directory: %w", err)
	}

	const chunkSize = 1024 * 1024 * 4
	err, chunkMap := p.store.DivideToChunk(file, chunkSize, fileDirectory, p.peerServerAddr)
	if err != nil {
		return fmt.Errorf("failed to process chunks: %w", err)
	}

	metadata := protocol.FileMetadata{
		FileId:        hashString,
		FileName:      fileName[0],
		FileExtension: fileName[1],
		FileSize:      uint32(fileInfo.Size()),
		ChunkSize:     chunkSize,
		ChunkInfo:     chunkMap,
	}

	time.Sleep(time.Millisecond * 600)
	p.peerLock.Lock()
	centralServer := p.centralServerPeer
	p.peerLock.Unlock()

	if centralServer == nil {
		return fmt.Errorf("not connected to central server")
	}

	err = centralServer.Send(metadata)

	if err != nil {
		return fmt.Errorf("failed to send file metadata to central server: %w", err)
	}

	logger.Sugar.Infof("[PeerServer] Successfully registered file: %s with ID: %s", fileInfo.Name(), hashString)
	return nil
}

func (p *PeerServer) registerAsSeeder(fileId string) error {
	p.downloadsMutex.RLock()
	isComplete, exists := p.completedDownloads[fileId]
	p.downloadsMutex.RUnlock()

	if !exists || !isComplete {
		return fmt.Errorf("file %s is not completely downloaded", fileId)
	}
	// 发送注册成为seed的消息
	registerMsg := protocol.RegisterSeeder{
		FileId:   fileId,
		PeerAddr: p.peerServerAddr,
	}

	//
	p.peerLock.Lock()
	centralServer := p.centralServerPeer
	p.peerLock.Unlock()

	if centralServer == nil {
		return fmt.Errorf("not connected to central server")
	}

	err := centralServer.Send(registerMsg)

	if err != nil {
		return fmt.Errorf("failed to send register seeder message: %w", err)
	}

	logger.Sugar.Infof("[PeerServer] Registered as seeder for file: %s", fileId)
	return nil
}

func (p *PeerServer) RequestChunkData(fileId string) error {
	logger.Sugar.Infof("[PeerServer] Requesting chunk data for file: %s", fileId)

	req := protocol.RequestChunkData{
		FileId: fileId,
	}

	p.peerLock.Lock()
	centralServer := p.centralServerPeer
	p.peerLock.Unlock()

	if centralServer == nil {
		return fmt.Errorf("not connected to central server")
	}

	err := centralServer.Send(req)

	if err != nil {
		return fmt.Errorf("failed to send chunk data request: %w", err)
	}

	logger.Sugar.Infof("[PeerServer] Successfully sent request for chunk data of file: %s", fileId)
	return nil
}

func (p *PeerServer) Start() error {
	logger.Sugar.Infof("[PeerServer] Starting peer server on address: %s", p.Transport.Addr())

	// Enable profiling for mutex and block contention
	runtime.SetMutexProfileFraction(1) // Record all mutex contentions
	runtime.SetBlockProfileRate(1)     // Record all blocking operations

	err := p.Transport.ListenAndAccept()
	if err != nil {
		return fmt.Errorf("failed to start listening: %w", err)
	}

	// Start pprof HTTP server (non-blocking, localhost only)
	go func() {
		logger.Sugar.Infof("[pprof] listening on http://localhost:6060/debug/pprof/")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			logger.Sugar.Errorf("[pprof] server error: %v", err)
		}
	}()

	// Start periodic metrics logging
	go monitor.LogPeriodic(30 * time.Second)

	err = p.RegisterPeer()
	if err != nil {
		logger.Sugar.Warnf("[PeerServer] Warning: Failed to register peer: %v", err)
	}

	p.messageLoop()
	return nil
}

func (p *PeerServer) GetStatus() string {
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	status := fmt.Sprintf("Peer Server Running on: %s\n", p.peerServerAddr)
	status += fmt.Sprintf("Central Server: %s ", p.centralServerAddr)
	if p.centralServerPeer != nil {
		status += "(Connected)\n"
	} else {
		status += "(Disconnected)\n"
	}
	status += fmt.Sprintf("Connected Peers: %d\n", len(p.peers))

	p.downloadsMutex.RLock()
	status += fmt.Sprintf("Completed Downloads: %d\n", len(p.completedDownloads))
	p.downloadsMutex.RUnlock()

	return status
}

func (p *PeerServer) Stop() {
	close(p.quitCh)
	p.Transport.Close()
}
