package peer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"tarun-kavipurapu/p2p-transfer/pkg/protocol"
	"tarun-kavipurapu/p2p-transfer/pkg/storage"
)

type ChunkJob struct {
	FileId          string
	ChunkIndex      uint32
	PeerAddr        string
	ChunkHash       string
	p               *PeerServer
	downloadTracker *DownloadTracker
}

func (cj *ChunkJob) Execute() error {
	return cj.p.fileRequest(cj.FileId, cj.ChunkIndex, cj.PeerAddr, cj.ChunkHash, cj.downloadTracker)
}

func (p *PeerServer) handleChunks(fileMetadata protocol.FileMetadata) error {
	numOfChunks := uint32(len(fileMetadata.ChunkInfo))
	chunkPeerAssign := assignChunks(fileMetadata)

	// 关注点分离，提前建立连接
	logger.Sugar.Infof("[PeerServer] Pre-connecting to required peers...")
	uniquePeers := make(map[string]struct{})
	for _, peerAddr := range chunkPeerAssign {
		uniquePeers[peerAddr] = struct{}{}
	}

	var wgDial sync.WaitGroup
	for peerAddr := range uniquePeers {
		wgDial.Add(1)
		go func(addr string) {
			defer wgDial.Done()
			if _, err := p.GetOrDialPeer(addr); err != nil {
				logger.Sugar.Warnf("[PeerServer] Failed to pre-connect to %s: %v", addr, err)
			}
		}(peerAddr)
	}
	wgDial.Wait()

	// Create progress tracker
	downloadTracker := NewDownloadTracker(
		fileMetadata.FileId,
		fileMetadata.FileName,
		fileMetadata.FileExtension,
		uint64(fileMetadata.FileSize),
		numOfChunks,
	)

	// Initialize chunk sizes to display progress of downloading
	chunkSizes := make(map[uint32]uint32)
	for index := range fileMetadata.ChunkInfo {
		size := fileMetadata.ChunkSize
		if index == uint32(len(fileMetadata.ChunkInfo)) {
			// fix correct size of last chunk
			remainder := fileMetadata.FileSize % fileMetadata.ChunkSize
			if remainder > 0 {
				size = remainder
			}
		}
		chunkSizes[index] = size
	}
	downloadTracker.InitChunks(chunkSizes)

	// Start progress renderer
	renderer := NewProgressRenderer(downloadTracker, true)
	go renderer.Start()
	defer renderer.StopAndWait()

	logger.Sugar.Infof("[PeerServer] Starting download of file %s (%s) with %d chunks", fileMetadata.FileName, fileMetadata.FileId, numOfChunks)

	const maxWorkers = 12
	workerPool := NewWorkerPool(maxWorkers)
	workerPool.Start()

	var wg sync.WaitGroup
	for index, peerAddr := range chunkPeerAssign {
		logger.Sugar.Debugf("[PeerServer] assign chunk: file=%s.%s fileId=%s chunk=%d source=%s",
			fileMetadata.FileName, fileMetadata.FileExtension, fileMetadata.FileId, index, peerAddr)
		wg.Add(1)
		go func(index uint32, peerAddr string) {
			defer wg.Done()
			chunk := fileMetadata.ChunkInfo[index]
			// Mark chunk as starting
			downloadTracker.StartChunk(index, peerAddr)
			job := &ChunkJob{
				FileId:          fileMetadata.FileId,
				ChunkIndex:      index,
				PeerAddr:        peerAddr,
				ChunkHash:       chunk.ChunkHash,
				p:               p,
				downloadTracker: downloadTracker,
			}
			workerPool.Submit(job)
		}(index, peerAddr)
	}
	go func() {
		wg.Wait()
		workerPool.Stop()
	}()

	successfulChunks := 0
	var resultLock sync.Mutex
	var lastError error

	for result := range workerPool.Results() {
		chunkJob := result.Job.(*ChunkJob)
		if result.Err != nil {
			logger.Sugar.Errorf("[PeerServer] chunk download failed: fileId=%s chunk=%d from=%s err=%v", chunkJob.FileId, chunkJob.ChunkIndex, chunkJob.PeerAddr, result.Err)
			downloadTracker.FailChunk(chunkJob.ChunkIndex)
			lastError = result.Err
		} else {
			resultLock.Lock()
			successfulChunks++
			resultLock.Unlock()
			downloadTracker.CompleteChunk(chunkJob.ChunkIndex)
		}
	}
	// 等待所有chunk下载完成
	<-workerPool.Done()

	if successfulChunks != int(numOfChunks) {
		return fmt.Errorf("download incomplete for file %s. Fetched %d/%d chunks. Last error: %v", fileMetadata.FileName, successfulChunks, numOfChunks, lastError)
	}

	// Mark tracker as complete
	downloadTracker.MarkComplete()

	logger.Sugar.Infof("[PeerServer] All %d chunks fetched successfully for file %s. Reassembling...", numOfChunks, fileMetadata.FileName)
	err := p.store.ReassembleFile(fileMetadata.FileId, fileMetadata.FileName, fileMetadata.FileExtension, p.peerServerAddr)
	if err != nil {
		return fmt.Errorf("error reassembling file %s: %w", fileMetadata.FileName, err)
	}

	logger.Sugar.Infof("[PeerServer] File %s reassembled successfully", fileMetadata.FileName)

	p.downloadsMutex.Lock()
	p.completedDownloads[fileMetadata.FileId] = true
	p.downloadsMutex.Unlock()

	logger.Sugar.Infof("[PeerServer] Marked file %s (%s) as completed download", fileMetadata.FileName, fileMetadata.FileId)

	err = p.registerAsSeeder(fileMetadata.FileId)
	if err != nil {
		return fmt.Errorf("failed to register as seeder for file %s: %w", fileMetadata.FileName, err)
	}

	logger.Sugar.Infof("[PeerServer] Successfully registered as a new seeder for file %s", fileMetadata.FileName)
	return nil
}

