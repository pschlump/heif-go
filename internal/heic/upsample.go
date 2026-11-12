package heic

func chromaRowPair(y, ch int, c chromaFormat) (int, int, uint32) {
	if ch == 0 {
		return 0, 0, 4
	}
	if c.yShift() == 0 {
		r := min(y, ch-1)
		return r, r, 4
	}
	cy := min(y>>1, ch-1)
	near := cy - 1
	if y&1 != 0 {
		near = min(cy+1, ch-1)
	} else if near < 0 {
		near = 0
	}
	return cy, near, 3
}
func verticalBlend(a, b uint16, w0 uint32) uint16 {
	return uint16((uint32(a)*w0 + uint32(b)*(4-w0) + 2) >> 2)
}
func horizontalAverage(a, b uint16) uint16 { return uint16((uint32(a) + uint32(b) + 1) >> 1) }
func upsampleHorizontal(dst, src []uint16, xShift uint32) {
	n := len(src)
	if n == 0 || len(dst) == 0 {
		return
	}
	done := 0
	if xShift == 0 {
		done = min(len(dst), n)
		copy(dst[:done], src[:done])
	} else {
		body := min(n-1, len(dst)/2)
		for i := range body {
			dst[2*i] = src[i]
			dst[2*i+1] = horizontalAverage(src[i], src[i+1])
		}
		done = body * 2
	}
	for i := done; i < len(dst); i++ {
		dst[i] = src[n-1]
	}
}
func upsampleRow(dst, src0, src1 []uint16, w0, xShift uint32) {
	n := min(len(src0), len(src1))
	if n == 0 || len(dst) == 0 {
		return
	}
	done := 0
	if xShift == 0 {
		done = min(len(dst), n)
		for i := range done {
			dst[i] = verticalBlend(src0[i], src1[i], w0)
		}
	} else {
		body := min(n-1, len(dst)/2)
		for i := range body {
			dst[2*i] = verticalBlend(src0[i], src1[i], w0)
			dst[2*i+1] = verticalBlend(horizontalAverage(src0[i], src0[i+1]), horizontalAverage(src1[i], src1[i+1]), w0)
		}
		done = body * 2
	}
	edge := verticalBlend(src0[n-1], src1[n-1], w0)
	for i := done; i < len(dst); i++ {
		dst[i] = edge
	}
}
func upsamplePlane(src []uint16, stride, cw, ch, w, h uint32, c chromaFormat) []uint16 {
	out := make([]uint16, int(w*h))
	if cw == 0 || ch == 0 {
		return out
	}
	for y := range int(h) {
		r0, r1, weight := chromaRowPair(y, int(ch), c)
		o0, o1 := r0*int(stride), r1*int(stride)
		if o0+int(cw) > len(src) || o1+int(cw) > len(src) {
			break
		}
		upsampleRow(out[y*int(w):(y+1)*int(w)], src[o0:o0+int(cw)], src[o1:o1+int(cw)], weight, c.xShift())
	}
	return out
}
