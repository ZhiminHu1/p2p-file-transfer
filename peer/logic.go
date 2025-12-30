package peer

import (
	"bytes"
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

type ChunkStatus struct {
	Index      uint32
	isFetched  bool
	isFetching bool
}

type ChunkJob struct {
	FileId     string
	ChunkIndex uint32
	PeerAddr   string
	ChunkHash  string
	p          *PeerServer
	tracker    *DownloadTracker
}

func (cj *ChunkJob) Execute() error {
	return cj.p.fileRequest(cj.FileId, cj.ChunkIndex, cj.PeerAddr, cj.ChunkHash, cj.tracker)
}

func (p *PeerServer) handleChunks(fileMetaData protocol.FileMetaData) error {
	numOfChunks := uint32(len(fileMetaData.ChunkInfo))
	chunkPeerAssign := assignChunks(fileMetaData)

	// Create progress tracker
	tracker := NewDownloadTracker(
		fileMetaData.FileId,
		fileMetaData.FileName,
		fileMetaData.FileExtension,
		uint64(fileMetaData.FileSize),
		numOfChunks,
	)

	// Initialize chunk sizes to display progress of downloading
	chunkSizes := make(map[uint32]uint32)
	for index := range fileMetaData.ChunkInfo {
		size := fileMetaData.ChunkSize
		if index == uint32(len(fileMetaData.ChunkInfo)) {
			// fix correct size of last chunk
			remainder := fileMetaData.FileSize & fileMetaData.ChunkSize
			if remainder > 0 {
				size = remainder
			}
		}
		chunkSizes[index] = size
	}
	tracker.InitChunks(chunkSizes)

	// Start progress renderer
	renderer := NewProgressRenderer(tracker, true)
	go renderer.Start()
	defer renderer.StopAndWait()

	logger.Sugar.Infof("[PeerServer] Starting download of file %s (%s) with %d chunks", fileMetaData.FileName, fileMetaData.FileId, numOfChunks)

	const maxWorkers = 5
	workerPool := NewWorkerPool(maxWorkers)
	workerPool.Start()

	var wg sync.WaitGroup
	for index, peerAddr := range chunkPeerAssign {
		wg.Add(1)
		go func(index uint32, peerAddr string) {
			defer wg.Done()
			chunk := fileMetaData.ChunkInfo[index]
			// Mark chunk as starting
			tracker.StartChunk(index, peerAddr)
			job := &ChunkJob{
				FileId:     fileMetaData.FileId,
				ChunkIndex: index,
				PeerAddr:   peerAddr,
				ChunkHash:  chunk.ChunkHash,
				p:          p,
				tracker:    tracker,
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
			logger.Sugar.Errorf("[PeerServer] Failed to fetch chunk %d from peer %s: %v", chunkJob.ChunkIndex, chunkJob.PeerAddr, result.Err)
			tracker.FailChunk(chunkJob.ChunkIndex)
			lastError = result.Err
		} else {
			resultLock.Lock()
			successfulChunks++
			resultLock.Unlock()
			tracker.CompleteChunk(chunkJob.ChunkIndex)
		}
	}
	// 等待所有chunk下载完成
	<-workerPool.Done()

	if successfulChunks != int(numOfChunks) {
		renderer.RenderError(fmt.Errorf("download incomplete: %d/%d chunks", successfulChunks, numOfChunks))
		return fmt.Errorf("download incomplete for file %s. Fetched %d/%d chunks. Last error: %v", fileMetaData.FileName, successfulChunks, numOfChunks, lastError)
	}

	// Mark tracker as complete
	tracker.MarkComplete()

	logger.Sugar.Infof("[PeerServer] All %d chunks fetched successfully for file %s. Reassembling...", numOfChunks, fileMetaData.FileName)
	err := p.store.ReassembleFile(fileMetaData.FileId, fileMetaData.FileName, fileMetaData.FileExtension, p.peerServAddr)
	if err != nil {
		return fmt.Errorf("error reassembling file %s: %w", fileMetaData.FileName, err)
	}

	logger.Sugar.Infof("[PeerServer] File %s reassembled successfully", fileMetaData.FileName)

	p.downloadsMutex.Lock()
	p.completedDownloads[fileMetaData.FileId] = true
	p.downloadsMutex.Unlock()

	logger.Sugar.Infof("[PeerServer] Marked file %s (%s) as completed download", fileMetaData.FileName, fileMetaData.FileId)

	err = p.registerAsSeeder(fileMetaData.FileId)
	if err != nil {
		return fmt.Errorf("failed to register as seeder for file %s: %w", fileMetaData.FileName, err)
	}

	logger.Sugar.Infof("[PeerServer] Successfully registered as a new seeder for file %s", fileMetaData.FileName)
	return nil
}

// use async request to opt speed of downloading fileMetaData
func (p *PeerServer) fileRequest(fileId string, chunkIndex uint32, peerAddr string, chunkHash string, tracker *DownloadTracker) error {
	// Async Request with Transport

	// 1. Setup response channel
	key := fmt.Sprintf("%s:%d", fileId, chunkIndex)
	respCh := make(chan protocol.ChunkDataResponse, 1)

	p.pendingChunksLock.Lock()
	p.pendingChunks[key] = respCh
	p.pendingChunksLock.Unlock()

	defer func() {
		p.pendingChunksLock.Lock()
		delete(p.pendingChunks, key)
		p.pendingChunksLock.Unlock()
	}()

	// 2. Connect/Reuse connection
	p.peerLock.Lock()
	node, exist := p.peers[peerAddr]
	p.peerLock.Unlock()

	if !exist {
		logger.Sugar.Infof("connect peer addr : %s", peerAddr)
		// Connect connection
		newNode, err := p.Transport.Dial(peerAddr)
		if err != nil {
			return fmt.Errorf("failed to dial peer %s: %w", peerAddr, err)
		}

		p.peerLock.Lock()
		// Double check if connection was established while we were dialing
		// 保证对同一个peer 只会有一条TCP连接
		if existingNode, ok := p.peers[peerAddr]; ok {
			newNode.Close()
			node = existingNode
		} else {
			p.peers[peerAddr] = newNode
			node = newNode
		}
		p.peerLock.Unlock()
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

	// 4. Wait for response
	select {

	case resp := <-respCh:
		// Process response
		return p.saveChunk(fileId, chunkIndex, resp.Data, chunkHash, tracker)
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for chunk %d", chunkIndex)
	}
}

func (p *PeerServer) saveChunk(fileId string, chunkIndex uint32, data []byte, expectedHash string, tracker *DownloadTracker) error {
	// Verify Hash in memory before writing to disk
	dataReader := bytes.NewReader(data)
	calculatedHash, err := storage.HashChunk(dataReader)
	if err != nil {
		return fmt.Errorf("failed to calculate hash from memory: %w", err)
	}

	if calculatedHash != expectedHash {
		return fmt.Errorf("chunk hash verification failed (memory). Expected: %s, Got: %s", expectedHash, calculatedHash)
	}

	baseDir := fmt.Sprintf("chunks-%s", strings.Split(p.peerServAddr, ":")[0])
	folderDirectory, err := p.store.CreateChunkDirectory(baseDir, fileId)
	if err != nil {
		return fmt.Errorf("failed to create chunk directory: %w", err)
	}

	chunkName := fmt.Sprintf("chunk_%d.chunk", chunkIndex)
	filePath := filepath.Join(folderDirectory, chunkName)

	writeFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer writeFile.Close()

	if _, err := writeFile.Write(data); err != nil {
		return err
	}
	// update tracker mark as chunk completed
	if tracker != nil {
		tracker.UpdateChunkProgress(chunkIndex, uint64(len(data)))
	}

	return nil
}

func assignChunks(fileMetaData protocol.FileMetaData) map[uint32]string {
	chunkPeerAssign := make(map[uint32]string) //chunkIndex->peer
	peerLoad := make(map[string]uint32)        //peerId ->number of chunks

	for index, chunkInfo := range fileMetaData.ChunkInfo {
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
