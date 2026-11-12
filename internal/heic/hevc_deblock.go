package heic

var betaTable = [52]int32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 52, 54, 56, 58, 60, 62, 64}
var tcTable = [54]int32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 5, 5, 6, 6, 7, 8, 9, 10, 11, 13, 14, 16, 18, 20, 22, 24}

func clip3(lo, hi, v int32) int32 { return max(lo, min(v, hi)) }

type filterSegment struct{ first, along, line int }

func newFilterSegment(stride int, ver bool, e, s, taps int) filterSegment {
	if ver {
		return filterSegment{first: s*stride + e - taps, along: 1, line: stride}
	}
	return filterSegment{first: (e-taps)*stride + s, along: stride, line: 1}
}
func (s *filterSegment) at(i, j int) int { return s.first + i*s.line + j*s.along }
func filterLocked(f uint8, info *filterInfo) bool {
	return f&flagTQBypass != 0 || f&flagPCM != 0 && info.pcmLoopFilterDisabled
}
func filterLumaLine(sm *[8]int32, de uint8, dep, deq bool, tc, maxSample int32) [8]int32 {
	o := *sm
	p3, p2, p1, p0 := sm[0], sm[1], sm[2], sm[3]
	q0, q1, q2, q3 := sm[4], sm[5], sm[6], sm[7]
	if de == 2 {
		o[3] = clip3(p0-2*tc, p0+2*tc, (p2+2*p1+2*p0+2*q0+q1+4)>>3)
		o[2] = clip3(p1-2*tc, p1+2*tc, (p2+p1+p0+q0+2)>>2)
		o[1] = clip3(p2-2*tc, p2+2*tc, (2*p3+3*p2+p1+p0+q0+4)>>3)
		o[4] = clip3(q0-2*tc, q0+2*tc, (p1+2*p0+2*q0+2*q1+q2+4)>>3)
		o[5] = clip3(q1-2*tc, q1+2*tc, (p0+q0+q1+q2+2)>>2)
		o[6] = clip3(q2-2*tc, q2+2*tc, (p0+q0+q1+3*q2+2*q3+4)>>3)
	} else if de == 1 {
		raw := (9*(q0-p0) - 3*(q1-p1) + 8) >> 4
		if abs32(raw) < tc*10 {
			delta := clip3(-tc, tc, raw)
			o[3] = clip3(0, maxSample, p0+delta)
			o[4] = clip3(0, maxSample, q0-delta)
			if dep {
				dv := clip3(-(tc >> 1), tc>>1, (((p2+p0+1)>>1)-p1+delta)>>1)
				o[2] = clip3(0, maxSample, p1+dv)
			}
			if deq {
				dv := clip3(-(tc >> 1), tc>>1, (((q2+q0+1)>>1)-q1-delta)>>1)
				o[5] = clip3(0, maxSample, q1+dv)
			}
		}
	}
	return o
}
func filterLumaSegment(pic *picture, info *filterInfo, ver bool, e, s int) {
	px, py, qx, qy := e-1, s, e, s
	if !ver {
		px, py, qx, qy = s, e-1, s, e
	}
	qc := info.ctbOf(qx, qy)
	if qc >= len(info.ctb) {
		return
	}
	cf := info.ctb[qc]
	if cf.deblockDisabled || !info.mayCross(info.ctbOf(px, py), qc) {
		return
	}
	pl, ql := filterLocked(pic.cuFlags[pic.idx4(px, py)], info), filterLocked(pic.cuFlags[pic.idx4(qx, qy)], info)
	if pl && ql {
		return
	}
	qpl := (int32(pic.qpY[pic.idx4(qx, qy)]) + int32(pic.qpY[pic.idx4(px, py)]) + 1) >> 1
	scale := int32(1) << max(int(info.bitDepthY)-8, 0)
	beta := betaTable[clip3(0, 51, qpl+int32(cf.betaOffsetDiv2)<<1)] * scale
	qtc := clip3(0, 53, qpl+2+int32(cf.tcOffsetDiv2)<<1)
	tc := tcTable[qtc] * scale
	seg := newFilterSegment(pic.y.stride, ver, e, s, 4)
	data := pic.y.data
	var sm [4][8]int32
	for i := range 4 {
		for j := range 8 {
			sm[i][j] = int32(data[seg.at(i, j)])
		}
	}
	dp := func(l *[8]int32) int32 { return abs32(l[1] - 2*l[2] + l[3]) }
	dq := func(l *[8]int32) int32 { return abs32(l[6] - 2*l[5] + l[4]) }
	dp0, dq0, dp3, dq3 := dp(&sm[0]), dq(&sm[0]), dp(&sm[3]), dq(&sm[3])
	dpq0, dpq3 := dp0+dq0, dp3+dq3
	if dpq0+dpq3 >= beta {
		return
	}
	sam := func(l *[8]int32, dpq int32) bool {
		return 2*dpq < beta>>2 && abs32(l[0]-l[3])+abs32(l[4]-l[7]) < beta>>3 && abs32(l[3]-l[4]) < (5*tc+1)>>1
	}
	de := uint8(1)
	if sam(&sm[0], dpq0) && sam(&sm[3], dpq3) {
		de = 2
	}
	thr := (beta + (beta >> 1)) >> 3
	dep, deq := dp0+dp3 < thr, dq0+dq3 < thr
	maxSample := int32(1<<info.bitDepthY) - 1
	lo, hi := 1, 7
	if pl {
		lo = 4
	}
	if ql {
		hi = 4
	}
	for i := range 4 {
		o := filterLumaLine(&sm[i], de, dep, deq, tc, maxSample)
		for j := lo; j < hi; j++ {
			data[seg.at(i, j)] = uint16(o[j])
		}
	}
}
func filterLumaDirection(pic *picture, info *filterInfo, ver bool) {
	kind := uint8(edgeHor)
	elen, slen := pic.y.height, pic.y.width
	if ver {
		kind = edgeVer
		elen, slen = pic.y.width, pic.y.height
	}
	for by := range pic.min8H {
		for bx := range pic.min8W {
			if pic.edges[by*pic.min8W+bx]&kind == 0 {
				continue
			}
			e, s0 := by*8, bx*8
			if ver {
				e, s0 = bx*8, by*8
			}
			if e == 0 || e+3 >= elen {
				continue
			}
			for segment := range 2 {
				s := s0 + segment*4
				if s+3 < slen {
					filterLumaSegment(pic, info, ver, e, s)
				}
			}
		}
	}
}
func filterChromaSegment(pic *picture, info *filterInfo, ver bool, e, s int) {
	sw, sh := info.subW, info.subH
	lqx, lqy := e*sw, s*sh
	lpx, lpy := lqx-1, lqy
	if !ver {
		lqx, lqy = s*sw, e*sh
		lpx, lpy = lqx, lqy-1
	}
	qc := info.ctbOf(lqx, lqy)
	if qc >= len(info.ctb) {
		return
	}
	cf := info.ctb[qc]
	if cf.deblockDisabled || !info.mayCross(info.ctbOf(lpx, lpy), qc) {
		return
	}
	pl, ql := filterLocked(pic.cuFlags[pic.idx4(lpx, lpy)], info), filterLocked(pic.cuFlags[pic.idx4(lqx, lqy)], info)
	if pl && ql {
		return
	}
	qpl := (int32(pic.qpY[pic.idx4(lqx, lqy)]) + int32(pic.qpY[pic.idx4(lpx, lpy)]) + 1) >> 1
	scale := int32(1) << max(int(info.bitDepthC)-8, 0)
	maxSample := int32(1<<info.bitDepthC) - 1
	for comp := range 2 {
		off := int32(cf.crQPOffset)
		if comp == 0 {
			off = int32(cf.cbQPOffset)
		}
		qpc := chromaQPFromLuma(qpl+off, info.chromaArrayType)
		tc := tcTable[clip3(0, 53, qpc+2+int32(cf.tcOffsetDiv2)<<1)] * scale
		p := &pic.cb
		if comp == 1 {
			p = &pic.cr
		}
		seg := newFilterSegment(p.stride, ver, e, s, 2)
		for i := range 4 {
			p1, p0 := int32(p.data[seg.at(i, 0)]), int32(p.data[seg.at(i, 1)])
			q0, q1 := int32(p.data[seg.at(i, 2)]), int32(p.data[seg.at(i, 3)])
			delta := clip3(-tc, tc, (((q0-p0)<<2)+p1-q1+4)>>3)
			if !pl {
				p.data[seg.at(i, 1)] = uint16(clip3(0, maxSample, p0+delta))
			}
			if !ql {
				p.data[seg.at(i, 2)] = uint16(clip3(0, maxSample, q0-delta))
			}
		}
	}
}
func filterChromaDirection(pic *picture, info *filterInfo, ver bool) {
	if info.chromaArrayType == 0 || info.subW == 0 || info.subH == 0 {
		return
	}
	kind := uint8(edgeHor)
	elen, slen := pic.cb.height, pic.cb.width
	if ver {
		kind = edgeVer
		elen, slen = pic.cb.width, pic.cb.height
	}
	for e := 8; e+1 < elen; e += 8 {
		for s := 0; s+3 < slen; s += 4 {
			lx, ly := e*info.subW, s*info.subH
			if !ver {
				lx, ly = s*info.subW, e*info.subH
			}
			cell := (ly>>3)*pic.min8W + (lx >> 3)
			if cell < len(pic.edges) && pic.edges[cell]&kind != 0 {
				filterChromaSegment(pic, info, ver, e, s)
			}
		}
	}
}
func deblockPicture(pic *picture, info *filterInfo) {
	filterLumaDirection(pic, info, true)
	filterChromaDirection(pic, info, true)
	filterLumaDirection(pic, info, false)
	filterChromaDirection(pic, info, false)
}
