package heic

import (
	"encoding/binary"
	"runtime"
	"sync"
)

type colorMode uint8

const (
	colorLuma colorMode = iota
	colorIdentity
	colorMatrix
)

type coeffs struct {
	shift                                               uint
	round, maxOut, yOffset, cOffset, ky, rv, gu, gv, bu int32
}

func quantize(v float32) int32 {
	if v < 0 {
		return int32(v - .5)
	}
	return int32(v + .5)
}
func sampleMax(depth uint8) int32 { return int32((uint32(1) << depth) - 1) }
func matrixWeights(m matrixCoefficients) (float32, float32) {
	switch m {
	case matrixBT601:
		return .299, .114
	case matrix240M:
		return .212, .087
	case matrixBT2020:
		return .2627, .0593
	default:
		return .2126, .0722
	}
}
func newCoeffs(n nclx, depth uint8, wide bool) coeffs {
	shift := uint(16)
	maxOut := int32(255)
	if wide {
		shift = 13
		maxOut = 65535
	}
	unit := float32(maxOut) * float32(uint32(1)<<shift)
	max := float32(sampleMax(depth))
	var yOff, yGain, cOff, cGain float32
	if n.sampleRange == rangeFull {
		yOff = 0
		yGain = 1 / max
		cOff = float32(uint16(1) << (depth - 1))
		cGain = 1 / max
	} else {
		step := float32(uint32(1) << (depth - 8))
		yOff = 16 * step
		yGain = 1 / (219 * step)
		cOff = 128 * step
		cGain = 1 / (224 * step)
	}
	kr, kb := matrixWeights(n.matrix)
	kg := 1 - kr - kb
	return coeffs{shift: shift, round: 1 << (shift - 1), maxOut: maxOut, yOffset: int32(yOff), cOffset: int32(cOff), ky: quantize(yGain * unit), rv: quantize(2 * (1 - kr) * cGain * unit), gu: quantize(-2 * kb * (1 - kb) / kg * cGain * unit), gv: quantize(-2 * kr * (1 - kr) / kg * cGain * unit), bu: quantize(2 * (1 - kb) * cGain * unit)}
}
func (c *coeffs) finish(acc int32) int32 {
	v := (acc + c.round) >> c.shift
	if v < 0 {
		return 0
	}
	if v > c.maxOut {
		return c.maxOut
	}
	return v
}

type rowSource struct {
	frame  *frame
	mosaic *mosaic
}

func (s *rowSource) width() uint32 {
	if s.frame != nil {
		return s.frame.width
	}
	return s.mosaic.width
}
func (s *rowSource) height() uint32 {
	if s.frame != nil {
		return s.frame.height
	}
	return s.mosaic.height
}
func (s *rowSource) chroma() chromaFormat {
	if s.frame != nil {
		return s.frame.chroma
	}
	return s.mosaic.chroma
}
func (s *rowSource) bitDepth() uint8 {
	if s.frame != nil {
		return s.frame.bitDepth
	}
	return s.mosaic.bitDepth
}
func (s *rowSource) chromaWidth() int {
	cw, _ := s.chroma().size(s.width(), s.height())
	return int(cw)
}
func (s *rowSource) luma(y int, buf []uint16) ([]uint16, bool) {
	if s.frame != nil {
		f := s.frame
		start := y * int(f.yStride)
		if start < 0 || start+int(f.width) > len(f.y) {
			return nil, false
		}
		return f.y[start : start+int(f.width)], true
	}
	if len(buf) < int(s.width()) {
		return nil, false
	}
	row := buf[:s.width()]
	return row, s.mosaic.lumaRow(y, row)
}
func (s *rowSource) chromaRow(cr bool, r int, buf []uint16) ([]uint16, bool) {
	cw := s.chromaWidth()
	if s.frame != nil {
		f := s.frame
		start := r * int(f.cStride)
		plane := f.cb
		if cr {
			plane = f.cr
		}
		if start < 0 || start+cw > len(plane) {
			return nil, false
		}
		return plane[start : start+cw], true
	}
	if len(buf) < cw {
		return nil, false
	}
	row := buf[:cw]
	return row, s.mosaic.chromaRow(cr, r, row)
}

type chromaRing struct {
	keys  [3]int
	rows  []uint16
	width int
}

func newChromaRing(rows []uint16, width int) chromaRing {
	return chromaRing{keys: [3]int{-1, -1, -1}, rows: rows, width: width}
}
func (r *chromaRing) slot(src *rowSource, row int, xShift uint32, tmp []uint16) (int, bool) {
	for i, k := range r.keys {
		if k == row {
			return i, true
		}
	}
	s, best := 0, -1
	for i, k := range r.keys {
		d := int(^uint(0) >> 1)
		if k >= 0 {
			d = k - row
			if d < 0 {
				d = -d
			}
		}
		if d > best {
			s, best = i, d
		}
	}
	w := r.width
	slot := r.rows[s*2*w : (s+1)*2*w]
	cb, cr := slot[:w], slot[w:]
	b, ok := src.chromaRow(false, row, tmp)
	if !ok {
		return 0, false
	}
	upsampleHorizontal(cb, b, xShift)
	v, ok := src.chromaRow(true, row, tmp)
	if !ok {
		return 0, false
	}
	upsampleHorizontal(cr, v, xShift)
	r.keys[s] = row
	return s, true
}
func (r *chromaRing) get(s int) ([]uint16, []uint16) {
	w := r.width
	v := r.rows[s*2*w : (s+1)*2*w]
	return v[:w], v[w:]
}

