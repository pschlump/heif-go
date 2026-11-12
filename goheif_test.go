package goheif

import (
	"bytes"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("note", "heic-rs", "tests", "fixtures", name)
}

func TestDecodeFixtures(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{"flat-white-16.heic", 16, 16},
		{"flat-64.heic", 64, 64},
		{"checker-64.heic", 64, 64},
		{"rgb-strips-96.heic", 96, 32},
		{"gradient-512.heic", 512, 512},
		{"checker-1024.heic", 1024, 1024},
		{"photo-2048.heic", 2048, 1536},
		{"rotated-90.heic", 1536, 2048},
		{"with-exif.heic", 2048, 1536},
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
			if got := img.Bounds().Size(); got.X != tt.width || got.Y != tt.height {
				t.Fatalf("size = %v, want %dx%d", got, tt.width, tt.height)
			}
			if err := jpeg.Encode(&bytes.Buffer{}, img, nil); err != nil {
				t.Fatalf("JPEG conversion: %v", err)
			}
		})
	}
}
