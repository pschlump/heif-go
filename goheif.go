// Package goheif decodes HEIC and HEIF still images in pure Go.
package goheif

import (
	"image"
	"image/color"
	"io"
	"math"

	"github.com/pschlump/heif-go/internal/heic"
)

// SafeEncoding is retained for source compatibility with goheif. The pure Go
// decoder does not have the C decoder's container memory-safety modes.
var SafeEncoding bool

// ErrNoEXIF is returned when a file has no EXIF metadata item.
var ErrNoEXIF = heic.ErrNoEXIF

// Decode reads a HEIC/HEIF image and returns non-premultiplied RGBA pixels.
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	img, err := heic.Decode(data)
	if err != nil {
		return nil, err
	}
	out := &image.NRGBA{Pix: img.Data, Stride: img.Stride, Rect: image.Rect(0, 0, img.Width, img.Height)}
	return out, nil
}

// DecodeConfig returns an image's displayed dimensions without decoding pixels.
func DecodeConfig(r io.Reader) (image.Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return image.Config{}, err
	}
	info, err := heic.Probe(data)
	if err != nil {
		return image.Config{}, err
	}
	return image.Config{ColorModel: color.NRGBAModel, Width: info.Width, Height: info.Height}, nil
}

// ExtractExif returns the TIFF payload of the primary image's EXIF item.
func ExtractExif(ra io.ReaderAt) ([]byte, error) {
	data, err := io.ReadAll(io.NewSectionReader(ra, 0, math.MaxInt64))
	if err != nil {
		return nil, err
	}
	return heic.ExtractEXIF(data)
}

func init() {
	image.RegisterFormat("heic", "????ftyp", Decode, DecodeConfig)
}
