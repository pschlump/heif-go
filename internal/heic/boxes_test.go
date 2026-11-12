package heic

import (
	"encoding/binary"
	"testing"
)

func testBox(kind string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out, uint32(len(out)))
	copy(out[4:8], kind)
	copy(out[8:], payload)
	return out
}

func TestBoxTraversal(t *testing.T) {
	data := append(testBox("ftyp", []byte("heic")), testBox("free", nil)...)
	data = append(data, testBox("mdat", []byte{1, 2, 3})...)
	it := newBoxIter(data)
	for i, want := range []struct {
		kind string
		n    int
	}{{"ftyp", 4}, {"free", 0}, {"mdat", 3}} {
		b, ok, err := it.next()
		if err != nil || !ok || b.boxType != want.kind || len(b.payload) != want.n {
			t.Fatalf("box %d = %+v, %v, %v", i, b, ok, err)
		}
	}
}

func TestHostileBoxHeadersDoNotPanic(t *testing.T) {
	for _, kind := range []string{"uuid", "mdat", "meta"} {
		for _, large := range []bool{false, true} {
			for stated := uint64(0); stated < 64; stated++ {
				for length := 0; length < 64; length++ {
					var data []byte
					if large {
						data = make([]byte, 16)
						binary.BigEndian.PutUint32(data, 1)
						binary.BigEndian.PutUint64(data[8:], stated)
					} else {
						data = make([]byte, 8)
						binary.BigEndian.PutUint32(data, uint32(stated))
					}
					copy(data[4:8], kind)
					if length > len(data) {
						data = append(data, make([]byte, length-len(data))...)
					} else {
						data = data[:length]
					}
					it := newBoxIter(data)
					for {
						_, ok, err := it.next()
						if err != nil || !ok {
							break
						}
					}
					_, _ = decodeBytes(data, defaultDecodeOptions())
				}
			}
		}
	}
}
