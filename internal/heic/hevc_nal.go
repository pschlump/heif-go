package heic

type nalHeader struct{ nalUnitType, layerID, temporalID uint8 }

func parseNALHeader(nal []byte) (nalHeader, error) {
	if len(nal) < 2 {
		return nalHeader{}, truncated("an HEVC bitstream")
	}
	if nal[0]&0x80 != 0 {
		return nalHeader{}, malformed("forbidden_zero_bit set")
	}
	tid := nal[1] & 7
	if tid == 0 {
		return nalHeader{}, malformed("nuh_temporal_id_plus1 is zero")
	}
	return nalHeader{nalUnitType: (nal[0] >> 1) & 0x3f, layerID: (nal[0]&1)<<5 | nal[1]>>3, temporalID: tid - 1}, nil
}
func (h nalHeader) isVCL() bool  { return h.nalUnitType < 32 }
func (h nalHeader) isIRAP() bool { return h.nalUnitType >= 16 && h.nalUnitType <= 23 }

type rbsp struct {
	data []byte
	epb  []uint32
}

func rbspFromNAL(nal []byte) (rbsp, error) {
	if len(nal) < 2 {
		return rbsp{}, truncated("an HEVC bitstream")
	}
	out := rbsp{data: make([]byte, 0, len(nal)-2)}
	zeros, i := 0, 2
	for i < len(nal) {
		b := nal[i]
		if zeros >= 2 && b == 3 {
			out.epb = append(out.epb, uint32(i))
			zeros = 0
			i++
			continue
		}
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
		out.data = append(out.data, b)
		i++
	}
	return out, nil
}
func (r *rbsp) rbspToNAL(off int) int {
	out := off + 2
	for _, p := range r.epb {
		if int(p) <= out {
			out++
		} else {
			break
		}
	}
	return out
}
func (r *rbsp) nalToRBSP(off int) int {
	removed := 0
	for _, p := range r.epb {
		if int(p) < off {
			removed++
		} else {
			break
		}
	}
	return max(0, off-2-removed)
}
