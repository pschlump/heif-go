package heic

import "fmt"

const maxBoxSize = uint64(1 << 30)

type boxHeader struct {
	boxType string
	payload []byte
	uuid    *[16]byte
	offset  int
}

func (b boxHeader) is(code string) bool { return b.boxType == code }
func (b boxHeader) reader() *reader     { return newReader(b.payload) }
func (b boxHeader) children() boxIter   { return newBoxIter(b.payload) }
func (b boxHeader) fullBox(what string) (uint8, uint32, *reader, error) {
	r := b.reader()
	v, f, err := r.fullBox(what)
	return v, f, r, err
}

type boxIter struct {
	data []byte
	pos  int
	done bool
}

func newBoxIter(data []byte) boxIter { return boxIter{data: data} }

func (it *boxIter) next() (boxHeader, bool, error) {
	if it.done || it.pos >= len(it.data) {
		it.done = true
		return boxHeader{}, false, nil
	}
	start := it.pos
	r := newReader(it.data[start:])
	size32, err := r.u32("box size")
	if err != nil {
		it.done = true
		return boxHeader{}, false, err
	}
	boxType, err := r.fourcc("box type")
	if err != nil {
		it.done = true
		return boxHeader{}, false, err
	}
	var total uint64
	switch size32 {
	case 1:
		total, err = r.u64("box largesize")
	case 0:
		total = uint64(len(it.data) - start)
	default:
		total = uint64(size32)
	}
	if err != nil {
		it.done = true
		return boxHeader{}, false, err
	}
	var uuid *[16]byte
	if boxType == "uuid" {
		b, e := r.take(16, "uuid")
		if e != nil {
			it.done = true
			return boxHeader{}, false, e
		}
		uuid = new([16]byte)
		copy(uuid[:], b)
	}
	headerLen := r.position()
	if total < uint64(headerLen) {
		it.done = true
		return boxHeader{}, false, malformed("box size is smaller than its own header")
	}
	if total > maxBoxSize {
		it.done = true
		return boxHeader{}, false, fmt.Errorf("box %q declares an implausible size of %d bytes", boxType, total)
	}
	if total > uint64(len(it.data)-start) {
		it.done = true
		return boxHeader{}, false, truncated("box body")
	}
	end := start + int(total)
	it.pos = end
	return boxHeader{boxType: boxType, payload: it.data[start+headerLen : end], uuid: uuid, offset: start}, true, nil
}

func (it boxIter) find(code string) (boxHeader, bool, error) {
	for {
		b, ok, err := it.next()
		if err != nil || !ok {
			return boxHeader{}, false, err
		}
		if b.is(code) {
			return b, true, nil
		}
	}
}

func (it boxIter) require(code string) (boxHeader, error) {
	b, ok, err := it.find(code)
	if err != nil {
		return boxHeader{}, err
	}
	if !ok {
		return boxHeader{}, missingBox(code)
	}
	return b, nil
}
