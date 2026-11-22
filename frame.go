package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Frame limits; a larger frame means the stream is broken.
const (
	maxFrameHeader = 64 << 10
	maxFrameBody   = 64 << 20
)

var errFrameTooLarge = errors.New("frame too large")

// writeFrame writes header as JSON then body, each after its big-endian
// uint32 length.
func writeFrame(w io.Writer, header any, body []byte) error {
	h, err := json.Marshal(header)
	if err != nil {
		return err
	}
	if len(h) > maxFrameHeader || len(body) > maxFrameBody {
		return errFrameTooLarge
	}
	buf := make([]byte, 0, 8+len(h))
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(h)))
	buf = append(buf, h...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(body)))
	if _, err := w.Write(buf); err != nil {
		return err
	}
	if len(body) > 0 {
		_, err = w.Write(body)
	}
	return err
}

// readFrame reads one frame. It returns io.EOF only when the stream ends
// before a frame starts.
func readFrame(r io.Reader) (header, body []byte, err error) {
	if header, err = readPart(r, maxFrameHeader); err != nil {
		return nil, nil, err
	}
	if body, err = readPart(r, maxFrameBody); err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, nil, err
	}
	return header, body, nil
}

func readPart(r io.Reader, max int) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(n[:])
	if size > uint32(max) {
		return nil, fmt.Errorf("%w: %d bytes", errFrameTooLarge, size)
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return b, nil
}
