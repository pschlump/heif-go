package heic

func codingQuadtree(d *decoderState, x0, y0, log2Size int, depth uint8) error {
	size := 1 << log2Size
	fits := x0+size <= d.sps.width && y0+size <= d.sps.height
	split := log2Size > d.sps.log2MinCB
	if fits && split {
		inc := 0
		if x0 > 0 && d.available(x0, y0, x0-1, y0) && d.pic.ctDepth[d.pic.idx4(x0-1, y0)] > depth {
			inc++
		}
		if y0 > 0 && d.available(x0, y0, x0, y0-1) && d.pic.ctDepth[d.pic.idx4(x0, y0-1)] > depth {
			inc++
		}
		split = d.cab.decision(ctxSplitCU+inc) != 0
	}
	if d.pps.cuQPDeltaEnabled && log2Size >= d.log2QG {
		first := d.firstQG
		d.startQG(x0, y0, first)
		d.firstQG = false
	}
	if d.pps.chromaQPOffsetListEnabled && log2Size >= d.log2CQG {
		d.cqoCoded = false
		d.cqoCB, d.cqoCR = 0, 0
	}
	if split {
		half := size >> 1
		for _, p := range [][2]int{{0, 0}, {half, 0}, {0, half}, {half, half}} {
			x, y := x0+p[0], y0+p[1]
			if x < d.sps.width && y < d.sps.height {
				if err := codingQuadtree(d, x, y, log2Size-1, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return codingUnit(d, x0, y0, log2Size, depth)
}
func codingUnit(d *decoderState, x0, y0, log2Size int, depth uint8) error {
	size := 1 << log2Size
	fill4(d.pic.ctDepth, d.pic.min4W, x0, y0, size, depth)
	d.tqBypass = false
	if d.pps.transquantBypass {
		d.tqBypass = d.cab.decision(ctxTQBypass) != 0
	}
	d.intraSplit = log2Size == d.sps.log2MinCB && d.cab.decision(ctxPartMode) == 0
	flags := uint8(0)
	if d.tqBypass {
		flags |= flagTQBypass
	}
	d.pic.markEdge(x0, y0, size, edgeVer|edgeHor)
	pcmPossible := !d.intraSplit && d.sps.pcmEnabled && log2Size >= d.sps.log2MinPCMCB && log2Size <= d.sps.log2MaxPCMCB
	if pcmPossible {
		v, err := d.cab.terminate()
		if err != nil {
			return err
		}
		if v != 0 {
			flags |= flagPCM
			fill4(d.pic.cuFlags, d.pic.min4W, x0, y0, size, flags)
			fill4(d.pic.intraMode, d.pic.min4W, x0, y0, size, 1)
			if err = readPCM(d, x0, y0, size); err != nil {
				return err
			}
			d.storeQP(x0, y0, size)
			return nil
		}
	}
	fill4(d.pic.cuFlags, d.pic.min4W, x0, y0, size, flags)
	if err := lumaPredictionMode(d, x0, y0, log2Size); err != nil {
		return err
	}
	if err := chromaPredictionMode(d); err != nil {
		return err
	}
	maxDepth := d.sps.maxTrDepthIntra + uint32(boolInt(d.intraSplit))
	d.cbfCB = [6][2]bool{}
	d.cbfCR = [6][2]bool{}
	if err := transformTree(d, x0, y0, x0, y0, log2Size, 0, 0, maxDepth); err != nil {
		return err
	}
	d.storeQP(x0, y0, size)
	return nil
}
func readPCM(d *decoderState, x0, y0, size int) error {
	d.cab.alignAfterTerminate()
	shiftY := d.sps.bitDepthY - d.sps.pcmBitDepthY
	for j := range size {
		for i := range size {
			v, err := d.cab.rawBits(uint32(d.sps.pcmBitDepthY))
			if err != nil {
				return err
			}
			if x0+i < d.pic.y.width && y0+j < d.pic.y.height {
				d.pic.y.put(x0+i, y0+j, uint16(v)<<shiftY)
			}
		}
	}
	if d.cat != 0 {
		sw, sh := d.sps.subW, d.sps.subH
		cw, ch := size/sw, size/sh
		shiftC := d.sps.bitDepthC - d.sps.pcmBitDepthC
		for c := range 2 {
			for j := range ch {
				for i := range cw {
					v, err := d.cab.rawBits(uint32(d.sps.pcmBitDepthC))
					if err != nil {
						return err
					}
					px, py := x0/sw+i, y0/sh+j
					p := &d.pic.cb
					if c == 1 {
						p = &d.pic.cr
					}
					if px < p.width && py < p.height {
						p.put(px, py, uint16(v)<<shiftC)
					}
				}
			}
		}
	}
	return d.cab.initEngine()
}
func codingTreeUnit(d *decoderState, ctbRS int) error {
	log2 := d.sps.log2CTB
	x, y := (ctbRS%d.geo.wCTBs)<<log2, (ctbRS/d.geo.wCTBs)<<log2
	d.ctbRS = ctbRS
	if d.sh.saoLuma || d.sh.saoChroma {
		decodeSAOSyntax(d, ctbRS)
	}
	if x >= d.sps.width || y >= d.sps.height {
		return malformed("coding tree block outside the picture")
	}
	return codingQuadtree(d, x, y, log2, 0)
}

var map422 = [35]uint8{0, 1, 2, 2, 2, 2, 3, 5, 7, 8, 10, 12, 13, 15, 16, 18, 20, 22, 23, 25, 27, 29, 31, 33, 34, 34, 34, 34, 34, 34, 34, 34, 34, 34, 34}

func lumaPredictionMode(d *decoderState, x0, y0, log2Size int) error {
	size := 1 << log2Size
	n := 1
	if d.intraSplit {
		n = 2
	}
	offPB := size / n
	var prev [4]bool
	var sel [4]uint32
	for k := range n * n {
		prev[k] = d.cab.decision(ctxPrevIntra) != 0
	}
	for k := range n * n {
		if prev[k] {
			if d.cab.bypass() == 1 {
				sel[k] = 1 + d.cab.bypass()
			}
		} else {
			sel[k] = d.cab.bypassBits(5)
		}
	}
	for k := range n * n {
		x, y := x0+(k%n)*offPB, y0+(k/n)*offPB
		cands := predictionCandidates(d, x, y)
		mode := uint8(0)
		if prev[k] {
			mode = cands[min(sel[k], 2)]
		} else {
			if cands[0] > cands[1] {
				cands[0], cands[1] = cands[1], cands[0]
			}
			if cands[1] > cands[2] {
				cands[1], cands[2] = cands[2], cands[1]
			}
			if cands[0] > cands[1] {
				cands[0], cands[1] = cands[1], cands[0]
			}
			m := uint8(sel[k])
			for _, cand := range cands {
				if m >= cand {
					m++
				}
			}
			mode = min(m, 34)
		}
		d.modeY[k] = mode
		fill4(d.pic.intraMode, d.pic.min4W, x, y, offPB, mode)
	}
	return nil
}
func predictionCandidates(d *decoderState, x, y int) [3]uint8 {
	ctbMask := (1 << d.sps.log2CTB) - 1
	a := neighbourPredictionMode(d, x, y, x-1, y, false)
	b := neighbourPredictionMode(d, x, y, x, y-1, y&ctbMask == 0)
	if a == b {
		if a < 2 {
			return [3]uint8{0, 1, 26}
		}
		return [3]uint8{a, 2 + (a+29)%32, 2 + (a-2+1)%32}
	}
	third := uint8(26)
	if a != 0 && b != 0 {
		third = 0
	} else if a != 1 && b != 1 {
		third = 1
	}
	return [3]uint8{a, b, third}
}
func neighbourPredictionMode(d *decoderState, x, y, nx, ny int, aboveCTB bool) uint8 {
	if aboveCTB || !d.available(x, y, nx, ny) {
		return 1
	}
	if d.pic.cuFlags[d.pic.idx4(nx, ny)]&flagPCM != 0 {
		return 1
	}
	return d.modeAt(nx, ny)
}
func chromaPredictionMode(d *decoderState) error {
	if d.cat == 0 {
		return nil
	}
	n := 1
	if d.cat == 3 && d.intraSplit {
		n = 4
	}
	for k := range n {
		raw := uint32(4)
		if d.cab.decision(ctxIntraChroma) != 0 {
			raw = d.cab.bypassBits(2)
		}
		idx := 0
		if n == 4 {
			idx = k
		}
		luma := d.modeY[idx]
		m := luma
		switch raw {
		case 0:
			m = 0
		case 1:
			m = 26
		case 2:
			m = 10
		case 3:
			m = 1
		}
		if raw != 4 && m == luma {
			m = 34
		}
		if d.cat == 2 {
			m = map422[m]
		}
		d.modeC[k] = m
	}
	if n == 1 {
		d.modeC[1], d.modeC[2], d.modeC[3] = d.modeC[0], d.modeC[0], d.modeC[0]
	}
	return nil
}
func transformTree(d *decoderState, x0, y0, xBase, yBase, log2Size int, depth uint32, blkIdx int, maxDepth uint32) error {
	dp := int(depth)
	forced := log2Size > d.sps.log2MaxTB || d.intraSplit && depth == 0
	split := forced
	if log2Size <= d.sps.log2MaxTB && log2Size > d.sps.log2MinTB && depth < maxDepth && !(d.intraSplit && depth == 0) {
		split = d.cab.decision(ctxSplitTransform+5-log2Size) != 0
	}
	if log2Size > 2 && d.cat != 0 {
		extra := d.cat == 2 && (!split || log2Size == 3)
		for c := range 2 {
			parent := true
			if depth > 0 {
				if c == 0 {
					parent = d.cbfCB[dp-1][0]
				} else {
					parent = d.cbfCR[dp-1][0]
				}
			}
			var v [2]bool
			if parent {
				v[0] = d.cab.decision(ctxCBFChroma+dp) != 0
				if extra {
					v[1] = d.cab.decision(ctxCBFChroma+dp) != 0
				}
			}
			if c == 0 {
				d.cbfCB[dp] = v
			} else {
				d.cbfCR[dp] = v
			}
		}
	} else if log2Size == 2 && d.cat != 0 && depth > 0 {
		d.cbfCB[dp] = d.cbfCB[dp-1]
		d.cbfCR[dp] = d.cbfCR[dp-1]
	}
	if split {
		half := 1 << (log2Size - 1)
		for i, p := range [][2]int{{0, 0}, {half, 0}, {0, half}, {half, half}} {
			if err := transformTree(d, x0+p[0], y0+p[1], x0, y0, log2Size-1, depth+1, i, maxDepth); err != nil {
				return err
			}
		}
		return nil
	}
	cbfLuma := d.cab.decision(ctxCBFLuma+boolInt(depth == 0)) != 0
	return transformUnit(d, x0, y0, xBase, yBase, log2Size, depth, blkIdx, cbfLuma)
}
func transformUnit(d *decoderState, x0, y0, xBase, yBase, log2Size int, depth uint32, blkIdx int, cbfLuma bool) error {
	dp := int(depth)
	small := d.cat != 3 && log2Size == 2
	log2C := max(log2Size, 3) - 1
	if d.cat == 3 {
		log2C = log2Size
	}
	cbfDepthC := dp
	if small {
		cbfDepthC = dp - 1
	}
	xc, yc := x0, y0
	if small {
		xc, yc = xBase, yBase
	}
	nChroma := 1
	if d.cat == 2 {
		nChroma = 2
	}
	chromaHere := d.cat != 0 && (!small || blkIdx == 3)
	cbfChroma := false
	if d.cat != 0 {
		for t := range nChroma {
			cbfChroma = cbfChroma || d.cbfCB[cbfDepthC][t] || d.cbfCR[cbfDepthC][t]
		}
	}
	if cbfLuma || cbfChroma {
		if d.pps.cuQPDeltaEnabled && !d.qpDeltaCoded {
			delta := readQPDelta(d)
			d.qpDeltaCoded = true
			d.setQPDelta(delta)
		}
		if d.sh.cuChromaQPOffsetEnabled && cbfChroma && !d.tqBypass && !d.cqoCoded {
			readChromaQPOffset(d)
		}
	}
	d.pic.markEdge(x0, y0, 1<<log2Size, edgeVer|edgeHor)
	modeY := d.modeAt(x0, y0)
	if cbfLuma {
		ts, err := residualCoding(d, log2Size, 0, modeY)
		if err != nil {
			return err
		}
		if err = reconstructBlock(d, x0, y0, log2Size, 0, modeY, true, ts); err != nil {
			return err
		}
	} else {
		if err := reconstructBlock(d, x0, y0, log2Size, 0, modeY, false, false); err != nil {
			return err
		}
	}
	if chromaHere {
		modeC := d.modeC[0]
		if d.cat == 3 && d.intraSplit {
			modeC = d.modeC[blkIdx]
		}
		sw, sh := d.sps.subW, d.sps.subH
		cx, cy := xc/sw, yc/sh
		for c := 1; c < 3; c++ {
			for t := range nChroma {
				cbf := d.cbfCR[cbfDepthC][t]
				if c == 1 {
					cbf = d.cbfCB[cbfDepthC][t]
				}
				y := cy + (t << log2C)
				if cbf {
					ts, err := residualCoding(d, log2C, c, modeC)
					if err != nil {
						return err
					}
					if err = reconstructBlock(d, cx, y, log2C, c, modeC, true, ts); err != nil {
						return err
					}
				} else {
					if err := reconstructBlock(d, cx, y, log2C, c, modeC, false, false); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
func readQPDelta(d *decoderState) int32 {
	prefix := uint32(0)
	for prefix < 5 {
		ctx := ctxCUQPDelta + boolInt(prefix > 0)
		if d.cab.decision(ctx) == 0 {
			break
		}
		prefix++
	}
	v := prefix
	if prefix == 5 {
		k := uint32(0)
		for d.cab.bypass() == 1 && k < 24 {
			k++
		}
		v = 5 + (1 << k) - 1 + d.cab.bypassBits(k)
	}
	if v == 0 {
		return 0
	}
	if d.cab.bypass() == 1 {
		return -int32(v)
	}
	return int32(v)
}
func readChromaQPOffset(d *decoderState) {
	d.cqoCoded = true
	if d.cab.decision(ctxChromaQPFlag) == 0 {
		d.cqoCB, d.cqoCR = 0, 0
		return
	}
	idx := 0
	for idx+1 < len(d.pps.cbQPOffsetList) {
		if d.cab.decision(ctxChromaQPIdx) == 0 {
			break
		}
		idx++
	}
	if idx < len(d.pps.cbQPOffsetList) {
		d.cqoCB = d.pps.cbQPOffsetList[idx]
	}
	if idx < len(d.pps.crQPOffsetList) {
		d.cqoCR = d.pps.crQPOffsetList[idx]
	}
}
