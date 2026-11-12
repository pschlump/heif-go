package heic

func decodeSAOSyntax(d *decoderState, ctbRS int) {
	w := d.geo.wCTBs
	rx, ry := ctbRS%w, ctbRS/w
	mergeLeft, mergeUp := false, false
	if rx > 0 && saoMergeable(d, ctbRS, ctbRS-1) {
		mergeLeft = d.cab.decision(ctxSAOMerge) != 0
	}
	if !mergeLeft && ry > 0 && saoMergeable(d, ctbRS, ctbRS-w) {
		mergeUp = d.cab.decision(ctxSAOMerge) != 0
	}
	if mergeLeft || mergeUp {
		src := ctbRS - w
		if mergeLeft {
			src = ctbRS - 1
		}
		d.fi.ctb[ctbRS].sao = d.fi.ctb[src].sao
		return
	}
	components := 1
	if d.cat != 0 {
		components = 3
	}
	maxAbs := uint32(1<<(min(d.sps.bitDepthY, 10)-5)) - 1
	maxAbsC := uint32(1<<(min(d.sps.bitDepthC, 10)-5)) - 1
	var shared saoCTB
	for c := range components {
		enabled := d.sh.saoChroma
		if c == 0 {
			enabled = d.sh.saoLuma
		}
		if !enabled {
			d.fi.ctb[ctbRS].sao[c] = saoCTB{}
			continue
		}
		s := saoCTB{}
		if c == 2 {
			s.typeIdx, s.eoClass = shared.typeIdx, shared.eoClass
		}
		if c != 2 {
			s.typeIdx = readSAOType(d)
		}
		if s.typeIdx != 0 {
			cap := maxAbsC
			if c == 0 {
				cap = maxAbs
			}
			var abs [4]uint32
			for k := range abs {
				for abs[k] < cap && d.cab.bypass() == 1 {
					abs[k]++
				}
			}
			scale := d.pps.saoOffsetScaleChroma
			if c == 0 {
				scale = d.pps.saoOffsetScaleLuma
			}
			if s.typeIdx == 1 {
				for k, a := range abs {
					neg := a != 0 && d.cab.bypass() == 1
					v := int16(a << scale)
					if neg {
						v = -v
					}
					s.offsets[k] = v
				}
				s.bandPosition = uint8(d.cab.bypassBits(5))
			} else {
				for k, a := range abs {
					v := int16(a << scale)
					if k >= 2 {
						v = -v
					}
					s.offsets[k] = v
				}
				if c != 2 {
					s.eoClass = uint8(d.cab.bypassBits(2))
				}
			}
		}
		if c == 1 {
			shared = s
		}
		d.fi.ctb[ctbRS].sao[c] = s
	}
}
func readSAOType(d *decoderState) uint8 {
	if d.cab.decision(ctxSAOType) == 0 {
		return 0
	}
	if d.cab.bypass() == 0 {
		return 1
	}
	return 2
}
func saoMergeable(d *decoderState, cur, src int) bool {
	return d.geo.tileID[cur] == d.geo.tileID[src] && d.fi.sliceAddr[cur] == d.fi.sliceAddr[src]
}
