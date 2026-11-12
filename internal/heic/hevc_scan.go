package heic

type scanPoint struct{ x, y uint8 }
type scanTable [64]scanPoint

var scanOrders [4][3]scanTable
var inverseScans [4][3][64]uint8
var subRaster [3][16]uint8

func init() {
	for log2 := range 4 {
		s := 1 << log2
		for idx := range 3 {
			var table scanTable
			if idx == 0 {
				i, x, y := 0, 0, 0
				for {
					for y >= 0 {
						if x < s && y < s {
							table[i] = scanPoint{uint8(x), uint8(y)}
							i++
						}
						y--
						x++
					}
					y = x
					x = 0
					if i >= s*s {
						break
					}
				}
			} else if idx == 1 {
				for y := range s {
					for x := range s {
						table[y*s+x] = scanPoint{uint8(x), uint8(y)}
					}
				}
			} else {
				for x := range s {
					for y := range s {
						table[x*s+y] = scanPoint{uint8(x), uint8(y)}
					}
				}
			}
			scanOrders[log2][idx] = table
			for i := range s * s {
				p := table[i]
				inverseScans[log2][idx][int(p.y)*s+int(p.x)] = uint8(i)
			}
		}
	}
	for idx := range 3 {
		for i := range 16 {
			p := scanOrders[2][idx][i]
			subRaster[idx][i] = p.y<<2 | p.x
		}
	}
}
func scanOrder(log2Size, scanIdx int) *scanTable { return &scanOrders[log2Size][scanIdx] }
func subScan(scanIdx int) *scanTable             { return &scanOrders[2][scanIdx] }
func scanPos(log2Size, scanIdx, x, y int) int {
	return int(inverseScans[log2Size][scanIdx][(y<<log2Size)+x])
}
