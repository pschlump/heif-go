package heic

import "math/bits"

const numCABACContexts = 139
const (
	ctxSAOMerge       = 0
	ctxSAOType        = 1
	ctxSplitCU        = 2
	ctxTQBypass       = 5
	ctxPartMode       = 6
	ctxPrevIntra      = 7
	ctxIntraChroma    = 8
	ctxSplitTransform = 9
	ctxCBFLuma        = 12
	ctxCBFChroma      = 14
	ctxCUQPDelta      = 19
	ctxTSLuma         = 21
	ctxTSChroma       = 22
	ctxLastX          = 23
	ctxLastY          = 41
	ctxCSBF           = 59
	ctxSig            = 63
	ctxGT1            = 107
	ctxGT2            = 131
	ctxChromaQPFlag   = 137
	ctxChromaQPIdx    = 138
)

var cabacInitValues = [numCABACContexts]uint8{153, 200, 139, 141, 157, 154, 184, 184, 63, 153, 138, 138, 111, 141, 94, 138, 182, 154, 154, 154, 154, 139, 139, 110, 110, 124, 125, 140, 153, 125, 127, 140, 109, 111, 143, 127, 111, 79, 108, 123, 63, 110, 110, 124, 125, 140, 153, 125, 127, 140, 109, 111, 143, 127, 111, 79, 108, 123, 63, 91, 171, 134, 141, 111, 111, 125, 110, 110, 94, 124, 108, 124, 107, 125, 141, 179, 153, 125, 107, 125, 141, 179, 153, 125, 107, 125, 141, 179, 153, 125, 140, 139, 182, 182, 152, 136, 152, 136, 153, 136, 139, 111, 136, 139, 111, 141, 111, 140, 92, 137, 138, 140, 152, 138, 139, 153, 74, 149, 92, 139, 107, 122, 152, 140, 179, 166, 182, 140, 227, 122, 197, 138, 153, 136, 167, 152, 152, 154, 154}
var rangeTabLPS = [64][4]uint8{{128, 176, 208, 240}, {128, 167, 197, 227}, {128, 158, 187, 216}, {123, 150, 178, 205}, {116, 142, 169, 195}, {111, 135, 160, 185}, {105, 128, 152, 175}, {100, 122, 144, 166}, {95, 116, 137, 158}, {90, 110, 130, 150}, {85, 104, 123, 142}, {81, 99, 117, 135}, {77, 94, 111, 128}, {73, 89, 105, 122}, {69, 85, 100, 116}, {66, 80, 95, 110}, {62, 76, 90, 104}, {59, 72, 86, 99}, {56, 69, 81, 94}, {53, 65, 77, 89}, {51, 62, 73, 85}, {48, 59, 69, 80}, {46, 56, 66, 76}, {43, 53, 63, 72}, {41, 50, 59, 69}, {39, 48, 56, 65}, {37, 45, 54, 62}, {35, 43, 51, 59}, {33, 41, 48, 56}, {32, 39, 46, 53}, {30, 37, 43, 50}, {29, 35, 41, 48}, {27, 33, 39, 45}, {26, 31, 37, 43}, {24, 30, 35, 41}, {23, 28, 33, 39}, {22, 27, 32, 37}, {21, 26, 30, 35}, {20, 24, 29, 33}, {19, 23, 27, 31}, {18, 22, 26, 30}, {17, 21, 25, 28}, {16, 20, 23, 27}, {15, 19, 22, 25}, {14, 18, 21, 24}, {14, 17, 20, 23}, {13, 16, 19, 22}, {12, 15, 18, 21}, {12, 14, 17, 20}, {11, 14, 16, 19}, {11, 13, 15, 18}, {10, 12, 15, 17}, {10, 12, 14, 16}, {9, 11, 13, 15}, {9, 11, 12, 14}, {8, 10, 12, 14}, {8, 9, 11, 13}, {7, 9, 11, 12}, {7, 9, 10, 12}, {7, 8, 10, 11}, {6, 8, 9, 11}, {6, 7, 9, 10}, {6, 7, 8, 9}, {2, 2, 2, 2}}
var transIdxLPS = [64]uint8{0, 0, 1, 2, 2, 4, 4, 5, 6, 7, 8, 9, 9, 11, 11, 12, 13, 13, 15, 15, 16, 16, 18, 18, 19, 19, 21, 21, 22, 22, 23, 24, 24, 25, 26, 26, 27, 27, 28, 29, 29, 30, 30, 30, 31, 32, 32, 33, 33, 33, 34, 34, 35, 35, 35, 36, 36, 36, 37, 37, 37, 38, 38, 63}
var transIdxMPS = [64]uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 62, 63}

const cabacOverreadSlack = 16

type cabac struct {
	data              []byte
	next, pos         int
	rangeValue, value uint32
	bitsNeeded        int32
	ctx               [numCABACContexts]uint8
}

