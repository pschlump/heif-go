package heic

const (
	nalVPS = 32
	nalSPS = 33
	nalPPS = 34
)

type nalArray struct {
	completeness bool
	nalType      uint8
	nals         [][]byte
}

type hevcConfig struct {
	configurationVersion             uint8
	generalProfileSpace              uint8
	generalTierFlag                  bool
	generalProfileIDC                uint8
	generalProfileCompatibilityFlags uint32
	generalConstraintIndicatorFlags  uint64
	generalLevelIDC                  uint8
	minSpatialSegmentationIDC        uint16
	parallelismType                  uint8
	chromaFormat                     uint8
	bitDepthLuma                     uint8
	bitDepthChroma                   uint8
	avgFrameRate                     uint16
	numTemporalLayers                uint8
	temporalIDNested                 bool
	lengthSize                       uint8
	arrays                           []nalArray
}

func (c *hevcConfig) parameterSets() [][]byte {
	var out [][]byte
	for _, want := range []uint8{nalVPS, nalSPS, nalPPS} {
		for _, a := range c.arrays {
			if a.nalType == want {
				out = append(out, a.nals...)
			}
		}
	}
	return out
}

func (c *hevcConfig) splitNALs(data []byte) ([][]byte, error) {
	r := newReader(data)
	var out [][]byte
	for r.remaining() > 0 {
		if r.remaining() < int(c.lengthSize) {
			return nil, truncated("NAL length prefix")
		}
		n, err := r.uintN(c.lengthSize, "NAL length prefix")
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, malformed("zero-length NAL unit")
		}
		if n > uint64(r.remaining()) {
			return nil, truncated("NAL unit")
		}
		b, err := r.take(int(n), "NAL unit")
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func parseHVCC(b boxHeader) (*hevcConfig, error) {
	r := b.reader()
	c := new(hevcConfig)
	var err error
	c.configurationVersion, err = r.u8("hvcC version")
	if err != nil {
		return nil, err
	}
	w, err := r.u8("hvcC profile byte")
	if err != nil {
		return nil, err
	}
	c.generalProfileSpace, c.generalTierFlag, c.generalProfileIDC = w>>6, w&0x20 != 0, w&0x1f
	c.generalProfileCompatibilityFlags, err = r.u32("hvcC compatibility flags")
	if err != nil {
		return nil, err
	}
	hi, err := r.u16("hvcC constraint flags")
	if err != nil {
		return nil, err
	}
	lo, err := r.u32("hvcC constraint flags")
	if err != nil {
		return nil, err
	}
	c.generalConstraintIndicatorFlags = uint64(hi)<<32 | uint64(lo)
	c.generalLevelIDC, err = r.u8("hvcC level")
	if err != nil {
		return nil, err
	}
	c.minSpatialSegmentationIDC, err = r.u16("hvcC segmentation")
	if err != nil {
		return nil, err
	}
	c.minSpatialSegmentationIDC &= 0x0fff
	v, err := r.u8("hvcC parallelism")
	if err != nil {
		return nil, err
	}
	c.parallelismType = v & 3
	v, err = r.u8("hvcC chroma format")
	if err != nil {
		return nil, err
	}
	c.chromaFormat = v & 3
	v, err = r.u8("hvcC luma depth")
	if err != nil {
		return nil, err
	}
	c.bitDepthLuma = v&7 + 8
	v, err = r.u8("hvcC chroma depth")
	if err != nil {
		return nil, err
	}
	c.bitDepthChroma = v&7 + 8
	c.avgFrameRate, err = r.u16("hvcC frame rate")
	if err != nil {
		return nil, err
	}
	last, err := r.u8("hvcC layer byte")
	if err != nil {
		return nil, err
	}
	c.numTemporalLayers, c.temporalIDNested, c.lengthSize = (last>>3)&7, last&4 != 0, last&3+1
	n, err := r.u8("hvcC array count")
	if err != nil {
		return nil, err
	}
	c.arrays = make([]nalArray, 0, n)
	for range n {
		head, e := r.u8("hvcC array header")
		if e != nil {
			return nil, e
		}
		count, e := r.u16("hvcC nalu count")
		if e != nil {
			return nil, e
		}
		if int(count)*2 > r.remaining() {
			return nil, malformed("hvcC declares more NAL units than it holds")
		}
		a := nalArray{completeness: head&0x80 != 0, nalType: head & 0x3f, nals: make([][]byte, 0, count)}
		for range count {
			length, e := r.u16("hvcC nalu length")
			if e != nil {
				return nil, e
			}
			nal, e := r.take(int(length), "hvcC nal unit")
			if e != nil {
				return nil, e
			}
			a.nals = append(a.nals, nal)
		}
		c.arrays = append(c.arrays, a)
	}
	return c, nil
}
