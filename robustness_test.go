package goheif

import (
	"bytes"
	"errors"
	"image"
	"os"
	"testing"
)

func TestFormatRegistered(t *testing.T) {
	data, err := os.ReadFile("note/goheif/testdata/camel.heic")
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if format != "heic" {
		t.Fatalf("format = %q, want heic", format)
	}
	if got := img.Bounds().Size(); got.X != 1596 || got.Y != 1064 {
		t.Fatalf("size = %v, want 1596x1064", got)
	}
}

func TestMissingEXIF(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, "flat-64.heic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractExif(bytes.NewReader(data)); !errors.Is(err, ErrNoEXIF) {
		t.Fatalf("error = %v, want ErrNoEXIF", err)
	}
}

func TestTruncatedRealFileDoesNotPanic(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, "photo-2048.heic"))
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{1, 8, 32, 100, 500, 800, 2000, 40000} {
		if cut > len(data) {
			continue
		}
		_, _ = Decode(bytes.NewReader(data[:cut]))
	}
}

func FuzzDecode(f *testing.F) {
	for _, name := range []string{"flat-white-16.heic", "flat-64.heic", "checker-1024.heic"} {
		data, err := os.ReadFile("note/heic-rs/tests/fixtures/" + name)
		if err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Decode(bytes.NewReader(data))
	})
}

func TestReportedFuzzFailuresDoNotPanic(t *testing.T) {
	for _, name := range []string{"heic-rs-bug2.heic", "heic-rs-bug3.heic", "heic-rs-bug4.heic", "heic-rs-bug5.heic", "heic-rs-bug6.heic"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(fixturePath(t, "fuzz/"+name))
			if err != nil {
				t.Fatal(err)
			}
			_, _ = Decode(bytes.NewReader(data))
		})
	}
}
