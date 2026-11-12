package heic

type mosaic struct {
	grid          grid
	tiles         []frame
	tileW, tileH  uint32
	chroma        chromaFormat
	bitDepth      uint8
	width, height uint32
}

func newMosaic(g grid, tiles []frame) (mosaic, error) {
	if uint64(len(tiles)) != g.tileCount() {
		return mosaic{}, malformed("the grid's tile count disagrees with the number of decoded tiles")
	}
	if len(tiles) == 0 {
		return mosaic{}, malformed("grid has no tiles")
	}
	m := mosaic{grid: g, tiles: tiles, tileW: tiles[0].width, tileH: tiles[0].height, chroma: tiles[0].chroma, bitDepth: tiles[0].bitDepth, width: g.outputWidth, height: g.outputHeight}
	for i := range tiles {
		t := &tiles[i]
		if err := t.validate(); err != nil {
			return mosaic{}, err
		}
		if t.width != m.tileW || t.height != m.tileH {
			return mosaic{}, malformed("grid tiles are not all the same size")
		}
		if t.chroma != m.chroma || t.bitDepth != m.bitDepth {
			return mosaic{}, malformed("grid tiles do not share a pixel format")
		}
	}
	if uint64(m.tileW)*uint64(g.columns) < uint64(g.outputWidth) || uint64(m.tileH)*uint64(g.rows) < uint64(g.outputHeight) {
		return mosaic{}, malformed("grid tiles do not cover the declared output size")
	}
	xs, ys := m.chroma.xShift(), m.chroma.yShift()
	if m.chroma != chromaMonochrome && !((g.columns == 1 || (m.tileW>>xs)<<xs == m.tileW) && (g.rows == 1 || (m.tileH>>ys)<<ys == m.tileH)) {
		return mosaic{}, unsupported("grid tile origin is not on a chroma sample")
	}
	return m, nil
}
func (m *mosaic) chromaSize() (uint32, uint32) { return m.chroma.size(m.width, m.height) }
func (m *mosaic) lumaRow(y int, dst []uint16) bool {
	return m.stitch(y, int(m.height), int(m.tileW), int(m.tileH), dst, false, false)
}
func (m *mosaic) chromaRow(cr bool, y int, dst []uint16) bool {
	cw, ch := m.chroma.size(m.tileW, m.tileH)
	_, rows := m.chromaSize()
	return m.stitch(y, int(rows), int(cw), int(ch), dst, true, cr)
}
func (m *mosaic) stitch(y, rows, tileW, tileH int, dst []uint16, chroma, cr bool) bool {
	if y < 0 || y >= rows || tileW == 0 || tileH == 0 {
		return false
	}
	row, local := y/tileH, y%tileH
	columns := int(m.grid.columns)
	first := row * columns
	x := 0
	for col := range columns {
		if x >= len(dst) {
			break
		}
		n := min(tileW, len(dst)-x)
		t := &m.tiles[first+col]
		var samples []uint16
		stride := int(t.yStride)
		if chroma {
			stride = int(t.cStride)
			if cr {
				samples = t.cr
			} else {
				samples = t.cb
			}
		} else {
			samples = t.y
		}
		s := local * stride
		if s < 0 || s+n > len(samples) {
			return false
		}
		copy(dst[x:x+n], samples[s:s+n])
		x += n
	}
	return true
}
