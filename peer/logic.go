package peer

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tarun-kavipurapu/p2p-transfer/pkg"
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

func (p *PeerServer) handleChunks(fileMetaData pkg.FileMetaData) error {
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

	// Initialize chunk sizes
	// Most chunks have the same size, except possibly the last one
	chunkSizes := make(map[uint32]uint32)
	for index := range fileMetaData.ChunkInfo {
		size := fileMetaData.ChunkSize
		// Last chunk might be smaller
		if index == uint32(len(fileMetaData.ChunkInfo))-1 {
			remainingSize := fileMetaData.FileSize - (fileMetaData.ChunkSize * uint32(len(fileMetaData.ChunkInfo)-1))
			if remainingSize > 0 && remainingSize < fileMetaData.ChunkSize {
				size = remainingSize
			}
		}
		chunkSizes[index] = size
	}
	tracker.InitChunks(chunkSizes)

	// Start progress renderer
	renderer := NewProgressRenderer(tracker, true)
	go renderer.Start()
	defer renderer.StopAndWait()

	log.Printf("[PeerServer] Starting download of file %s (%s) with %d chunks", fileMetaData.FileName, fileMetaData.FileId, numOfChunks)

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
			log.Printf("[PeerServer] Failed to fetch chunk %d from peer %s: %v", chunkJob.ChunkIndex, chunkJob.PeerAddr, result.Err)
			tracker.FailChunk(chunkJob.ChunkIndex)
			lastError = result.Err
		} else {
			resultLock.Lock()
			successfulChunks++
			resultLock.Unlock()
			tracker.CompleteChunk(chunkJob.ChunkIndex)
		}
	}

	<-workerPool.Done()

	if successfulChunks != int(numOfChunks) {
		renderer.RenderError(fmt.Errorf("download incomplete: %d/%d chunks", successfulChunks, numOfChunks))
		return fmt.Errorf("download incomplete for file %s. Fetched %d/%d chunks. Last error: %v", fileMetaData.FileName, successfulChunks, numOfChunks, lastError)
	}

	// Mark tracker as complete
	tracker.MarkComplete()

	log.Printf("[PeerServer] All %d chunks fetched successfully for file %s. Reassembling...", numOfChunks, fileMetaData.FileName)
	err := p.store.ReassembleFile(fileMetaData.FileId, fileMetaData.FileName, fileMetaData.FileExtension, p.peerServAddr)
	if err != nil {
		return fmt.Errorf("error reassembling file %s: %w", fileMetaData.FileName, err)
	}

	log.Printf("[PeerServer] File %s reassembled successfully", fileMetaData.FileName)

	p.downloadsMutex.Lock()
	p.completedDownloads[fileMetaData.FileId] = true
	p.downloadsMutex.Unlock()

	log.Printf("[PeerServer] Marked file %s (%s) as completed download", fileMetaData.FileName, fileMetaData.FileId)

	err = p.registerAsSeeder(fileMetaData.FileId)
	if err != nil {
		return fmt.Errorf("failed to register as seeder for file %s: %w", fileMetaData.FileName, err)
	}

	log.Printf("[PeerServer] Successfully registered as a new seeder for file %s", fileMetaData.FileName)
	return nil
}
func (p *PeerServer) fileRequest(fileId string, chunkIndex uint32, peerAddr string, chunkHash string, tracker *DownloadTracker) error {
	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", peerAddr, err)
	}
	defer conn.Close()

	peer := pkg.NewTCPPeer(conn, true)

	// Update peer map
	p.peerLock.Lock()
	p.peers[peerAddr] = peer
	p.peerLock.Unlock()

	defer func() {
		p.peerLock.Lock()
		delete(p.peers, peerAddr)
		p.peerLock.Unlock()
	}()

	dataMessage := pkg.DataMessage{
		Payload: pkg.ChunkRequestToPeer{
			FileId:    fileId,
			ChunkHash: chunkHash,
			ChunkId:   chunkIndex,
			ChunkName: fmt.Sprintf("chunk_%d.chunk", chunkIndex),
		},
	}
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(dataMessage); err != nil {
		return fmt.Errorf("failed to encode chunk request: %w", err)
	}

	if err := peer.Send(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to send chunk request to peer %s: %w", peerAddr, err)
	}

	var chunkId int64
	var fileSize int64

	if err := binary.Read(peer, binary.LittleEndian, &chunkId); err != nil {
		return fmt.Errorf("failed to read chunk id from peer %s: %w", peerAddr, err)
	}

	if err := binary.Read(peer, binary.LittleEndian, &fileSize); err != nil {
		return fmt.Errorf("failed to read file size from peer %s: %w", peerAddr, err)
	}

	baseDir := fmt.Sprintf("chunks-%s", strings.Split(p.peerServAddr, ":")[0])
	folderDirectory, err := p.store.createChunkDirectory(baseDir, fileId)
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

	// Track download progress with periodic updates
	const progressUpdateInterval = 64 * 1024 // Update every 64KB
	var bytesCopied int64
	buffer := make([]byte, 32*1024) // 32KB buffer

	for bytesCopied < fileSize {
		toRead := int64(len(buffer))
		remaining := fileSize - bytesCopied
		if remaining < toRead {
			toRead = remaining
		}

		n, err := io.ReadFull(peer, buffer[:toRead])
		if err != nil {
			return fmt.Errorf("failed to read chunk data from peer %s: %w", peerAddr, err)
		}

		written, err := writeFile.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("failed to write chunk data: %w", err)
		}

		bytesCopied += int64(written)

		// Update progress periodically
		if tracker != nil && bytesCopied%progressUpdateInterval == 0 {
			tracker.UpdateChunkProgress(chunkIndex, uint64(bytesCopied))
		}
	}

	// Final progress update to ensure accurate count
	if tracker != nil {
		tracker.UpdateChunkProgress(chunkIndex, uint64(fileSize))
	}

	if _, err := writeFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start of file %s: %w", filePath, err)
	}

	if err := pkg.CheckFileHash(writeFile, chunkHash); err != nil {
		return fmt.Errorf("chunk hash verification failed: %w", err)
	}

	return nil
}
func assignChunks(fileMetaData pkg.FileMetaData) map[uint32]string {
	chunkPeerAssign := make(map[uint32]string) //chunkIndex->peer
	peerLoad := make(map[string]uint32)        //peerId ->number of chunks

	for index, chunkInfo := range fileMetaData.ChunkInfo {
		peers := chunkInfo.PeersWithChunk
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
