package heic

import "fmt"

const defaultMaxPixels = uint64(268435456)

type chromaFormat uint8

const (
	chromaMonochrome chromaFormat = iota
	chroma420
	chroma422
	chroma444
)

func chromaFromIDC(idc uint8) (chromaFormat, bool) {
	if idc > 3 {
		return 0, false
	}
	return chromaFormat(idc), true
}
func (c chromaFormat) xShift() uint32 {
	if c == chroma420 || c == chroma422 {
		return 1
	}
	return 0
}
func (c chromaFormat) yShift() uint32 {
	if c == chroma420 {
		return 1
	}
	return 0
}
func (c chromaFormat) size(width, height uint32) (uint32, uint32) {
	if c == chromaMonochrome {
		return 0, 0
	}
	xs, ys := c.xShift(), c.yShift()
	return (width + (1 << xs) - 1) >> xs, (height + (1 << ys) - 1) >> ys
}

type frame struct {
	width, height uint32
	bitDepth      uint8
	chroma        chromaFormat
	y, cb, cr     []uint16
	yStride       uint32
	cStride       uint32
}

func (f *frame) validate() error {
	if f.width == 0 || f.height == 0 {
		return malformed("decoded frame has a zero dimension")
	}
	if f.yStride < f.width {
		return malformed("luma stride is narrower than the frame")
	}
	need := uint64(f.yStride) * uint64(f.height)
	if need > uint64(len(f.y)) {
		return malformed("luma plane is shorter than its geometry")
	}
	if f.chroma == chromaMonochrome {
		return nil
	}
	cw, ch := f.chroma.size(f.width, f.height)
	if f.cStride < cw {
		return malformed("chroma stride is narrower than the frame")
	}
	need = uint64(f.cStride) * uint64(ch)
	if need > uint64(len(f.cb)) || need > uint64(len(f.cr)) {
		return malformed("chroma plane is shorter than its geometry")
	}
	return nil
}

type codecInfo struct {
	width, height uint32
	bitDepth      uint8
	chroma        chromaFormat
}

func checkPixels(width, height uint32, maxPixels uint64) (uint64, error) {
	if width == 0 || height == 0 {
		return 0, malformed("image has a zero dimension")
	}
	pixels := uint64(width) * uint64(height)
	if pixels > maxPixels {
		return 0, fmt.Errorf("image would be %d pixels, above the %d pixel limit", pixels, maxPixels)
	}
	return pixels, nil
}

type grid struct {
	rows, columns             uint32
	outputWidth, outputHeight uint32
}

func parseGrid(data []byte) (grid, error) {
	r := newReader(data)
	version, err := r.u8("grid version")
	if err != nil {
		return grid{}, err
	}
	if version != 0 {
		return grid{}, unsupported("grid version above 0")
	}
	flags, err := r.u8("grid flags")
	if err != nil {
		return grid{}, err
	}
	rows, err := r.u8("grid rows")
	if err != nil {
		return grid{}, err
	}
	columns, err := r.u8("grid columns")
	if err != nil {
		return grid{}, err
	}
	g := grid{rows: uint32(rows) + 1, columns: uint32(columns) + 1}
	if flags&1 != 0 {
		g.outputWidth, err = r.u32("grid output width")
		if err == nil {
			g.outputHeight, err = r.u32("grid output height")
		}
	} else {
		w, e := r.u16("grid output width")
		g.outputWidth, err = uint32(w), e
		if err == nil {
			h, e := r.u16("grid output height")
			g.outputHeight, err = uint32(h), e
		}
	}
	if err != nil {
		return grid{}, err
	}
	if g.outputWidth == 0 || g.outputHeight == 0 {
		return grid{}, malformed("grid declares a zero output dimension")
	}
	return g, nil
}

func (g grid) tileCount() uint64 { return uint64(g.rows) * uint64(g.columns) }

func composeGrid(g grid, tiles []frame, maxPixels uint64) (frame, error) {
	m, err := newMosaic(g, tiles)
	if err != nil {
		return frame{}, err
	}
	if _, err = checkPixels(m.width, m.height, maxPixels); err != nil {
		return frame{}, err
	}
	w, h := int(m.width), int(m.height)
	cw32, ch32 := m.chroma.size(m.width, m.height)
	cw, ch := int(cw32), int(ch32)
	out := frame{width: m.width, height: m.height, bitDepth: m.bitDepth, chroma: m.chroma, y: make([]uint16, w*h), yStride: m.width, cStride: cw32}
	for y := range h {
		if !m.lumaRow(y, out.y[y*w:(y+1)*w]) {
			return frame{}, malformed("grid tile is shorter than the row it must supply")
		}
	}
	if cw > 0 {
		out.cb, out.cr = make([]uint16, cw*ch), make([]uint16, cw*ch)
		for y := range ch {
			if !m.chromaRow(false, y, out.cb[y*cw:(y+1)*cw]) || !m.chromaRow(true, y, out.cr[y*cw:(y+1)*cw]) {
				return frame{}, malformed("grid tile is shorter than the row it must supply")
			}
		}
	}
	return out, nil
}

func exifTIFF(item []byte) ([]byte, error) {
	r := newReader(item)
	offset, err := r.u32("exif tiff header offset")
	if err != nil {
		return nil, err
	}
	if err = r.skip(int(offset), "exif payload"); err != nil {
		return nil, err
	}
	rest := r.rest()
	if len(rest) < 8 {
		return nil, truncated("exif tiff header")
	}
	valid := (rest[0] == 'I' && rest[1] == 'I' && rest[2] == 42 && rest[3] == 0) || (rest[0] == 'M' && rest[1] == 'M' && rest[2] == 0 && rest[3] == 42)
	if !valid {
		return nil, malformed("exif item does not contain a TIFF header")
	}
	return rest, nil
}
