package pkg

import "time"

type PeerMetadata struct {
	PeerId     string
	Addr       string
	Port       uint16
	LastActive time.Time
}

type ChunkMetadata struct {
	ChunkHash      string
	ChunkIndex     uint32
	PeersWithChunk []string // 存储拥有该chunk的 peers 地址
}
type FileMetaData struct {
	FileId        string //this is nothing but file hash
	FileName      string
	FileExtension string
	FileSize      uint32
	ChunkSize     uint32
	ChunkInfo     map[uint32]*ChunkMetadata //chunkNumber-->ChunkMetaData

}
