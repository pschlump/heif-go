package heic

var saoEdgeOffsets = [4][2][2]int{{{-1, 0}, {1, 0}}, {{0, -1}, {0, 1}}, {{-1, -1}, {1, 1}}, {{1, -1}, {-1, 1}}}

type saoPlaneSnapshot struct {
	plane         *plane
	rows          [3][]uint16
	ids           [3]int
	width, height int
}

func newSAOPlaneSnapshot(p *plane) saoPlaneSnapshot {
	return saoPlaneSnapshot{plane: p, ids: [3]int{-1, -1, -1}, width: p.width, height: p.height}
}

func (s *saoPlaneSnapshot) preserve(y, first, last int) {
	if y < 0 || y >= s.plane.height {
		return
	}
	for _, id := range s.ids {
		if id == y {
			return
		}
	}
	slot := -1
	for i, id := range s.ids {
		if id < 0 {
			slot = i
			break
		}
	}
	if slot < 0 {
		for i, id := range s.ids {
			if id < first || id > last {
				slot = i
				break
			}
		}
	}
	if len(s.rows[slot]) < s.plane.width {
		s.rows[slot] = make([]uint16, s.plane.width)
	}
	start := y * s.plane.stride
	copy(s.rows[slot], s.plane.data[start:start+s.plane.width])
	s.ids[slot] = y
}

func (s *saoPlaneSnapshot) at(x, y int) uint16 {
	for i, id := range s.ids {
		if id == y {
			return s.rows[i][x]
		}
	}
	return 0
}

func saoEdgeOffset(src *saoPlaneSnapshot, info *filterInfo, s *saoCTB, addr, px, py, sw, sh int) (int32, bool) {
	rec := int32(src.at(px, py))
	idx := int32(2)
	for _, off := range saoEdgeOffsets[s.eoClass&3] {
		nx, ny := px+off[0], py+off[1]
		if nx < 0 || ny < 0 || nx >= src.width || ny >= src.height {
			return 0, false
		}
		if !info.mayCross(info.ctbOf(nx*sw, ny*sh), addr) {
			return 0, false
		}
		v := rec - int32(src.at(nx, ny))
		if v > 0 {
			idx++
		} else if v < 0 {
			idx--
		}
	}
	switch idx {
	case 2:
		idx = 0
	case 0:
		idx = 1
	case 1:
		idx = 2
	}
	if idx == 0 {
		return 0, true
	}
	return int32(s.offsets[idx-1]), true
}

func applySAOCTB(dst *plane, src *saoPlaneSnapshot, cuFlags []uint8, min4W int, info *filterInfo, s saoCTB, addr, sw, sh int, bd uint8) {
	cx, cy := addr%info.picWCTBs, addr/info.picWCTBs
	x0, y0 := (cx<<info.ctbLog2)/sw, (cy<<info.ctbLog2)/sh
	x1, y1 := min(((cx+1)<<info.ctbLog2)/sw, dst.width), min(((cy+1)<<info.ctbLog2)/sh, dst.height)
	maxSample := int32(1<<bd) - 1
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			lx, ly := px*sw, py*sh
			idx := (ly>>2)*min4W + (lx >> 2)
			f := uint8(0)
			if idx < len(cuFlags) {
				f = cuFlags[idx]
			}
			if filterLocked(f, info) {
				continue
			}
			rec := int32(src.at(px, py))
			var delta int32
			if s.typeIdx == 1 {
				k := (rec >> max(int(bd)-5, 0)) - int32(s.bandPosition)
				if k < 0 || k >= 4 {
					continue
				}
				delta = int32(s.offsets[k])
			} else {
				v, ok := saoEdgeOffset(src, info, &s, addr, px, py, sw, sh)
				if !ok {
					continue
				}
				delta = v
			}
			dst.put(px, py, uint16(clip3(0, maxSample, rec+delta)))
		}
	}
}

func applySAO(pic *picture, info *filterInfo) {
	if !info.saoEnabled || info.picWCTBs == 0 {
		return
	}
	ncomp := 1
	if info.chromaArrayType != 0 {
		ncomp = 3
	}
	n := min(len(info.ctb), info.picWCTBs*info.picHCTBs)
	used := func(c int) bool {
		for _, f := range info.ctb[:n] {
			if f.sao[c].typeIdx == 1 || f.sao[c].typeIdx == 2 {
				return true
			}
		}
		return false
	}
	planes := [3]*plane{&pic.y, &pic.cb, &pic.cr}
	for c := range ncomp {
		if !used(c) {
			continue
		}
		sw, sh := max(info.subW, 1), max(info.subH, 1)
		bd := info.bitDepthC
		if c == 0 {
			sw, sh, bd = 1, 1, info.bitDepthY
		}
		snapshot := newSAOPlaneSnapshot(planes[c])
		for ry := 0; ry < info.picHCTBs; ry++ {
			for y := ry - 1; y <= ry+1; y++ {
				snapshot.preserve(y, ry-1, ry+1)
			}
			for cx := 0; cx < info.picWCTBs; cx++ {
				addr := ry*info.picWCTBs + cx
				if addr >= n {
					continue
				}
				s := info.ctb[addr].sao[c]
				if s.typeIdx == 1 || s.typeIdx == 2 {
					applySAOCTB(planes[c], &snapshot, pic.cuFlags, pic.min4W, info, s, addr, sw, sh, bd)
				}
			}
		}
	}
}
