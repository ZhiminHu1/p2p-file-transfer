package peer

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/storage"
	"tarun-kavipurapu/p2p-transfer/pkg/transport"
	"tarun-kavipurapu/p2p-transfer/pkg/transport/tcp"
)

// this should have the Metadata of the files it posses
type PeerServer struct {
	peerLock           sync.Mutex
	peers              map[string]transport.Node
	centralServerPeer  transport.Node
	fileMetaDataInfo   map[string]*protocol.FileMetaData
	Transport          transport.Transport
	peerServAddr       string
	cServerAddr        string
	store              storage.Store
	completedDownloads map[string]bool
	downloadsMutex     sync.RWMutex

	pendingChunksLock sync.Mutex
	pendingChunks     map[string]chan protocol.ChunkDataResponse
}

func init() {
}

func NewPeerServer(addr string, cServerAddr string) *PeerServer {
	trans := tcp.NewTCPTransport(addr)
	peerServer := &PeerServer{
		peers:              make(map[string]transport.Node),
		peerServAddr:       addr,
		peerLock:           sync.Mutex{},
		Transport:          trans,
		cServerAddr:        cServerAddr,
		store:              storage.Store{},
		fileMetaDataInfo:   make(map[string]*protocol.FileMetaData),
		completedDownloads: make(map[string]bool),
		downloadsMutex:     sync.RWMutex{},
		pendingChunks:      make(map[string]chan protocol.ChunkDataResponse),
	}
	trans.SetOnPeer(peerServer.OnPeer)

	logger.Sugar.Infof("[PeerServer] Initialized with address: %s", peerServer.peerServAddr)
	return peerServer
}

func (p *PeerServer) loop() {
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
		}
	}
}

func (p *PeerServer) handleMessage(from string, msg protocol.RPC) error {
	switch v := msg.Payload.(type) {
	case protocol.FileMetaData:
		logger.Sugar.Infof("[PeerServer] Received FileMetaData for file: %s", v.FileName)
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
	case protocol.ChunkDataResponse:
		return p.handleChunkDataResponse(v)
	default:
		return fmt.Errorf("unknown message type received from %s: %T", from, v)
	}
}

// 使用 chunksMap优化
func (p *PeerServer) handleChunkDataResponse(resp protocol.ChunkDataResponse) error {
	key := fmt.Sprintf("%s:%d", resp.FileId, resp.ChunkId)
	p.pendingChunksLock.Lock()
	ch, exists := p.pendingChunks[key]
	if exists {
		delete(p.pendingChunks, key)
	}
	p.pendingChunksLock.Unlock()

	if exists {
		ch <- resp
	} else {
		logger.Sugar.Warnf("[PeerServer] Warning: Received chunk data for %s but no pending request found.", key)
	}
	return nil
}

func (p *PeerServer) SendChunkData(from string, v protocol.ChunkRequestToPeer) error {
	// Logic to read file and send data
	filePath := fmt.Sprintf("C:\\Users\\13237\\Desktop\\githubproject\\p2p-file-transfer\\cmd\\peer/chunks-%s/%s/%s", strings.Split(p.peerServAddr, ":")[0], v.FileId, v.ChunkName)
	logger.Sugar.Infof("[PeerServer] Sending chunk %d of file %s to %s", v.ChunkId, v.FileId, from)

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

	data := make([]byte, fileInfo.Size())
	_, err = file.Read(data)
	if err != nil {
		return err
	}

	response := protocol.ChunkDataResponse{
		FileId:  v.FileId,
		ChunkId: v.ChunkId,
		Data:    data,
	}

	p.peerLock.Lock()
	peer, ok := p.peers[from]
	p.peerLock.Unlock()

	if !ok {
		return fmt.Errorf("peer %s not found in map", from)
	}

	if err := peer.Send(response); err != nil {
		return fmt.Errorf("failed to send chunk data: %w", err)
	}

	logger.Sugar.Infof("[PeerServer] Sent %d bytes of chunk %d to %s", len(data), v.ChunkId, from)
	return nil
}

func (p *PeerServer) handleRequestChunks(from string, msg protocol.FileMetaData) error {
	logger.Sugar.Infof("[PeerServer] Handling request for chunks of file: %s", msg.FileName)
	err := p.handleChunks(msg)
	if err != nil {
		logger.Sugar.Errorf("[PeerServer] Error handling chunks for file %s: %v", msg.FileName, err)
		return err
	}
	return nil
}