// use async request to opt speed of downloading fileMetadata
func (p *PeerServer) fileRequest(fileId string, chunkIndex uint32, peerAddr string, chunkHash string, downloadTracker *DownloadTracker) error {
	// Async Request with Transport
	// 1. Setup response channel
	key := fmt.Sprintf("%s:%d", fileId, chunkIndex)
	respCh := make(chan protocol.ChunkMetaDataResponse, 1)

	p.pendingChunksLock.Lock()
	p.pendingChunks[key] = respCh
	p.pendingChunksLock.Unlock()

	defer func() {
		p.pendingChunksLock.Lock()
		delete(p.pendingChunks, key)
		p.pendingChunksLock.Unlock()
	}()

	// 如果 peerAddr 是自己，直接验证本地 chunk
	if peerAddr == p.peerServerAddr {
		logger.Sugar.Infof("[PeerServer] Chunk %d owner is self, validating local chunk", chunkIndex)
		return p.verifyChunkHash(fileId, chunkIndex, chunkHash, downloadTracker)
	}

	// 2. Connect/Reuse connection
	p.peerLock.Lock()
	node, exists := p.peers[peerAddr]
	p.peerLock.Unlock()
	if !exists {
		// Fallback: try to dial once more
		logger.Sugar.Warnf("[PeerServer] peer %s not in cache, attempting to dial", peerAddr)
		newNode, err := p.GetOrDialPeer(peerAddr)
		if err != nil {
			return fmt.Errorf("peer %s not available: %w", peerAddr, err)
		}
		node = newNode
	}

	// 3. Send Request
	req := protocol.ChunkRequestToPeer{
		FileId:    fileId,
		ChunkHash: chunkHash,
		ChunkId:   chunkIndex,
		ChunkName: fmt.Sprintf("chunk_%d.chunk", chunkIndex),
	}

	if err := node.Send(req); err != nil {
		return fmt.Errorf("failed to send chunk request: %w", err)
	}

	// 4. Wait for response (peer 层已经完成存储)
	select {
	case meta := <-respCh:
		// 验证 hash 并更新进度
		return p.verifyChunkHash(meta.FileId, meta.ChunkId, chunkHash, downloadTracker)
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for chunk %d", chunkIndex)
	}
}

func (p *PeerServer) verifyChunkHash(fileId string, chunkIndex uint32, expectedHash string, downloadTracker *DownloadTracker) error {
	baseDir := fmt.Sprintf("chunks-%s", strings.Split(p.peerServerAddr, ":")[0])
	folderDirectory := filepath.Join(baseDir, fileId)
	chunkPath := filepath.Join(folderDirectory, fmt.Sprintf("chunk_%d.chunk", chunkIndex))

	// 打开已保存的文件验证 hash
	file, err := os.Open(chunkPath)
	if err != nil {
		return fmt.Errorf("failed to open chunk for hash verification: %w", err)
	}
	defer file.Close()

	if err := storage.CheckFileHash(file, expectedHash); err != nil {
		os.Remove(chunkPath)
		return fmt.Errorf("hash verification failed for chunk %d: %w", chunkIndex, err)
	}

	// 获取文件大小用于更新进度
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat chunk file: %w", err)
	}

	// update tracker mark as chunk completed
	if downloadTracker != nil {
		downloadTracker.UpdateChunkProgress(chunkIndex, uint64(fileInfo.Size()))
	}

	return nil
}

func assignChunks(fileMetadata protocol.FileMetadata) map[uint32]string {
	chunkPeerAssign := make(map[uint32]string) //chunkIndex->peer
	peerLoad := make(map[string]uint32)        //peerId ->number of chunks

	for index, chunkInfo := range fileMetadata.ChunkInfo {
		peers := chunkInfo.Owners
		if len(peers) > 0 {
			minPeer := peers[0]
			for _, peer := range peers {
				if peerLoad[peer] < peerLoad[minPeer] {
					minPeer = peer
				}
			}
			peerLoad[minPeer]++
			chunkPeerAssign[index] = minPeer
		}
	}

	return chunkPeerAssign
}
