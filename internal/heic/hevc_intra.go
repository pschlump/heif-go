package heic

type intraRefs struct {
	buf [129]uint16
	n   int
}

func newIntraRefs(n int) intraRefs           { return intraRefs{n: n} }
func (r *intraRefs) left(y int) uint16       { return r.buf[2*r.n-1-y] }
func (r *intraRefs) top(x int) uint16        { return r.buf[2*r.n+1+x] }
func (r *intraRefs) setLeft(y int, v uint16) { r.buf[2*r.n-1-y] = v }
func (r *intraRefs) setTop(x int, v uint16)  { r.buf[2*r.n+1+x] = v }

var intraAngles = [33]int32{32, 26, 21, 17, 13, 9, 5, 2, 0, -2, -5, -9, -13, -17, -21, -26, -32, -26, -21, -17, -13, -9, -5, -2, 0, 2, 5, 9, 13, 17, 21, 26, 32}
var inverseAngles = [15]int32{-4096, -1638, -910, -630, -482, -390, -315, -256, -315, -390, -482, -630, -910, -1638, -4096}

func filterIntraRefs(r *intraRefs, mode uint8, cIdx int, bitDepth, chromaArrayType uint8, strong, disable bool) intraRefs {
	n := r.n
	nn := 2 * n
	if disable || (cIdx != 0 && chromaArrayType != 3) {
		return *r
	}
	m := int32(mode)
	dist := min(abs32(m-26), abs32(m-10))
	thresh := 0
	if n == 8 {
		thresh = 7
	} else if n == 16 {
		thresh = 1
	}
	if mode == 1 || mode > 34 || n == 4 || dist <= int32(thresh) {
		return *r
	}
	c := int32(r.top(-1))
	te, le := int32(r.top(nn-1)), int32(r.left(nn-1))
	lim := int32(1<<bitDepth) >> 5
	bilin := strong && cIdx == 0 && n == 32 && abs32(c+te-2*int32(r.top(nn/2-1))) < lim && abs32(c+le-2*int32(r.left(nn/2-1))) < lim
	o := newIntraRefs(n)
	if bilin {
		o.setTop(-1, uint16(c))
		for i := range nn - 1 {
			w0, w1 := int32(nn-1-i), int32(i+1)
			o.setLeft(i, uint16((w0*c+w1*le+32)>>6))
			o.setTop(i, uint16((w0*c+w1*te+32)>>6))
		}
	} else {
		o.setTop(-1, uint16((int32(r.left(0))+2*c+int32(r.top(0))+2)>>2))
		for i := range nn - 1 {
			l := int32(r.left(i+1)) + 2*int32(r.left(i)) + int32(r.left(i-1))
			t := int32(r.top(i+1)) + 2*int32(r.top(i)) + int32(r.top(i-1))
			o.setLeft(i, uint16((l+2)>>2))
			o.setTop(i, uint16((t+2)>>2))
		}
	}
	o.setLeft(nn-1, uint16(le))
	o.setTop(nn-1, uint16(te))
	return o
}
func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
func predictIntra(r *intraRefs, mode uint8, cIdx int, bitDepth uint8, out []uint16, stride int) {
	n := r.n
	if n != 4 && n != 8 && n != 16 && n != 32 {
		return
	}
	if mode == 0 {
		predictPlanar(r, out, stride)
		return
	}
	if mode >= 2 && mode <= 34 {
		predictAngular(r, mode, cIdx, bitDepth, out, stride)
		return
	}
	predictDC(r, cIdx, out, stride)
}
func predictPlanar(r *intraRefs, out []uint16, st int) {
	n := r.n
	ni := int32(n)
	sh := uint(0)
	for 1<<sh < n {
		sh++
	}
	sh++
	tn, ln := int32(r.top(n)), int32(r.left(n))
	var top [32]int32
	for x := range n {
		top[x] = int32(r.top(x))
	}
	for y := range n {
		l, yi := int32(r.left(y)), int32(y)
		a, b := ni-1-yi, (yi+1)*ln+ni
		for x := range n {
			xi := int32(x)
			out[y*st+x] = uint16(((ni-1-xi)*l + (xi+1)*tn + a*top[x] + b) >> sh)
		}
	}
}
func predictDC(r *intraRefs, cIdx int, out []uint16, st int) {
	n := r.n
	sum := int32(n)
	for i := range n {
		sum += int32(r.top(i)) + int32(r.left(i))
	}
	sh := uint(0)
	for 1<<sh < n {
		sh++
	}
	sh++
	d := sum >> sh
	for y := range n {
		for x := range n {
			out[y*st+x] = uint16(d)
		}
	}
	if cIdx == 0 && n < 32 {
		for x := range n {
			out[x] = uint16((int32(r.top(x)) + 3*d + 2) >> 2)
		}
		for y := 1; y < n; y++ {
			out[y*st] = uint16((int32(r.left(y)) + 3*d + 2) >> 2)
		}
		out[0] = uint16((int32(r.left(0)) + 2*d + int32(r.top(0)) + 2) >> 2)
	}
}
func predictAngular(r *intraRefs, mode uint8, cIdx int, bitDepth uint8, out []uint16, st int) {
	n := r.n
	ni := int32(n)
	a := intraAngles[mode-2]
	vert := mode >= 18
	var rf [97]int32
	main := func(i int) int32 {
		if vert {
			return int32(r.top(i - 1))
		}
		return int32(r.left(i - 1))
	}
	side := func(i int) int32 {
		if vert {
			return int32(r.left(i - 1))
		}
		return int32(r.top(i - 1))
	}
	for i := range n + 1 {
		rf[n+i] = main(i)
	}
	if a < 0 {
		lim := (ni * a) >> 5
		if lim < -1 {
			inv := inverseAngles[mode-11]
			for x := int32(-1); x >= lim; x-- {
				rf[int(ni+x)] = side(int((x*inv + 128) >> 8))
			}
		}
	} else {
		for i := range n {
			rf[2*n+1+i] = main(n + 1 + i)
		}
	}
	for m := range n {
		t := int32(m+1) * a
		idx, f := t>>5, t&31
		b := int(ni + 1 + idx)
		if vert {
			if f == 0 {
				for k := range n {
					out[m*st+k] = uint16(rf[b+k])
				}
			} else {
				for k := range n {
					out[m*st+k] = uint16(((32-f)*rf[b+k] + f*rf[b+k+1] + 16) >> 5)
				}
			}
		} else {
			for k := range n {
				v := rf[b+k]
				if f != 0 {
					v = ((32-f)*rf[b+k] + f*rf[b+k+1] + 16) >> 5
				}
				out[k*st+m] = uint16(v)
			}
		}
	}
	if cIdx == 0 && n < 32 && (mode == 26 || mode == 10) {
		c, hi := int32(r.top(-1)), int32(1<<bitDepth)-1
		if mode == 26 {
			p := int32(r.top(0))
			for y := range n {
				v := p + ((int32(r.left(y)) - c) >> 1)
				out[y*st] = uint16(max(0, min(v, hi)))
			}
		} else {
			p := int32(r.left(0))
			for x := range n {
				v := p + ((int32(r.top(x)) - c) >> 1)
				out[x] = uint16(max(0, min(v, hi)))
			}
		}
	}
}
