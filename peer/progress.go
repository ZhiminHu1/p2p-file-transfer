package peer

import (
	"sync"
	"time"
)

// ChunkState rep
// resents the current state of a chunk download
type ChunkState int

const (
	ChunkPending ChunkState = iota
	ChunkDownloading
	ChunkCompleted
	ChunkFailed
)

// String returns a string representation of the chunk state
func (s ChunkState) String() string {
	switch s {
	case ChunkPending:
		return "pending"
	case ChunkDownloading:
		return "downloading"
	case ChunkCompleted:
		return "completed"
	case ChunkFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Icon returns an icon representation of the chunk state
func (s ChunkState) Icon() string {
	switch s {
	case ChunkPending:
		return "⏳"
	case ChunkDownloading:
		return "↓"
	case ChunkCompleted:
		return "✓"
	case ChunkFailed:
		return "✗"
	default:
		return "?"
	}
}

// ChunkProgress tracks the progress of a single chunk
type ChunkProgress struct {
	Index      uint32
	State      ChunkState
	PeerAddr   string
	BytesDone  uint64
	BytesTotal uint64
	StartTime  time.Time
	EndTime    time.Time
}

// IsComplete returns true if the chunk is completed
func (cp *ChunkProgress) IsComplete() bool {
	return cp.State == ChunkCompleted
}

// ProgressPercentage returns the progress percentage (0-100)
func (cp *ChunkProgress) ProgressPercentage() float64 {
	if cp.BytesTotal == 0 {
		return 0
	}
	return float64(cp.BytesDone) / float64(cp.BytesTotal) * 100
}

// DownloadTracker tracks the progress of an entire file download
type DownloadTracker struct {
	mu              sync.RWMutex
	FileId          string
	FileName        string
	FileExtension   string
	FileSize        uint64
	TotalChunks     uint32
	Chunks          map[uint32]*ChunkProgress // index -> progress
	ActivePeers     map[string]int            // peerAddr -> active chunk count
	StartTime       time.Time
	EndTime         time.Time
	BytesDownloaded uint64

	// Speed calculation
	lastBytes    uint64
	lastTime     time.Time
	currentSpeed float64 // bytes/sec

	// Statistics
	failedChunks  uint32
	retryCount    uint32
	completedSize uint64
}

// NewDownloadTracker creates a new download tracker
func NewDownloadTracker(fileId, fileName, fileExt string, fileSize uint64, totalChunks uint32) *DownloadTracker {
	return &DownloadTracker{
		FileId:        fileId,
		FileName:      fileName,
		FileExtension: fileExt,
		FileSize:      fileSize,
		TotalChunks:   totalChunks,
		Chunks:        make(map[uint32]*ChunkProgress),
		ActivePeers:   make(map[string]int),
		StartTime:     time.Now(),
		lastTime:      time.Now(),
	}
}

// InitChunks initializes all chunk states with their sizes
func (dt *DownloadTracker) InitChunks(chunkSizes map[uint32]uint32) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	for index, size := range chunkSizes {
		dt.Chunks[index] = &ChunkProgress{
			Index:      index,
			State:      ChunkPending,
			BytesTotal: uint64(size),
		}
	}
}

// StartChunk marks a chunk as being downloaded
func (dt *DownloadTracker) StartChunk(index uint32, peerAddr string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if chunk, exists := dt.Chunks[index]; exists {
		chunk.State = ChunkDownloading
		chunk.PeerAddr = peerAddr
		chunk.StartTime = time.Now()
	}
	dt.ActivePeers[peerAddr]++
}

// UpdateChunkProgress updates the downloaded bytes for a chunk
func (dt *DownloadTracker) UpdateChunkProgress(index uint32, bytesDone uint64) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if chunk, exists := dt.Chunks[index]; exists {
		if bytesDone > chunk.BytesDone {
			increment := bytesDone - chunk.BytesDone
			chunk.BytesDone = bytesDone
			dt.BytesDownloaded += increment
		} else {
			chunk.BytesDone = bytesDone
		}

	}
}

// CompleteChunk marks a chunk as completed
func (dt *DownloadTracker) CompleteChunk(index uint32) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if chunk, exists := dt.Chunks[index]; exists {
		wasDownloading := chunk.State == ChunkDownloading
		chunk.State = ChunkCompleted

		// Update BytesDownloaded: add any remaining bytes not yet counted
		remainingBytes := chunk.BytesTotal - chunk.BytesDone
		if remainingBytes > 0 {
			dt.BytesDownloaded += remainingBytes
		}
		chunk.BytesDone = chunk.BytesTotal

		chunk.EndTime = time.Now()
		dt.completedSize += chunk.BytesTotal

		if wasDownloading {
			dt.ActivePeers[chunk.PeerAddr]--
			if dt.ActivePeers[chunk.PeerAddr] <= 0 {
				delete(dt.ActivePeers, chunk.PeerAddr)
			}
		}
	}
}

