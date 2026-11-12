package heic

import "math/bits"

type bitReader struct {
	data           []byte
	pos, end, stop int
}

func newBitReader(data []byte) *bitReader {
	last := len(data)
	for last > 0 && data[last-1] == 0 {
		last--
	}
	stop := 0
	if last > 0 {
		stop = (last-1)*8 + 7 - bits.TrailingZeros8(data[last-1])
	}
	return &bitReader{data: data, end: len(data) * 8, stop: stop}
}
func (r *bitReader) u1() (uint32, error) {
	if r.pos >= r.end {
		return 0, truncated("an HEVC bitstream")
	}
	v := uint32((r.data[r.pos>>3] >> (7 - (r.pos & 7))) & 1)
	r.pos++
	return v, nil
}
func (r *bitReader) u(n uint32) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if n > 32 {
		return 0, malformed("u(n) with n > 32")
	}
	if int(n) > r.end-r.pos {
		return 0, truncated("an HEVC bitstream")
	}
	var v uint32
	left := int(n)
	for left > 0 {
		b := uint32(r.data[r.pos>>3])
		off := r.pos & 7
		avail := 8 - off
		take := min(left, avail)
		chunk := (b >> (avail - take)) & ((1 << take) - 1)
		v = v<<take | chunk
		r.pos += take
		left -= take
	}
	return v, nil
}
func (r *bitReader) ue() (uint32, error) {
	zeros := uint32(0)
	for {
		b, err := r.u1()
		if err != nil {
			return 0, err
		}
		if b != 0 {
			break
		}
		zeros++
		if zeros > 31 {
			return 0, malformed("ue(v) prefix too long")
		}
	}
	if zeros == 0 {
		return 0, nil
	}
	rest, err := r.u(zeros)
	if err != nil {
		return 0, err
	}
	return (1 << zeros) - 1 + rest, nil
}
func (r *bitReader) se() (int32, error) {
	k, err := r.ue()
	if err != nil {
		return 0, err
	}
	mag := int32((k + 1) >> 1)
	if k&1 == 1 {
		return mag, nil
	}
	return -mag, nil
}
func (r *bitReader) skip(n int) error {
	if n < 0 || n > r.end-r.pos {
		return truncated("an HEVC bitstream")
	}
	r.pos += n
	return nil
}
func (r *bitReader) byteAlign()         { r.pos = (r.pos + 7) &^ 7 }
func (r *bitReader) moreRBSPData() bool { return r.pos < r.stop }
func (r *bitReader) bytePos() int       { return (r.pos + 7) >> 3 }

type bitWriter struct {
	bytes []byte
	nbits int
}

func (w *bitWriter) put(v uint32, n uint32) {
	for i := int(n) - 1; i >= 0; i-- {
		if w.nbits&7 == 0 {
			w.bytes = append(w.bytes, 0)
		}
		w.bytes[w.nbits>>3] |= uint8((v>>i)&1) << (7 - (w.nbits & 7))
		w.nbits++
	}
}
func (w *bitWriter) ue(v uint32) {
	c := v + 1
	n := bits.Len32(c)
	w.put(0, uint32(n-1))
	w.put(c, uint32(n))
}
func (w *bitWriter) se(v int32) {
	var k uint32
	if v > 0 {
		k = uint32(v)*2 - 1
	} else {
		k = uint32(-v) * 2
	}
	w.ue(k)
}
func (w *bitWriter) rbspTrailing() {
	w.put(1, 1)
	for w.nbits&7 != 0 {
		w.put(0, 1)
	}
}
