package heic

import "errors"

var ErrNoEXIF = errors.New("heif: no EXIF found")

type Image struct {
	Data          []byte
	Stride        int
	Width, Height int
}

func Decode(data []byte) (Image, error) {
	o := defaultDecodeOptions()
	o.layout = layoutRGBA8
	img, err := decodeBytes(data, o)
	if err != nil {
		return Image{}, err
	}
	return Image{Data: img.data, Stride: img.rowBytes(), Width: int(img.width), Height: int(img.height)}, nil
}

type Info struct{ Width, Height, BitDepth int }

func Probe(data []byte) (Info, error) {
	info, err := probeBytes(data)
	return Info{Width: int(info.width), Height: int(info.height), BitDepth: int(info.bitDepth)}, err
}

func ExtractEXIF(data []byte) ([]byte, error) {
	ctx, err := openContext(data)
	if err != nil {
		return nil, err
	}
	out, ok, err := ctx.exif(ctx.meta.primary)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNoEXIF
	}
	return out, nil
}
