package heic

import "fmt"

type pixelLayout uint8

const (
	layoutRGB8 pixelLayout = iota
	layoutRGBA8
	layoutBGR8
	layoutBGRA8
	layoutGray8
	layoutRGB16
	layoutRGBA16
)

func (l pixelLayout) channels() int {
	if l == layoutGray8 {
		return 1
	}
	if l == layoutRGBA8 || l == layoutBGRA8 || l == layoutRGBA16 {
		return 4
	}
	return 3
}
func (l pixelLayout) bytesPerChannel() int {
	if l == layoutRGB16 || l == layoutRGBA16 {
		return 2
	}
	return 1
}
func (l pixelLayout) bytesPerPixel() int { return l.channels() * l.bytesPerChannel() }
func (l pixelLayout) hasAlpha() bool {
	return l == layoutRGBA8 || l == layoutBGRA8 || l == layoutRGBA16
}
func (l pixelLayout) isBGR() bool { return l == layoutBGR8 || l == layoutBGRA8 }

type decodedImage struct {
	data          []byte
	width, height uint32
	layout        pixelLayout
}

func newDecodedImage(width, height uint32, layout pixelLayout, maxPixels uint64) (decodedImage, error) {
	p, err := checkPixels(width, height, maxPixels)
	if err != nil {
		return decodedImage{}, err
	}
	bytes := p * uint64(layout.bytesPerPixel())
	if bytes/p != uint64(layout.bytesPerPixel()) || bytes > uint64(^uint(0)>>1) {
		return decodedImage{}, fmt.Errorf("image allocation is too large")
	}
	return decodedImage{data: make([]byte, int(bytes)), width: width, height: height, layout: layout}, nil
}
func (i decodedImage) rowBytes() int { return int(i.width) * i.layout.bytesPerPixel() }

func cropSize(width, height uint32, c clap) (uint32, uint32, error) {
	if c.width.d == 0 || c.height.d == 0 {
		return 0, 0, malformed("clap has a zero denominator")
	}
	cw, ch := c.width.n/c.width.d, c.height.n/c.height.d
	if cw == 0 || ch == 0 || cw > width || ch > height {
		return 0, 0, malformed("clap declares an impossible cropped size")
	}
	return cw, ch, nil
}
func cropCorner(full, cropped uint32, off rationalI) (uint32, error) {
	if off.d == 0 {
		return 0, malformed("clap has a zero denominator")
	}
	off2 := 2 * int64(off.n) / int64(off.d)
	left2 := int64(full) - 1 + off2 - (int64(cropped) - 1)
	if left2 < 0 {
		return 0, malformed("clap selects a rectangle outside the image")
	}
	return uint32(left2 / 2), nil
}
func cropRect(width, height uint32, c clap) (uint32, uint32, uint32, uint32, error) {
	cw, ch, err := cropSize(width, height, c)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	left, err := cropCorner(width, cw, c.horizOff)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	top, err := cropCorner(height, ch, c.vertOff)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if left+cw > width || top+ch > height {
		return 0, 0, 0, 0, malformed("clap selects a rectangle outside the image")
	}
	return left, top, cw, ch, nil
}
func transformedSize(width, height uint32, ts []transform) (uint32, uint32, error) {
	var err error
	for _, t := range ts {
		switch t.kind {
		case transformCrop:
			width, height, err = cropSize(width, height, t.crop)
			if err != nil {
				return 0, 0, err
			}
		case transformRotate:
			if t.rot.swapsAxes() {
				width, height = height, width
			}
		}
	}
	return width, height, nil
}
func applyTransforms(img decodedImage, ts []transform) (decodedImage, error) {
	for _, t := range ts {
		switch t.kind {
		case transformCrop:
			left, top, cw, ch, err := cropRect(img.width, img.height, t.crop)
			if err != nil {
				return img, err
			}
			bpp, srcRow, dstRow := img.layout.bytesPerPixel(), img.rowBytes(), int(cw)*img.layout.bytesPerPixel()
			data := make([]byte, dstRow*int(ch))
			for y := range int(ch) {
				s := (int(top)+y)*srcRow + int(left)*bpp
				copy(data[y*dstRow:(y+1)*dstRow], img.data[s:s+dstRow])
			}
			img = decodedImage{data: data, width: cw, height: ch, layout: img.layout}
		case transformRotate:
			if t.rot == rotateNone {
				continue
			}
			bpp, w, h := img.layout.bytesPerPixel(), int(img.width), int(img.height)
			ow, oh := w, h
			if t.rot.swapsAxes() {
				ow, oh = h, w
			}
			data := make([]byte, w*h*bpp)
			for y := range h {
				for x := range w {
					var nx, ny int
					switch t.rot {
					case rotateCCW90:
						nx, ny = y, w-1-x
					case rotateCCW180:
						nx, ny = w-1-x, h-1-y
					default:
						nx, ny = h-1-y, x
					}
					s, d := (y*w+x)*bpp, (ny*ow+nx)*bpp
					copy(data[d:d+bpp], img.data[s:s+bpp])
				}
			}
			img = decodedImage{data: data, width: uint32(ow), height: uint32(oh), layout: img.layout}
		case transformMirror:
			bpp, row, w, h := img.layout.bytesPerPixel(), img.rowBytes(), int(img.width), int(img.height)
			if t.mir == mirrorLeftRight {
				for y := range h {
					base := y * row
					for x := range w / 2 {
						a, b := base+x*bpp, base+(w-1-x)*bpp
						for k := range bpp {
							img.data[a+k], img.data[b+k] = img.data[b+k], img.data[a+k]
						}
					}
				}
			} else {
				for y := range h / 2 {
					a, b := y*row, (h-1-y)*row
					for k := range row {
						img.data[a+k], img.data[b+k] = img.data[b+k], img.data[a+k]
					}
				}
			}
		}
	}
	return img, nil
}
