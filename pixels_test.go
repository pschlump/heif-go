package goheif

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func rgbPixels(t *testing.T, img image.Image) ([]byte, int, int) {
	t.Helper()
	b := img.Bounds()
	out := make([]byte, b.Dx()*b.Dy()*3)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := (y*b.Dx() + x) * 3
			out[i], out[i+1], out[i+2] = c.R, c.G, c.B
		}
	}
	return out, b.Dx(), b.Dy()
}

func referencePixels(t *testing.T, name string) ([]byte, int, int) {
	t.Helper()
	f, err := os.Open(fixturePath(t, strings.TrimSuffix(name, ".heic")+".ref.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return rgbPixels(t, img)
}

func psnr(a, b []byte) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	mse := sum / float64(len(a))
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

func lumaPSNR(a, b []byte) float64 {
	var sum float64
	for i := 0; i < len(a); i += 3 {
		ya := .299*float64(a[i]) + .587*float64(a[i+1]) + .114*float64(a[i+2])
		yb := .299*float64(b[i]) + .587*float64(b[i+1]) + .114*float64(b[i+2])
		sum += (ya - yb) * (ya - yb)
	}
	mse := sum / float64(len(a)/3)
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

func maxDelta(a, b []byte) int {
	worst := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
	}
	return worst
}

func TestPixelsMatchPlatformDecoder(t *testing.T) {
	tests := []struct {
		name  string
		exact bool
	}{
		{"flat-white-16.heic", true},
		{"checker-64.heic", true},
		{"checker-1024.heic", true},
		{"flat-64.heic", false},
		{"rgb-strips-96.heic", false},
		{"gradient-512.heic", false},
		{"photo-2048.heic", false},
		{"rotated-90.heic", false},
		{"with-exif.heic", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(fixturePath(t, tt.name))
			if err != nil {
				t.Fatal(err)
			}
			img, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			got, w, h := rgbPixels(t, img)
			want, rw, rh := referencePixels(t, tt.name)
			if w != rw || h != rh {
				t.Fatalf("size = %dx%d, reference %dx%d", w, h, rw, rh)
			}
			if tt.exact {
				if d := maxDelta(got, want); d != 0 {
					t.Fatalf("maximum pixel difference = %d, want exact", d)
				}
				return
			}
			if db := psnr(got, want); db < 45 {
				t.Errorf("RGB PSNR = %.2f dB, want >= 45", db)
			} else {
				t.Logf("RGB %.2f dB, luma %.2f dB", db, lumaPSNR(got, want))
			}
			if db := lumaPSNR(got, want); db < 55 {
				t.Errorf("luma PSNR = %.2f dB, want >= 55", db)
			}
		})
	}
}

func TestExtractExif(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries uint16
	}{{"rotated-90.heic", 3}, {"with-exif.heic", 6}} {
		data, err := os.ReadFile(fixturePath(t, tt.name))
		if err != nil {
			t.Fatal(err)
		}
		exif, err := ExtractExif(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(exif) < 10 || string(exif[:2]) != "MM" || binary.BigEndian.Uint16(exif[2:4]) != 42 {
			t.Fatalf("%s: invalid TIFF header", tt.name)
		}
		ifd := binary.BigEndian.Uint32(exif[4:8])
		if got := binary.BigEndian.Uint16(exif[ifd : ifd+2]); got != tt.entries {
			t.Errorf("%s: EXIF entries = %d, want %d", tt.name, got, tt.entries)
		}
	}
}
