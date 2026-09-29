package dsmr

import (
	"bufio"
	"errors"
	"io"
)

// MaxTelegramSize bounds a single telegram. Real telegrams are well under 2 KiB;
// the limit protects memory when the serial port produces garbage.
const MaxTelegramSize = 4096

// Reader extracts raw telegrams from a byte stream. It resynchronises on the
// '/' header and returns everything up to and including the '!' terminator
// line. The returned slice is reused by the next call and must not be retained.
type Reader struct {
	br  *bufio.Reader
	buf []byte
}

// NewReader wraps r. Only a fixed 4 KiB buffer plus one telegram buffer are
// ever allocated.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		br:  bufio.NewReaderSize(r, MaxTelegramSize),
		buf: make([]byte, 0, MaxTelegramSize),
	}
}

// Next blocks until a complete telegram has been read and returns its raw
// bytes. It does not validate the CRC; use Parse for that. Errors from the
// underlying reader are returned as is.
func (r *Reader) Next() ([]byte, error) {
	r.buf = r.buf[:0]
	for {
		line, err := r.br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Line longer than any legitimate telegram line: drop it and resync.
			r.buf = r.buf[:0]
			if _, derr := r.br.ReadSlice('\n'); derr != nil && !errors.Is(derr, bufio.ErrBufferFull) {
				return nil, derr
			}
			continue
		}
		if err != nil {
			if err == io.EOF && len(line) > 0 && len(r.buf) > 0 && line[0] == '!' {
				// Final telegram without a trailing newline (files).
				r.buf = append(r.buf, line...)
				return r.buf, nil
			}
			return nil, err
		}
		if len(line) == 0 {
			continue
		}
		switch {
		case line[0] == '/':
			r.buf = append(r.buf[:0], line...)
		case len(r.buf) == 0:
			// Not inside a telegram yet; skip until the header shows up.
		case len(r.buf)+len(line) > MaxTelegramSize:
			r.buf = r.buf[:0]
		default:
			r.buf = append(r.buf, line...)
			if line[0] == '!' {
				return r.buf, nil
			}
		}
	}
}
