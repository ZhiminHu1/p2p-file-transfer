package protocol

import (
	"encoding/gob"
	"io"
)

func init() {
	// Register types for GOB encoding
	gob.Register(FileMetadata{})
	gob.Register(ChunkMetadata{})
	gob.Register(RequestChunkData{})
	gob.Register(ChunkRequestToPeer{})
	gob.Register(RegisterSeeder{})
	gob.Register(ChunkDataResponse{})
	gob.Register(Heartbeat{})
	gob.Register(PeerRegistration{})
	gob.Register(DataMessage{})
}

// Message Types
const (
	IncomingStreamType  = 0x2
	IncomingMessageType = 0x1
)

// RPC represents a message received from the network
type RPC struct {
	From    string
	Payload any
}

// IncomingStream is a local wrapper for a network stream.
// It is NOT sent over the wire, but passed from Transport to Peer.
type IncomingStream struct {
	Stream io.Reader
	Length int64
	Meta   any // The metadata associated with the stream
	Done   chan struct{}
}

// DataMessage is a wrapper
type DataMessage struct {
	Incoming uint8
	Msg      any
}

// --- Domain Types ---

type FileRegister struct {
}

type PeerRegistration struct {
	ListenAddr string
}

type FileMetadata struct {
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
	Data    []byte // 流式数据
}

type Heartbeat struct {
	Timestamp int64
}
