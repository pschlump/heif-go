package heic

import (
	"os"
	"testing"
)

func TestParallelMatchesSerial(t *testing.T) {
	data, err := os.ReadFile("../../note/heic-rs/tests/fixtures/photo-2048.heic")
	if err != nil {
		t.Fatal(err)
	}
	serialOptions := defaultDecodeOptions()
	serialOptions.threads = 1
	serial, err := decodeBytes(data, serialOptions)
	if err != nil {
		t.Fatal(err)
	}
	parallelOptions := defaultDecodeOptions()
	parallel, err := decodeBytes(data, parallelOptions)
	if err != nil {
		t.Fatal(err)
	}
	for i := range serial.data {
		if serial.data[i] != parallel.data[i] {
			t.Fatalf("first difference at %d: serial %d, parallel %d", i, serial.data[i], parallel.data[i])
		}
	}
}
