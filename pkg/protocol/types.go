package protocol

import "encoding/gob"

func init() {
	// Register types for GOB encoding
	gob.Register(FileMetaData{})
	gob.Register(ChunkMetadata{})
	gob.Register(RequestChunkData{})
	gob.Register(ChunkRequestToPeer{})
	gob.Register(RegisterSeeder{})
	gob.Register(ChunkDataResponse{})
	gob.Register(Heartbeat{})
	gob.Register(PeerRegistration{})

}

// Message Types
const (
	// 未来可能会优化大文件传输效率，
	// TCP的流式传输可以实现零拷贝
	IncomingStream  = 0x2
	IncomingMessage = 0x1
)

// RPC represents a message received from the network
type RPC struct {
	From    string
	Payload any
}

// DataMessage wraps the payload for transmission
type DataMessage struct {
	Payload any
}

// --- Domain Types ---

type FileRegister struct {
}

type PeerRegistration struct {
	ListenAddr string
}

type FileMetaData struct {
	FileId        string
	FileName      string
	FileExtension string
	FileSize      uint32
	ChunkSize     uint32
	ChunkInfo     map[uint32]ChunkMetadata // ChunkIndex -> ChunkMetadata
}

type ChunkMetadata struct {
	ChunkId   uint32
	ChunkHash string
	ChunkName string
	Owners    []string // addresses of peers who have this chunk
}

type RequestChunkData struct {
	FileId string
}

type ChunkRequestToPeer struct {
	FileId    string
	ChunkHash string
	ChunkId   uint32
	ChunkName string
}

type RegisterSeeder struct {
	FileId   string
	PeerAddr string
}

type ChunkDataResponse struct {
	FileId  string
	ChunkId uint32
	Data    []byte
}

type Heartbeat struct {
	Timestamp int64
}
