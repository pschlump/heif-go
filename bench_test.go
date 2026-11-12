package goheif

import (
	"bytes"
	"os"
	"testing"
)

func BenchmarkDecode(b *testing.B) {
	for _, name := range []string{"flat-64.heic", "gradient-512.heic", "checker-1024.heic", "photo-2048.heic"} {
		data, err := os.ReadFile("note/heic-rs/tests/fixtures/" + name)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Decode(bytes.NewReader(data)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecodeOriginalCamel(b *testing.B) {
	data, err := os.ReadFile("note/goheif/testdata/camel.heic")
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}