func convertFrame(f *frame, alpha *frame, n nclx, layout pixelLayout, maxPixels uint64) (decodedImage, error) {
	if err := f.validate(); err != nil {
		return decodedImage{}, err
	}
	return convertSource(&rowSource{frame: f}, alpha, n, layout, maxPixels)
}
func convertSource(src *rowSource, alpha *frame, n nclx, layout pixelLayout, maxPixels uint64) (decodedImage, error) {
	depth := src.bitDepth()
	if depth < 8 || depth > 16 {
		return decodedImage{}, unsupported("sample bit depth is outside 8 to 16")
	}
	if _, err := checkPixels(src.width(), src.height(), maxPixels); err != nil {
		return decodedImage{}, err
	}
	img, err := newDecodedImage(src.width(), src.height(), layout, maxPixels)
	if err != nil {
		return img, err
	}
	wide := layout.bytesPerChannel() == 2
	c := newCoeffs(n, depth, wide)
	mode := colorMatrix
	if src.chroma() == chromaMonochrome || layout == layoutGray8 {
		mode = colorLuma
	} else if n.matrix == matrixIdentity {
		mode = colorIdentity
	}
	convertBands(src, &c, mode, img.data, layout)
	if alpha != nil && layout.hasAlpha() {
		if alpha.bitDepth < 8 || alpha.bitDepth > 16 {
			return img, unsupported("sample bit depth is outside 8 to 16")
		}
		applyAlpha(img.data, alpha, int(img.width), int(img.height), layout.bytesPerPixel(), c.maxOut)
	}
	return img, nil
}
func colorScratchLen(src *rowSource, mode colorMode) int {
	n := 0
	if mode != colorLuma {
		n = 6 * int(src.width())
	}
	if src.mosaic != nil {
		n += int(src.width()) + src.chromaWidth()
	}
	return n
}

