package heic

type savedContext struct {
	row   int
	ctx   [numCABACContexts]uint8
	valid bool
}

func decodeSlice(s *sps, p *pps, g *geometry, sf *scalingFactors, pic *picture, fi *filterInfo, h sliceHeader, r *rbsp) error {
	starts := substreamStarts(&h, r)
	qpBDY, qpBDC := 6*(int32(s.bitDepthY)-8), 6*(int32(s.bitDepthC)-8)
	cab, err := newCABAC(r.data, starts[0], h.qp)
	if err != nil {
		return err
	}
	template := ctbFilter{deblockDisabled: h.deblockDisabled, betaOffsetDiv2: int8(max(-6, min(h.betaOffsetDiv2, 6))), tcOffsetDiv2: int8(max(-6, min(h.tcOffsetDiv2, 6))), acrossSlices: h.acrossSlices, cbQPOffset: int16(p.cbQPOffset + h.cbQPOffset), crQPOffset: int16(p.crQPOffset + h.crQPOffset)}
	d := decoderState{sps: s, pps: p, geo: g, sf: sf, pic: pic, fi: fi, sh: h, cab: cab, cat: s.chromaArrayType, qpBDY: qpBDY, qpBDC: qpBDC, log2QG: max(s.log2CTB-int(p.diffCUQPDeltaDepth), 0), log2CQG: max(s.log2CTB-int(p.diffCUChromaQPOffsetDepth), 0), qpPred: h.qp, qpLast: h.qp, qpY: h.qp, modeY: [4]uint8{1, 1, 1, 1}, modeC: [4]uint8{1, 1, 1, 1}, firstQG: true}
	var saved savedContext
	ts := int(g.rsToTS[d.sh.segmentAddress])
	sub := 0
	for {
		if ts >= g.sizeCTBs {
			return malformed("slice runs past the end of the picture")
		}
		rs := int(g.tsToRS[ts])
		if ts == int(g.rsToTS[d.sh.segmentAddress]) && p.wppEnabled && rs%g.wCTBs == 0 {
			restoreWPP(&d, &saved, rs, g)
		}
		d.fi.sliceAddr[rs] = d.sh.sliceAddress
		d.fi.ctb[rs] = template
		if err = codingTreeUnit(&d, rs); err != nil {
			return err
		}
		if p.wppEnabled && (rs%g.wCTBs == 1 || g.wCTBs == 1) {
			saved = savedContext{row: rs / g.wCTBs, ctx: d.cab.ctx, valid: true}
		}
		end, err := d.cab.terminate()
		if err != nil {
			return err
		}
		if end != 0 {
			break
		}
		ts++
		if ts >= g.sizeCTBs {
			return malformed("slice runs past the end of the picture")
		}
		next := int(g.tsToRS[ts])
		newTile := g.tileID[next] != g.tileID[rs]
		newRow := p.wppEnabled && next%g.wCTBs == 0
		if newTile || newRow {
			if _, err = d.cab.terminate(); err != nil {
				return err
			}
			sub++
			off := d.cab.alignedPos()
			if sub < len(starts) {
				off = starts[sub]
			}
			if newRow && !newTile {
				restoreWPP(&d, &saved, next, g)
			} else {
				d.cab.initContexts(d.sh.qp)
			}
			if err = d.cab.restartAt(off); err != nil {
				return err
			}
			d.firstQG = true
			d.statCoeff = [4]int32{}
			d.qpLast = d.sh.qp
		}
	}
	return nil
}
func restoreWPP(d *decoderState, saved *savedContext, rs int, g *geometry) {
	row := rs / g.wCTBs
	if row > 0 && saved.valid {
		aboveRight := (row-1)*g.wCTBs + boolInt(g.wCTBs > 1)
		if saved.row == row-1 && g.tileID[aboveRight] == g.tileID[rs] && d.fi.sliceAddr[aboveRight] == d.sh.sliceAddress {
			d.cab.ctx = saved.ctx
			return
		}
	}
	d.cab.initContexts(d.sh.qp)
}
func substreamStarts(h *sliceHeader, r *rbsp) []int {
	starts := make([]int, 1, len(h.entryPoints)+1)
	starts[0] = h.dataOffset
	nalOff := r.rbspToNAL(h.dataOffset)
	for _, e := range h.entryPoints {
		nalOff += int(e)
		starts = append(starts, r.nalToRBSP(nalOff))
	}
	return starts
}
