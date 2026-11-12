package heic

import (
	"encoding/binary"
	"unicode/utf8"
)

type reader struct {
	data []byte
	pos  int
}

func newReader(data []byte) *reader { return &reader{data: data} }
func (r *reader) remaining() int    { return len(r.data) - r.pos }
func (r *reader) position() int     { return r.pos }
func (r *reader) empty() bool       { return r.pos == len(r.data) }
func (r *reader) rest() []byte      { return r.data[r.pos:] }

func (r *reader) take(n int, what string) ([]byte, error) {
	if n < 0 || n > len(r.data)-r.pos {
		return nil, truncated(what)
	}
	out := r.data[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

func (r *reader) skip(n int, what string) error {
	_, err := r.take(n, what)
	return err
}

func (r *reader) u8(what string) (uint8, error) {
	b, err := r.take(1, what)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *reader) u16(what string) (uint16, error) {
	b, err := r.take(2, what)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *reader) u24(what string) (uint32, error) {
	b, err := r.take(3, what)
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2]), nil
}

func (r *reader) u32(what string) (uint32, error) {
	b, err := r.take(4, what)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (r *reader) u64(what string) (uint64, error) {
	b, err := r.take(8, what)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}

func (r *reader) fourcc(what string) (string, error) {
	b, err := r.take(4, what)
	return string(b), err
}

func (r *reader) uintN(size uint8, what string) (uint64, error) {
	switch size {
	case 0:
		return 0, nil
	case 4:
		v, err := r.u32(what)
		return uint64(v), err
	case 8:
		return r.u64(what)
	default:
		return 0, malformed("integer field width must be 0, 4 or 8")
	}
}

func (r *reader) fullBox(what string) (uint8, uint32, error) {
	v, err := r.u8(what)
	if err != nil {
		return 0, 0, err
	}
	f, err := r.u24(what)
	return v, f, err
}

func (r *reader) cstr(what string) (string, error) {
	rest := r.rest()
	end := len(rest)
	for i, b := range rest {
		if b == 0 {
			end = i
			break
		}
	}
	b, err := r.take(end, what)
	if err != nil {
		return "", err
	}
	if r.remaining() > 0 {
		if err := r.skip(1, what); err != nil {
			return "", err
		}
	}
	if !utf8.Valid(b) {
		return "", malformed("string is not valid UTF-8")
	}
	return string(b), nil
}
