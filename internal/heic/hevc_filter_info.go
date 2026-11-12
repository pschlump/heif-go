package heic

type saoCTB struct {
	typeIdx, eoClass, bandPosition uint8
	offsets                        [4]int16
}
type ctbFilter struct {
	deblockDisabled              bool
	betaOffsetDiv2, tcOffsetDiv2 int8
	acrossSlices                 bool
	cbQPOffset, crQPOffset       int16
	sao                          [3]saoCTB
}

func defaultCTBFilter() ctbFilter { return ctbFilter{deblockDisabled: true, acrossSlices: true} }

type filterInfo struct {
	bitDepthY, bitDepthC, chromaArrayType          uint8
	subW, subH, ctbLog2, picWCTBs, picHCTBs        int
	pcmLoopFilterDisabled, acrossTiles, saoEnabled bool
	ctb                                            []ctbFilter
	tileID                                         []uint16
	sliceAddr                                      []uint32
}

func (f *filterInfo) mayCross(a, b int) bool {
	if a == b {
		return true
	}
	if f.tileID[a] != f.tileID[b] && !f.acrossTiles {
		return false
	}
	if f.sliceAddr[a] != f.sliceAddr[b] && !f.ctb[b].acrossSlices {
		return false
	}
	return true
}
func (f *filterInfo) ctbOf(x, y int) int { return (y>>f.ctbLog2)*f.picWCTBs + (x >> f.ctbLog2) }
func chromaQPFromLuma(qpi int32, cat uint8) int32 {
	if cat != 1 {
		return min(qpi, 51)
	}
	if qpi < 30 {
		return qpi
	}
	if qpi > 43 {
		return qpi - 6
	}
	table := [14]int32{29, 30, 31, 32, 33, 33, 34, 34, 35, 35, 36, 36, 37, 37}
	return table[qpi-30]
}
