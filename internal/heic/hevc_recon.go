package heic

func gatherReferences(d *decoderState, x, y, n, cIdx int) intraRefs {
	sw, sh := 1, 1
	if cIdx != 0 {
		sw, sh = d.sps.subW, d.sps.subH
	}
	p := &d.pic.y
	if cIdx == 1 {
		p = &d.pic.cb
	} else if cIdx == 2 {
		p = &d.pic.cr
	}
	bd := d.sps.bitDepthY
	if cIdx != 0 {
		bd = d.sps.bitDepthC
	}
	xc, yc := x*sw, y*sh
	total := 4*n + 1
	r := newIntraRefs(n)
	var avail [129]bool
	minTB := 1 << d.geo.minTBLog2
	stepX, stepY := max(minTB/sw, 1), max(minTB/sh, 1)
	all := true
	leftIn, aboveIn := x > 0 && x-1 < p.width, y > 0 && y-1 < p.height
	if leftIn {
		col := x - 1
		for k := 0; k < 2*n; k += stepY {
			yy := y + k
			if yy >= p.height {
				all = false
				break
			}
			if d.available(xc, yc, col*sw, yy*sh) {
				end := min(yy+stepY, p.height, y+2*n)
				for yj := yy; yj < end; yj++ {
					idx := 2*n - 1 - k - (yj - yy)
					r.buf[idx] = p.data[yj*p.stride+col]
					avail[idx] = true
				}
			} else {
				all = false
			}
		}
	} else {
		all = false
	}
	if leftIn && aboveIn && d.available(xc, yc, (x-1)*sw, (y-1)*sh) {
		r.buf[2*n] = p.data[(y-1)*p.stride+x-1]
		avail[2*n] = true
	} else {
		all = false
	}
	if aboveIn {
		row := (y - 1) * p.stride
		for k := 0; k < 2*n; k += stepX {
			xx := x + k
			if xx >= p.width {
				all = false
				break
			}
			if d.available(xc, yc, xx*sw, (y-1)*sh) {
				end := min(xx+stepX, p.width, x+2*n)
				i0 := 2*n + 1 + k
				for j, v := range p.data[row+xx : row+end] {
					r.buf[i0+j] = v
					avail[i0+j] = true
				}
			} else {
				all = false
			}
		}
	} else {
		all = false
	}
	if !all {
		substituteReferences(&r, &avail, total, bd)
	}
	return r
}
func substituteReferences(r *intraRefs, avail *[129]bool, total int, bitDepth uint8) {
	any := false
	first := 0
	for i := range total {
		if avail[i] {
			if !any {
				first = i
			}
			any = true
			break
		}
	}
	if !any {
		mid := uint16(1) << (bitDepth - 1)
		for i := range total {
			r.buf[i] = mid
		}
		return
	}
	if !avail[0] {
		r.buf[0] = r.buf[first]
	}
	last := r.buf[0]
	for i := range total {
		if avail[i] {
			last = r.buf[i]
		} else {
			r.buf[i] = last
		}
	}
}
func reconstructBlock(d *decoderState, x, y, log2, cIdx int, mode uint8, hasResid, ts bool) error {
	n := 1 << log2
	bd := d.sps.bitDepthY
	if cIdx != 0 {
		bd = d.sps.bitDepthC
	}
	raw := gatherReferences(d, x, y, n, cIdx)
	filtered := filterIntraRefs(&raw, mode, cIdx, bd, d.cat, d.sps.strongIntraSmoothing, d.sps.intraSmoothingDisabled)
	predictIntra(&filtered, mode, cIdx, bd, d.pred[:], n)
	if hasResid && !d.tqBypass {
		qp := d.qpLuma()
		if cIdx != 0 {
			qp = d.qpChroma(cIdx)
		}
		dequantize(d.coeffs[:], n, qp, bd, d.factors(log2, cIdx, ts))
		if ts {
			transformSkip(d.coeffs[:], n, bd, d.sps.transformSkipRotation)
		} else {
			trType := uint8(0)
			if cIdx == 0 && n == 4 {
				trType = 1
			}
			inverseTransform(d.coeffs[:], n, trType, bd)
		}
	}
	maxSample := int32(1<<bd) - 1
	p := &d.pic.y
	if cIdx == 1 {
		p = &d.pic.cb
	} else if cIdx == 2 {
		p = &d.pic.cr
	}
	w, h := min(n, max(p.width-x, 0)), min(n, max(p.height-y, 0))
	if w == 0 || h == 0 {
		return nil
	}
	for j := range h {
		o := (y+j)*p.stride + x
		for i := range w {
			v := int32(d.pred[j*n+i])
			if hasResid {
				v += d.coeffs[j*n+i]
			}
			p.data[o+i] = uint16(max(0, min(v, maxSample)))
		}
	}
	return nil
}