func convertBands(src *rowSource, c *coeffs, mode colorMode, out []byte, layout pixelLayout) {
	w, bpp, h := int(src.width()), layout.bytesPerPixel(), int(src.height())
	workers := 1
	if uint64(w)*uint64(h) >= 768*768 {
		workers = min(runtime.GOMAXPROCS(0), (h+15)/16)
	}
	if workers <= 1 {
		convertBand(src, c, mode, out, layout, w, bpp, 0, h, make([]uint16, colorScratchLen(src, mode)))
		return
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			scratch := make([]uint16, colorScratchLen(src, mode))
			for y0 := range jobs {
				y1 := min(y0+16, h)
				convertBand(src, c, mode, out[y0*w*bpp:y1*w*bpp], layout, w, bpp, y0, y1, scratch)
			}
		}()
	}
	for y0 := 0; y0 < h; y0 += 16 {
		jobs <- y0
	}
	close(jobs)
	wg.Wait()
}
func convertBand(src *rowSource, c *coeffs, mode colorMode, out []byte, layout pixelLayout, w, bpp, y0, y1 int, scratch []uint16) {
	ringLen := 0
	if mode != colorLuma {
		ringLen = 6 * w
	}
	ring := newChromaRing(scratch[:ringLen], w)
	stitch := scratch[ringLen:]
	lumaBuf, chromaBuf := stitch[:min(w, len(stitch))], stitch[min(w, len(stitch)):]
	ch := 0
	_, ch32 := src.chroma().size(src.width(), src.height())
	ch = int(ch32)
	xShift := src.chroma().xShift()
	for bandRow, y := 0, y0; y < y1; bandRow, y = bandRow+1, y+1 {
		dst := out[bandRow*w*bpp : (bandRow+1)*w*bpp]
		luma, ok := src.luma(y, lumaBuf)
		if !ok {
			return
		}
		if mode == colorLuma {
			convertLumaRow(dst, luma, c, layout)
			continue
		}
		r0, r1, weight := chromaRowPair(y, ch, src.chroma())
		s0, ok := ring.slot(src, r0, xShift, chromaBuf)
		if !ok {
			return
		}
		s1, ok := ring.slot(src, r1, xShift, chromaBuf)
		if !ok {
			return
		}
		cb0, cr0 := ring.get(s0)
		cb1, cr1 := ring.get(s1)
		if mode == colorIdentity {
			convertIdentityRow(dst, luma, cb0, cb1, cr0, cr1, weight, c, layout)
		} else {
			convertMatrixRow(dst, luma, cb0, cb1, cr0, cr1, weight, c, layout)
		}
	}
}
func storePixel(dst []byte, layout pixelLayout, r, g, b, a int32) {
	first, third := r, b
	if layout.isBGR() {
		first, third = b, r
	}
	if layout.bytesPerChannel() == 2 {
		binary.LittleEndian.PutUint16(dst[0:2], uint16(first))
		binary.LittleEndian.PutUint16(dst[2:4], uint16(g))
		binary.LittleEndian.PutUint16(dst[4:6], uint16(third))
		if layout.hasAlpha() {
			binary.LittleEndian.PutUint16(dst[6:8], uint16(a))
		}
		return
	}
	dst[0], dst[1], dst[2] = byte(first), byte(g), byte(third)
	if layout.hasAlpha() {
		dst[3] = byte(a)
	}
}
func convertLumaRow(dst []byte, luma []uint16, c *coeffs, layout pixelLayout) {
	bpp := layout.bytesPerPixel()
	for x, y := range luma {
		v := c.finish((int32(y) - c.yOffset) * c.ky)
		storePixel(dst[x*bpp:(x+1)*bpp], layout, v, v, v, c.maxOut)
	}
}
func convertIdentityRow(dst []byte, luma, cb0, cb1, cr0, cr1 []uint16, w0 uint32, c *coeffs, layout pixelLayout) {
	bpp := layout.bytesPerPixel()
	for x, y := range luma {
		u, v := verticalBlend(cb0[x], cb1[x], w0), verticalBlend(cr0[x], cr1[x], w0)
		g := c.finish((int32(y) - c.yOffset) * c.ky)
		b := c.finish((int32(u) - c.yOffset) * c.ky)
		r := c.finish((int32(v) - c.yOffset) * c.ky)
		storePixel(dst[x*bpp:(x+1)*bpp], layout, r, g, b, c.maxOut)
	}
}
func convertMatrixRow(dst []byte, luma, cb0, cb1, cr0, cr1 []uint16, w0 uint32, c *coeffs, layout pixelLayout) {
	if layout == layoutRGB8 {
		convertMatrixRowRGB8(dst, luma, cb0, cb1, cr0, cr1, w0, c)
		return
	}
	if layout == layoutRGBA8 {
		convertMatrixRowRGBA8(dst, luma, cb0, cb1, cr0, cr1, w0, c)
		return
	}
	bpp := layout.bytesPerPixel()
	for x, y := range luma {
		u, v := int32(verticalBlend(cb0[x], cb1[x], w0))-c.cOffset, int32(verticalBlend(cr0[x], cr1[x], w0))-c.cOffset
		yy := (int32(y) - c.yOffset) * c.ky
		r := c.finish(yy + c.rv*v)
		g := c.finish(yy + c.gu*u + c.gv*v)
		b := c.finish(yy + c.bu*u)
		storePixel(dst[x*bpp:(x+1)*bpp], layout, r, g, b, c.maxOut)
	}
}
func convertMatrixRowRGB8(dst []byte, luma, cb0, cb1, cr0, cr1 []uint16, w0 uint32, c *coeffs) {
	for x, y := range luma {
		u := int32(verticalBlend(cb0[x], cb1[x], w0)) - c.cOffset
		v := int32(verticalBlend(cr0[x], cr1[x], w0)) - c.cOffset
		yy := (int32(y) - c.yOffset) * c.ky
		dst[3*x] = byte(c.finish(yy + c.rv*v))
		dst[3*x+1] = byte(c.finish(yy + c.gu*u + c.gv*v))
		dst[3*x+2] = byte(c.finish(yy + c.bu*u))
	}
}

func convertMatrixRowRGBA8(dst []byte, luma, cb0, cb1, cr0, cr1 []uint16, w0 uint32, c *coeffs) {
	for x, y := range luma {
		u := int32(verticalBlend(cb0[x], cb1[x], w0)) - c.cOffset
		v := int32(verticalBlend(cr0[x], cr1[x], w0)) - c.cOffset
		yy := (int32(y) - c.yOffset) * c.ky
		dst[4*x] = byte(c.finish(yy + c.rv*v))
		dst[4*x+1] = byte(c.finish(yy + c.gu*u + c.gv*v))
		dst[4*x+2] = byte(c.finish(yy + c.bu*u))
		dst[4*x+3] = byte(c.maxOut)
	}
}

func applyAlpha(out []byte, f *frame, w, h, bpp int, maxOut int32) {
	maxIn := sampleMax(f.bitDepth)
	gain := quantize(float32(maxOut) * float32(uint32(1)<<14) / float32(maxIn))
	off := bpp - 1
	wide := bpp == 8
	if wide {
		off = 6
	}
	for y := range h {
		dst := out[y*w*bpp : (y+1)*w*bpp]
		src := f.y[y*int(f.yStride):]
		for x := range min(w, len(src)) {
			v := min(int32(src[x]), maxIn)
			a := (v*gain + (1 << 13)) >> 14
			a = max(0, min(a, maxOut))
			if wide {
				binary.LittleEndian.PutUint16(dst[x*bpp+off:x*bpp+off+2], uint16(a))
			} else {
				dst[x*bpp+off] = byte(a)
			}
		}
	}
}
