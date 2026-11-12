package heic

type decoderState struct {
	sps                    *sps
	pps                    *pps
	geo                    *geometry
	sf                     *scalingFactors
	pic                    *picture
	fi                     *filterInfo
	sh                     sliceHeader
	cab                    cabac
	cat                    uint8
	qpBDY, qpBDC           int32
	log2QG, log2CQG, ctbRS int
	qpPred, qpLast, qpY    int32
	qpDeltaCoded, cqoCoded bool
	cqoCB, cqoCR           int32
	tqBypass, intraSplit   bool
	modeY, modeC           [4]uint8
	cbfCB, cbfCR           [6][2]bool
	coeffs                 [1024]int32
	pred                   [1024]uint16
	csbf                   [64]uint8
	firstQG                bool
	statCoeff              [4]int32
}

func (d *decoderState) available(xc, yc, xn, yn int) bool {
	if xn < 0 || yn < 0 || xn >= d.sps.width || yn >= d.sps.height {
		return false
	}
	if d.geo.z(xn, yn) > d.geo.z(xc, yc) {
		return false
	}
	a, b := d.geo.ctbRS(xn, yn), d.geo.ctbRS(xc, yc)
	return d.geo.tileID[a] == d.geo.tileID[b] && d.fi.sliceAddr[a] == d.fi.sliceAddr[b]
}
func (d *decoderState) modeAt(x, y int) uint8 { return d.pic.intraMode[d.pic.idx4(x, y)] }
func (d *decoderState) startQG(x, y int, first bool) {
	d.qpDeltaCoded = false
	prev := d.qpLast
	if first {
		prev = d.sh.qp
	}
	a, b := prev, prev
	if x > 0 && d.available(x, y, x-1, y) && d.geo.ctbRS(x-1, y) == d.geo.ctbRS(x, y) {
		a = int32(d.pic.qpY[d.pic.idx4(x-1, y)])
	}
	if y > 0 && d.available(x, y, x, y-1) && d.geo.ctbRS(x, y-1) == d.geo.ctbRS(x, y) {
		b = int32(d.pic.qpY[d.pic.idx4(x, y-1)])
	}
	d.qpPred = (a + b + 1) >> 1
	d.setQPDelta(0)
}
func positiveMod(v, m int32) int32 {
	v %= m
	if v < 0 {
		v += m
	}
	return v
}
func (d *decoderState) setQPDelta(delta int32) {
	m := 52 + d.qpBDY
	d.qpY = positiveMod(d.qpPred+delta+52+2*d.qpBDY, m) - d.qpBDY
}
func (d *decoderState) qpLuma() int32 { return d.qpY + d.qpBDY }
func (d *decoderState) qpChroma(cIdx int) int32 {
	ppsOff, sliceOff, cuOff := d.pps.crQPOffset, d.sh.crQPOffset, d.cqoCR
	if cIdx == 1 {
		ppsOff, sliceOff, cuOff = d.pps.cbQPOffset, d.sh.cbQPOffset, d.cqoCB
	}
	qpi := max(-d.qpBDC, min(d.qpY+ppsOff+sliceOff+cuOff, 57))
	return chromaQPFromLuma(qpi, d.cat) + d.qpBDC
}
func (d *decoderState) storeQP(x, y, size int) {
	v := int8(max(-128, min(d.qpY, 127)))
	x0, y0, n, w := x>>2, y>>2, size>>2, d.pic.min4W
	for j := range n {
		for i := range n {
			if idx := (y0+j)*w + x0 + i; idx < len(d.pic.qpY) {
				d.pic.qpY[idx] = v
			}
		}
	}
	d.qpLast = d.qpY
}
func (d *decoderState) factors(log2Size, cIdx int, ts bool) []uint8 {
	if d.sf == nil || ts && log2Size > 2 {
		return nil
	}
	return d.sf.get(log2Size, cIdx)
}
