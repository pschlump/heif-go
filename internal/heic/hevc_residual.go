package heic

var sigMap4x4 = [16]uint8{0, 1, 4, 5, 2, 3, 4, 5, 6, 6, 8, 8, 7, 7, 8, 8}
var sigPattern = [4][16]uint8{{2, 1, 1, 0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0}, {2, 2, 2, 2, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0}, {2, 1, 0, 0, 2, 1, 0, 0, 2, 1, 0, 0, 2, 1, 0, 0}, {2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}}

func residualScanIndex(d *decoderState, log2Size, cIdx int, predMode uint8) int {
	dependent := log2Size == 2 || log2Size == 3 && cIdx == 0 || log2Size == 3 && d.cat == 3
	if !dependent {
		return 0
	}
	if predMode >= 6 && predMode <= 14 {
		return 2
	}
	if predMode >= 22 && predMode <= 30 {
		return 1
	}
	return 0
}
func lastCoefficientPosition(d *decoderState, log2Size, cIdx int) (int, int) {
	ctxOffset, ctxShift := 15, log2Size-2
	if cIdx == 0 {
		ctxOffset = 3*(log2Size-2) + ((log2Size - 1) >> 2)
		ctxShift = (log2Size + 1) >> 2
	}
	cMax := (log2Size << 1) - 1
	var prefix, out [2]int
	for k, base := range []int{ctxLastX, ctxLastY} {
		v := 0
		for v < cMax {
			if d.cab.decision(base+ctxOffset+(v>>ctxShift)) == 0 {
				break
			}
			v++
		}
		prefix[k] = v
	}
	for k := range 2 {
		if prefix[k] > 3 {
			b := (prefix[k] >> 1) - 1
			suffix := int(d.cab.bypassBits(uint32(b)))
			out[k] = (1<<b)*(2+(prefix[k]&1)) + suffix
		} else {
			out[k] = prefix[k]
		}
	}
	return out[0], out[1]
}

type sigContext struct {
	table   *[16]uint8
	add, dc int
}

