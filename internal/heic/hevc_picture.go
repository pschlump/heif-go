package heic

type plane struct {
	data                  []uint16
	stride, width, height int
}

func newPlane(width, height int) (plane, error) {
	if width < 0 || height < 0 || width != 0 && height > 1<<30/width {
		return plane{}, malformed("the coded picture is too large to address")
	}
	n := width * height
	if n > 1<<30 {
		return plane{}, malformed("the coded picture is too large to address")
	}
	return plane{data: make([]uint16, n), stride: width, width: width, height: height}, nil
}
func (p *plane) at(x, y int) uint16     { return p.data[y*p.stride+x] }
func (p *plane) put(x, y int, v uint16) { p.data[y*p.stride+x] = v }

const (
	flagTQBypass = 1
	flagPCM      = 2
	edgeVer      = 1
	edgeHor      = 2
)

type picture struct {
	y, cb, cr        plane
	min4W, min4H     int
	intraMode        []uint8
	qpY              []int8
	cuFlags, ctDepth []uint8
	min8W, min8H     int
	edges            []uint8
}

func newPicture(width, height, subW, subH int) (picture, error) {
	y, err := newPlane(width, height)
	if err != nil {
		return picture{}, err
	}
	cw, ch := 0, 0
	if subW > 0 && subH > 0 {
		cw, ch = width/subW, height/subH
	}
	cb, err := newPlane(cw, ch)
	if err != nil {
		return picture{}, err
	}
	cr, err := newPlane(cw, ch)
	if err != nil {
		return picture{}, err
	}
	min4W, min4H := (width+3)/4, (height+3)/4
	min8W, min8H := (width+7)/8, (height+7)/8
	n4 := min4W * min4H
	p := picture{y: y, cb: cb, cr: cr, min4W: min4W, min4H: min4H, intraMode: make([]uint8, n4), qpY: make([]int8, n4), cuFlags: make([]uint8, n4), ctDepth: make([]uint8, n4), min8W: min8W, min8H: min8H, edges: make([]uint8, min8W*min8H)}
	for i := range p.intraMode {
		p.intraMode[i] = 1
	}
	return p, nil
}
func (p *picture) idx4(x, y int) int {
	xi := min(x>>2, p.min4W-1)
	yi := min(y>>2, p.min4H-1)
	return yi*p.min4W + xi
}
func (p *picture) markEdge(x, y, size int, kind uint8) {
	if x >= p.y.width || y >= p.y.height {
		return
	}
	if kind&edgeVer != 0 && x&7 == 0 {
		col := x >> 3
		for yy := y; yy < min(y+size, p.y.height); yy += 8 {
			p.edges[(yy>>3)*p.min8W+col] |= edgeVer
		}
	}
	if kind&edgeHor != 0 && y&7 == 0 {
		row := (y >> 3) * p.min8W
		for xx := x; xx < min(x+size, p.y.width); xx += 8 {
			p.edges[row+(xx>>3)] |= edgeHor
		}
	}
}
func fill4(arr []uint8, w, x, y, size int, value uint8) {
	x0, y0, n := x>>2, y>>2, size>>2
	for j := range n {
		row := (y0 + j) * w
		for i := range n {
			if idx := row + x0 + i; idx < len(arr) {
				arr[idx] = value
			}
		}
	}
}