func (p *PeerServer) OnPeer(peer transport.Node) error {
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	// Heuristic to check if it's the central server
	// NOTE: This might be fragile if address string format differs slightly
	if peer.Addr() == p.cServerAddr {
		p.centralServerPeer = peer
		logger.Sugar.Infof("[PeerServer] Connected to central server: %s", peer.Addr())
	} else {
		p.peers[peer.Addr()] = peer
		logger.Sugar.Infof("[PeerServer] New peer connected: %s", peer.Addr())
	}

	return nil
}

func (p *PeerServer) RegisterPeer() error {
	logger.Sugar.Infof("[PeerServer] Attempting to register with central server: %s", p.cServerAddr)
	// Dial returns the Node, and HandleConn starts loop (which calls OnPeer)
	// We don't need to manually assign p.centralServerPeer here if OnPeer does it.
	// But Dial returns the node immediately.
	node, err := p.Transport.Dial(p.cServerAddr)
	if err != nil {
		return fmt.Errorf("failed to dial central server: %w", err)
	}

	// Ensure OnPeer logic ran or manual assignment
	p.peerLock.Lock()
	p.centralServerPeer = node
	p.peerLock.Unlock()

	// In original protocol, just connecting was enough?
	// Or did we send a message?
	// Original code: err = p.centralServerPeer.Send([]byte{pkg.IncomingMessage}) -- wait, that line was commented out in original file!

	logger.Sugar.Infof("[PeerServer] Successfully registered with central server")
	return nil
}

func (p *PeerServer) RegisterFile(path string) error {
	logger.Sugar.Infof("[PeerServer] Registering file: %s", path)
	file, err := os.Open(path)
	if err != nil {
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

	vaildDir := fmt.Sprintf("chunks-%s", strings.Split(p.peerServAddr, ":")[0])
	fileDirectory, err := p.store.CreateChunkDirectory(vaildDir, hashString) // Fixed method casing if needed, previously createChunkDirectory (lowercase)
	if err != nil {
		return fmt.Errorf("failed to create chunk directory: %w", err)
	}

	const chunkSize = 1024 * 1024 * 4
	err, chunkMap := p.store.DivideToChunk(file, chunkSize, fileDirectory, p.peerServAddr) // Fixed casing
	if err != nil {
		return fmt.Errorf("failed to process chunks: %w", err)
	}

	metaData := protocol.FileMetaData{
		FileId:        hashString,
		FileName:      fileName[0],
		FileExtension: fileName[1],
		FileSize:      uint32(fileInfo.Size()),
		ChunkSize:     chunkSize,
		ChunkInfo:     chunkMap,
	}

	time.Sleep(time.Millisecond * 600)
	p.peerLock.Lock()
	cs := p.centralServerPeer
	p.peerLock.Unlock()

	if cs == nil {
		return fmt.Errorf("not connected to central server")
	}

	err = cs.Send(metaData)

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
		PeerAddr: p.peerServAddr,
	}

	// TCP连接并发数据写入不是安全的
	p.peerLock.Lock()
	cs := p.centralServerPeer
	p.peerLock.Unlock()

	if cs == nil {
		return fmt.Errorf("not connected to central server")
	}

	err := cs.Send(registerMsg)

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
	cs := p.centralServerPeer
	p.peerLock.Unlock()

	if cs == nil {
		return fmt.Errorf("not connected to central server")
	}

	err := cs.Send(req)

	if err != nil {
		return fmt.Errorf("failed to send chunk data request: %w", err)
	}

	logger.Sugar.Infof("[PeerServer] Successfully sent request for chunk data of file: %s", fileId)
	return nil
}

func (p *PeerServer) Start() error {
	logger.Sugar.Infof("[PeerServer] Starting peer server on address: %s", p.Transport.Addr())

	err := p.Transport.ListenAndAccept()
	if err != nil {
		return fmt.Errorf("failed to start listening: %w", err)
	}

	err = p.RegisterPeer()
	if err != nil {
		logger.Sugar.Warnf("[PeerServer] Warning: Failed to register peer: %v", err)
	}

	p.loop()
	return nil
}
