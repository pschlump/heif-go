package heic

import "math"

type geometry struct {
	wCTBs, hCTBs, sizeCTBs, ctbLog2 int
	colWidth, rowHeight             []int
	rsToTS, tsToRS                  []uint32
	tileID                          []uint16
	zs                              []uint32
	minTBW, minTBH, minTBLog2       int
}

func tileSpacing(total, n int, uniform bool, explicit []int) ([]int, error) {
	out := make([]int, n)
	if uniform {
		for i := range n {
			out[i] = (i+1)*total/n - i*total/n
		}
	} else {
		if len(explicit)+1 != n {
			return nil, malformed("tile spacing list has the wrong length")
		}
		used := 0
		for i := range n - 1 {
			out[i] = explicit[i]
			used += explicit[i]
		}
		if used >= total {
			return nil, malformed("explicit tile spacing exceeds the picture")
		}
		out[n-1] = total - used
	}
	for _, v := range out {
		if v == 0 {
			return nil, malformed("zero-sized tile")
		}
	}
	return out, nil
}
func newGeometry(s *sps, p *pps) (geometry, error) {
	g := geometry{wCTBs: s.widthInCTBs(), hCTBs: s.heightInCTBs(), ctbLog2: s.log2CTB, minTBLog2: s.log2MinTB}
	g.sizeCTBs = g.wCTBs * g.hCTBs
	cols, rows := p.numTileCols, p.numTileRows
	if cols > g.wCTBs || rows > g.hCTBs {
		return g, malformed("more tiles than coding tree blocks")
	}
	var err error
	g.colWidth, err = tileSpacing(g.wCTBs, cols, p.uniformSpacing, p.columnWidths)
	if err != nil {
		return g, err
	}
	g.rowHeight, err = tileSpacing(g.hCTBs, rows, p.uniformSpacing, p.rowHeights)
	if err != nil {
		return g, err
	}
	colBD, rowBD := make([]int, cols+1), make([]int, rows+1)
	for i := range cols {
		colBD[i+1] = colBD[i] + g.colWidth[i]
	}
	for j := range rows {
		rowBD[j+1] = rowBD[j] + g.rowHeight[j]
	}
	g.rsToTS, g.tsToRS, g.tileID = make([]uint32, g.sizeCTBs), make([]uint32, g.sizeCTBs), make([]uint16, g.sizeCTBs)
	for rs := range g.sizeCTBs {
		tbx, tby := rs%g.wCTBs, rs/g.wCTBs
		tx, ty := 0, 0
		for i := cols - 1; i >= 0; i-- {
			if tbx >= colBD[i] {
				tx = i
				break
			}
		}
		for j := rows - 1; j >= 0; j-- {
			if tby >= rowBD[j] {
				ty = j
				break
			}
		}
		ts := 0
		for _, w := range g.colWidth[:tx] {
			ts += g.rowHeight[ty] * w
		}
		for _, h := range g.rowHeight[:ty] {
			ts += g.wCTBs * h
		}
		ts += (tby-rowBD[ty])*g.colWidth[tx] + tbx - colBD[tx]
		g.rsToTS[rs] = uint32(ts)
		g.tsToRS[ts] = uint32(rs)
		g.tileID[rs] = uint16(ty*cols + tx)
	}
	g.minTBW = (s.width + (1 << g.minTBLog2) - 1) >> g.minTBLog2
	g.minTBH = (s.height + (1 << g.minTBLog2) - 1) >> g.minTBLog2
	shift := s.log2CTB - g.minTBLog2
	g.zs = make([]uint32, g.minTBW*g.minTBH)
	for y := range g.minTBH {
		for x := range g.minTBW {
			ctb := g.wCTBs*((y<<g.minTBLog2)>>s.log2CTB) + ((x << g.minTBLog2) >> s.log2CTB)
			v := g.rsToTS[ctb] << (shift * 2)
			for i := range shift {
				m := 1 << i
				mm := uint32(m * m)
				if x&m != 0 {
					v += mm
				}
				if y&m != 0 {
					v += 2 * mm
				}
			}
			g.zs[y*g.minTBW+x] = v
		}
	}
	return g, nil
}
func (g *geometry) z(x, y int) uint32 {
	xi, yi := x>>g.minTBLog2, y>>g.minTBLog2
	if xi >= g.minTBW || yi >= g.minTBH {
		return math.MaxUint32
	}
	return g.zs[yi*g.minTBW+xi]
}
func (g *geometry) ctbRS(x, y int) int { return (y>>g.ctbLog2)*g.wCTBs + (x >> g.ctbLog2) }