func newSigContext(d *decoderState, log2Size, cIdx, xs, ys, sbN, scanIdx int, tsCtx bool) sigContext {
	chroma := 0
	if cIdx > 0 {
		chroma = 27
	}
	if tsCtx {
		add := 42
		if cIdx > 0 {
			add = 43
		}
		return sigContext{table: new([16]uint8), add: add, dc: -1}
	}
	if log2Size == 2 {
		return sigContext{table: &sigMap4x4, add: chroma, dc: -1}
	}
	prev := 0
	if xs+1 < sbN {
		prev += int(d.csbf[ys*sbN+xs+1])
	}
	if ys+1 < sbN {
		prev += int(d.csbf[(ys+1)*sbN+xs]) << 1
	}
	add := chroma
	if cIdx == 0 {
		size := 21
		if log2Size == 3 {
			size = 9
			if scanIdx != 0 {
				size = 15
			}
		}
		add = 3*boolInt(xs+ys > 0) + size
	} else if log2Size == 3 {
		add += 9
	} else {
		add += 12
	}
	dc := -1
	if xs+ys == 0 {
		dc = chroma
	}
	return sigContext{table: &sigPattern[prev&3], add: add, dc: dc}
}
func (s *sigContext) at(m, raster int) int {
	if s.dc >= 0 && m == 0 {
		return s.dc
	}
	return int(s.table[raster&15]) + s.add
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func readRemaining(d *decoderState, rice uint32) (uint32, error) {
	prefix := uint32(0)
	for d.cab.bypass() == 1 {
		prefix++
		if prefix > 24 {
			return 0, malformed("coeff_abs_level_remaining prefix too long")
		}
	}
	if prefix < 3 {
		return (prefix << rice) + d.cab.bypassBits(rice), nil
	}
	extra := prefix - 3
	return d.cab.bypassBits(extra+rice) + (((1 << extra) + 3 - 1) << rice), nil
}
func residualCoding(d *decoderState, log2Size, cIdx int, predMode uint8) (bool, error) {
	n := 1 << log2Size
	for i := range n * n {
		d.coeffs[i] = 0
	}
	ts := false
	if d.pps.transformSkip && !d.tqBypass && log2Size <= d.pps.log2MaxTransformSkip {
		ctx := ctxTSChroma
		if cIdx == 0 {
			ctx = ctxTSLuma
		}
		ts = d.cab.decision(ctx) != 0
	}
	scanIdx := residualScanIndex(d, log2Size, cIdx, predMode)
	lastX, lastY := lastCoefficientPosition(d, log2Size, cIdx)
	if scanIdx == 2 {
		lastX, lastY = lastY, lastX
	}
	if lastX >= n || lastY >= n {
		return false, malformed("last significant coefficient outside block")
	}
	sbLog2 := log2Size - 2
	sbN := 1 << sbLog2
	sbScan := scanOrder(sbLog2, scanIdx)
	lastSB := scanPos(sbLog2, scanIdx, lastX>>2, lastY>>2)
	lastPos := scanPos(2, scanIdx, lastX&3, lastY&3)
	for i := range sbN * sbN {
		d.csbf[i] = 0
	}
	tsCtx := d.sps.transformSkipContext && (ts || d.tqBypass)
	sbType := boolInt(ts || d.tqBypass)
	if cIdx > 0 {
		sbType += 2
	}
	raster := &subRaster[scanIdx]
	c1 := int32(1)
	for i := lastSB; i >= 0; i-- {
		xs, ys := int(sbScan[i].x), int(sbScan[i].y)
		inferDC := false
		coded := true
		if i < lastSB && i > 0 {
			c := uint32(0)
			if xs+1 < sbN {
				c += uint32(d.csbf[ys*sbN+xs+1])
			}
			if ys+1 < sbN {
				c += uint32(d.csbf[(ys+1)*sbN+xs])
			}
			ctx := ctxCSBF + int(min(c, 1))
			if cIdx > 0 {
				ctx += 2
			}
			inferDC = true
			coded = d.cab.decision(ctx) != 0
		}
		d.csbf[ys*sbN+xs] = uint8(boolInt(coded))
		if !coded {
			continue
		}
		sig := newSigContext(d, log2Size, cIdx, xs, ys, sbN, scanIdx, tsCtx)
		var pos [16]uint8
		num, first := 0, 16
		if i == lastSB {
			pos[0] = uint8(lastPos)
			num = 1
			first = lastPos
		}
		for m := first - 1; m >= 0; m-- {
			if m > 0 || !inferDC {
				if d.cab.decision(ctxSig+sig.at(m, int(raster[m]))) != 0 {
					pos[num] = uint8(m)
					num++
					inferDC = false
				}
			} else {
				pos[num] = uint8(m)
				num++
			}
		}
		if err := decodeCoefficientLevels(d, pos[:num], i, cIdx, &c1, sbType, log2Size, scanIdx, xs, ys); err != nil {
			return false, err
		}
	}
	return ts, nil
}
func decodeCoefficientLevels(d *decoderState, sig []uint8, i, cIdx int, c1 *int32, sbType, log2Size, scanIdx, xs, ys int) error {
	ctxSet := 0
	if i > 0 && cIdx == 0 {
		ctxSet = 2
	}
	if *c1 == 0 {
		ctxSet++
	}
	*c1 = 1
	num := len(sig)
	if num == 0 {
		return nil
	}
	base := ctxGT1 + ctxSet*4
	if cIdx > 0 {
		base += 16
	}
	gt1 := uint16(0)
	lastGT1 := -1
	for k := range min(num, 8) {
		bin := d.cab.decision(base+int(min(*c1, 3))) != 0
		if bin {
			gt1 |= 1 << k
			*c1 = 0
			if lastGT1 < 0 {
				lastGT1 = k
			}
		} else if *c1 > 0 && *c1 < 3 {
			*c1++
		}
	}
	signHidden := int(sig[0])-int(sig[num-1]) > 3 && !d.tqBypass
	gt2 := false
	if lastGT1 >= 0 {
		ctx := ctxGT2 + ctxSet
		if cIdx > 0 {
			ctx += 4
		}
		gt2 = d.cab.decision(ctx) != 0
	}
	hide := d.pps.signDataHiding && signHidden
	codedSigns := num - boolInt(hide)
	signs := d.cab.bypassBits(uint32(codedSigns))
	rice := uint32(0)
	if d.sps.persistentRice {
		rice = uint32(d.statCoeff[sbType] / 4)
	}
	firstRem := true
	sumAbs := int64(0)
	posScan := subScan(scanIdx)
	n := 1 << log2Size
	for k, mv := range sig {
		m := int(mv) & 15
		baseLevel := 1 + int32((gt1>>k)&1)
		if k == lastGT1 && gt2 {
			baseLevel++
		}
		threshold := 1
		if k < 8 {
			threshold = 2
			if k == lastGT1 {
				threshold = 3
			}
		}
		level := int64(baseLevel)
		if baseLevel == int32(threshold) {
			rem, err := readRemaining(d, rice)
			if err != nil {
				return err
			}
			if firstRem && d.sps.persistentRice {
				updateStatCoeff(d, sbType, rem)
			}
			firstRem = false
			level += int64(rem)
			if level > 3<<rice {
				rice = min(rice+1, 4)
			}
		} else if level > 3<<rice {
			rice = min(rice+1, 4)
		}
		sumAbs += level
		v := level
		if k < codedSigns && (signs>>(codedSigns-1-k))&1 != 0 {
			v = -v
		}
		if hide && k == num-1 && sumAbs%2 == 1 {
			v = -v
		}
		xc := xs*4 + int(posScan[m].x)
		yc := ys*4 + int(posScan[m].y)
		d.coeffs[yc*n+xc] = int32(max(-32768, min(v, 32767)))
	}
	return nil
}
func updateStatCoeff(d *decoderState, sbType int, rem uint32) {
	s := d.statCoeff[sbType]
	if int64(rem) >= 3<<(s/4) {
		d.statCoeff[sbType] = s + 1
	} else if 2*int64(rem) < 1<<(s/4) && s > 0 {
		d.statCoeff[sbType] = s - 1
	}
}