func initialContexts(qp int32) [numCABACContexts]uint8 {
	q := max(0, min(qp, 51))
	var out [numCABACContexts]uint8
	for i, init := range cabacInitValues {
		slope := int32(init>>4)*5 - 45
		offset := int32(init&15)*8 - 16
		pre := max(1, min(((slope*q)>>4)+offset, 126))
		mps := uint8(0)
		if pre > 63 {
			mps = 1
		}
		state := 63 - pre
		if mps == 1 {
			state = pre - 64
		}
		out[i] = uint8(state)<<1 | mps
	}
	return out
}
func newCABAC(data []byte, byteOff int, qp int32) (cabac, error) {
	c := cabac{data: data, next: byteOff, pos: byteOff * 8, rangeValue: 510, bitsNeeded: -8}
	c.ctx = initialContexts(qp)
	err := c.initEngine()
	return c, err
}
func (c *cabac) initContexts(qp int32) { c.ctx = initialContexts(qp) }
func (c *cabac) fetch() uint32 {
	var b uint8
	if c.next < len(c.data) {
		b = c.data[c.next]
	}
	c.next++
	return uint32(b)
}
func (c *cabac) checkOverread() error {
	if c.next > len(c.data)+cabacOverreadSlack {
		return truncated("an HEVC bitstream")
	}
	return nil
}
func (c *cabac) initEngine() error {
	c.pos = (c.pos + 7) &^ 7
	c.next = c.pos >> 3
	c.rangeValue = 510
	c.value = c.fetch()<<8 | c.fetch()
	c.bitsNeeded = -8
	if c.value>>7 >= 510 {
		return malformed("CABAC ivlOffset is 510 or 511")
	}
	return c.checkOverread()
}
func (c *cabac) restartAt(off int) error { c.pos = off * 8; return c.initEngine() }
func (c *cabac) bitPos() int             { return c.next*8 - int(-c.bitsNeeded-1) }
func (c *cabac) alignedPos() int         { return (c.bitPos() + 7) >> 3 }
func (c *cabac) shiftOne() {
	c.value <<= 1
	c.bitsNeeded++
	if c.bitsNeeded == 0 {
		c.bitsNeeded = -8
		c.value |= c.fetch()
	}
}
func (c *cabac) decision(idx int) uint32 {
	s := c.ctx[idx]
	state := int((s >> 1) & 63)
	mps := uint32(s & 1)
	lps := uint32(rangeTabLPS[state][(c.rangeValue>>6)&3])
	r := c.rangeValue - lps
	scaled := r << 7
	if c.value < scaled {
		c.ctx[idx] = transIdxMPS[state]<<1 | (s & 1)
		if r < 256 {
			c.rangeValue = r << 1
			c.shiftOne()
		} else {
			c.rangeValue = r
		}
		return mps
	}
	c.value -= scaled
	shift := uint(bits.LeadingZeros32(lps) - 23)
	c.rangeValue = lps << shift
	c.value <<= shift
	c.bitsNeeded += int32(shift)
	if c.bitsNeeded >= 0 {
		b := c.fetch()
		c.value |= b << c.bitsNeeded
		c.bitsNeeded -= 8
	}
	newMPS := mps
	if state == 0 {
		newMPS = 1 - mps
	}
	c.ctx[idx] = transIdxLPS[state]<<1 | uint8(newMPS)
	return 1 - mps
}
func (c *cabac) bypass() uint32 {
	c.shiftOne()
	scaled := c.rangeValue << 7
	bit := uint32(0)
	if c.value >= scaled {
		bit = 1
	}
	c.value -= scaled & uint32(-int32(bit))
	return bit
}
func (c *cabac) bypassBits(n uint32) uint32 {
	var v uint32
	for range n {
		v = v<<1 | c.bypass()
	}
	return v
}
func (c *cabac) terminate() (uint32, error) {
	if err := c.checkOverread(); err != nil {
		return 0, err
	}
	c.rangeValue -= 2
	scaled := c.rangeValue << 7
	if c.value >= scaled {
		return 1, nil
	}
	if c.rangeValue < 256 {
		c.rangeValue <<= 1
		c.shiftOne()
	}
	return 0, nil
}
func (c *cabac) alignAfterTerminate() { c.pos = c.alignedPos() << 3 }
func (c *cabac) rawBits(n uint32) (uint32, error) {
	var v uint32
	for range n {
		idx := c.pos >> 3
		if idx > len(c.data)+cabacOverreadSlack {
			return 0, truncated("an HEVC bitstream")
		}
		var b uint8
		if idx < len(c.data) {
			b = c.data[idx]
		}
		v = v<<1 | uint32((b>>(7-(c.pos&7)))&1)
		c.pos++
	}
	return v, nil
}
