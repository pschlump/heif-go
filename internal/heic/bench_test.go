package heic

import (
	"os"
	"testing"
)

func benchmarkFixture(b *testing.B, name string) []byte {
	b.Helper()
	data, err := os.ReadFile("../../note/heic-rs/tests/fixtures/" + name)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

func BenchmarkDecodeRGB8(b *testing.B) {
	for _, name := range []string{"gradient-512.heic", "checker-1024.heic", "photo-2048.heic"} {
		data := benchmarkFixture(b, name)
		for _, threads := range []int{1, 14} {
			tag := "serial"
			if threads != 1 {
				tag = "pool"
			}
			b.Run(name+"/"+tag, func(b *testing.B) {
				o := defaultDecodeOptions()
				o.threads = threads
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := decodeBytes(data, o); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkColorConvertRGB8(b *testing.B) {
	const w, h = 2048, 1536
	cw, ch := w/2, h/2
	f := frame{width: w, height: h, bitDepth: 8, chroma: chroma420, yStride: w, cStride: uint32(cw), y: make([]uint16, w*h), cb: make([]uint16, cw*ch), cr: make([]uint16, cw*ch)}
	for i := range f.y {
		f.y[i] = uint16(i % 255)
	}
	for i := range f.cb {
		f.cb[i] = uint16(i % 255)
		f.cr[i] = uint16(i % 200)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := convertFrame(&f, nil, defaultNCLX(), layoutRGB8, defaultMaxPixels); err != nil {
			b.Fatal(err)
		}
	}
}
