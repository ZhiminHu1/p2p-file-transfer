package tcp

import (
	"encoding/binary"
	"io"
)

// Frame Types
const (
	FrameTypeControl = 0x01
	FrameTypeStream  = 0x02
)

// Header is the fixed-size frame header
// [Type (1 byte)] + [Length (4 bytes)]
const HeaderSize = 5

// writeFrameHeader writes the frame header to the writer
func writeFrameHeader(w io.Writer, msgType uint8, length uint32) error {
	buf := make([]byte, HeaderSize)
	buf[0] = msgType
	binary.BigEndian.PutUint32(buf[1:], length)

	_, err := w.Write(buf)
	return err
}

// readFrameHeader reads the frame header from the reader
// returns msgType, length, and error
func readFrameHeader(r io.Reader) (uint8, uint32, error) {
	buf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, 0, err
	}

	msgType := buf[0]
	length := binary.BigEndian.Uint32(buf[1:])

	return msgType, length, nil
}

// LimitedReader wraps io.LimitReader to provide a cleaner interface if needed,
// but standard io.LimitReader is usually sufficient.
// We just need to ensure we drain the reader if the user didn't read it all,
// but our protocol design enforces the user (or transport) MUST read exactly 'length' bytes.