// FailChunk marks a chunk as failed
func (dt *DownloadTracker) FailChunk(index uint32) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if chunk, exists := dt.Chunks[index]; exists {
		wasDownloading := chunk.State == ChunkDownloading
		chunk.State = ChunkFailed
		chunk.BytesDone = 0
		chunk.EndTime = time.Now()

		if wasDownloading {
			dt.ActivePeers[chunk.PeerAddr]--
			if dt.ActivePeers[chunk.PeerAddr] <= 0 {
				delete(dt.ActivePeers, chunk.PeerAddr)
			}
		}
		dt.failedChunks++
	}
}

// RetryChunk marks a failed chunk as pending for retry
func (dt *DownloadTracker) RetryChunk(index uint32) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if chunk, exists := dt.Chunks[index]; exists {
		if chunk.State == ChunkFailed {
			chunk.State = ChunkPending
			dt.BytesDownloaded -= chunk.BytesDone // 需要减去已经下载的chunk进度
			chunk.BytesDone = 0
			chunk.StartTime = time.Time{}
			chunk.EndTime = time.Time{}
		}
	}
	dt.retryCount++
}

// UpdateSpeed calculates and updates the current download speed
func (dt *DownloadTracker) UpdateSpeed() float64 {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(dt.lastTime).Seconds()

	if elapsed >= 0.5 { // Update every 0.5 seconds
		bytesDiff := dt.BytesDownloaded - dt.lastBytes
		if elapsed > 0 && bytesDiff >= 0 {
			dt.currentSpeed = float64(bytesDiff) / elapsed
		}
		dt.lastBytes = dt.BytesDownloaded
		dt.lastTime = now
	}

	return dt.currentSpeed
}

// GetProgress returns current progress statistics
// Returns: completed count, total count, speed (bytes/s), active peer count, failed count
func (dt *DownloadTracker) GetProgress() (completed, total uint32, speed float64, peerCount int, failed uint32) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	completed = uint32(0)
	for _, chunk := range dt.Chunks {
		if chunk.State == ChunkCompleted {
			completed++
		}
	}

	return completed, dt.TotalChunks, dt.currentSpeed, len(dt.ActivePeers), dt.failedChunks
}

// GetETA returns the estimated time remaining
func (dt *DownloadTracker) GetETA() time.Duration {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	remainingBytes := int64(dt.FileSize - dt.BytesDownloaded)
	if dt.currentSpeed <= 0 || remainingBytes <= 0 {
		return 0
	}

	return time.Duration(remainingBytes/int64(dt.currentSpeed)) * time.Second
}

// GetBytesDownloaded returns the total bytes downloaded
func (dt *DownloadTracker) GetBytesDownloaded() uint64 {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.BytesDownloaded
}

// GetFileSize returns the total file size
func (dt *DownloadTracker) GetFileSize() uint64 {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.FileSize
}

// GetActivePeers returns a list of active peer addresses
func (dt *DownloadTracker) GetActivePeers() []string {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	peers := make([]string, 0, len(dt.ActivePeers))
	for peer := range dt.ActivePeers {
		peers = append(peers, peer)
	}
	return peers
}

// IsComplete returns true if all chunks are completed
func (dt *DownloadTracker) IsComplete() bool {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if len(dt.Chunks) == 0 {
		return false
	}

	for _, chunk := range dt.Chunks {
		if chunk.State != ChunkCompleted {
			return false
		}
	}
	return true
}

// MarkComplete marks the download as complete
func (dt *DownloadTracker) MarkComplete() {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	dt.EndTime = time.Now()
}

// GetElapsedTime returns the elapsed time since download started
func (dt *DownloadTracker) GetElapsedTime() time.Duration {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if !dt.EndTime.IsZero() {
		return dt.EndTime.Sub(dt.StartTime)
	}
	return time.Since(dt.StartTime)
}

// GetChunkStatus returns the status of a specific chunk
func (dt *DownloadTracker) GetChunkStatus(index uint32) (ChunkState, bool) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if chunk, exists := dt.Chunks[index]; exists {
		return chunk.State, true
	}
	return ChunkPending, false
}

// GetCompletedChunks returns a list of completed chunk indices
func (dt *DownloadTracker) GetCompletedChunks() []uint32 {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	completed := make([]uint32, 0)
	for index, chunk := range dt.Chunks {
		if chunk.State == ChunkCompleted {
			completed = append(completed, index)
		}
	}
	return completed
}

// GetFailedChunks returns a list of failed chunk indices
func (dt *DownloadTracker) GetFailedChunks() []uint32 {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	failed := make([]uint32, 0)
	for index, chunk := range dt.Chunks {
		if chunk.State == ChunkFailed {
			failed = append(failed, index)
		}
	}
	return failed
}

// GetPendingChunks returns a list of pending chunk indices
func (dt *DownloadTracker) GetPendingChunks() []uint32 {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	pending := make([]uint32, 0)
	for index, chunk := range dt.Chunks {
		if chunk.State == ChunkPending {
			pending = append(pending, index)
		}
	}
	return pending
}
