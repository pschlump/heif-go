package heic

import "math/bits"

type sliceHeader struct {
	firstInPic                               bool
	ppsID                                    uint32
	dependent                                bool
	segmentAddress, sliceAddress             uint32
	saoLuma, saoChroma                       bool
	qp, cbQPOffset, crQPOffset               int32
	cuChromaQPOffsetEnabled, deblockDisabled bool
	betaOffsetDiv2, tcOffsetDiv2             int32
	acrossSlices                             bool
	entryPoints                              []uint32
	dataOffset                               int
}

func ceilLog2(n int) uint32 {
	if n <= 1 {
		return 0
	}
	return uint32(bits.Len(uint(n - 1)))
}
func parseSliceHeader(nh *nalHeader, data []byte, s *sps, p *pps) (sliceHeader, error) {
	r := newBitReader(data)
	h := sliceHeader{acrossSlices: p.acrossSlices}
	var err error
	flag, err := r.u1()
	if err != nil {
		return h, err
	}
	h.firstInPic = flag != 0
	if nh.isIRAP() {
		if _, err = r.u1(); err != nil {
			return h, err
		}
	}
	if h.ppsID, err = r.ue(); err != nil {
		return h, err
	}
	if !h.firstInPic {
		if p.dependentSlices {
			flag, err = r.u1()
			if err != nil {
				return h, err
			}
			h.dependent = flag != 0
		}
		if h.segmentAddress, err = r.u(ceilLog2(s.sizeInCTBs())); err != nil {
			return h, err
		}
		if int(h.segmentAddress) >= s.sizeInCTBs() {
			return h, malformed("slice_segment_address out of range")
		}
	}
	if h.dependent {
		return h, unsupported("dependent slice segments")
	}
	h.sliceAddress = h.segmentAddress
	if err = parseIndependentSlice(r, nh, s, p, &h); err != nil {
		return h, err
	}
	if p.tilesEnabled || p.wppEnabled {
		n, e := r.ue()
		if e != nil {
			return h, e
		}
		if uint64(n) > uint64(s.sizeInCTBs()+1) {
			return h, malformed("num_entry_point_offsets too large")
		}
		if n > 0 {
			length, e := r.ue()
			if e != nil {
				return h, e
			}
			length++
			if length > 32 {
				return h, malformed("offset_len_minus1 > 31")
			}
			for range n {
				v, e := r.u(length)
				if e != nil {
					return h, e
				}
				h.entryPoints = append(h.entryPoints, v+1)
			}
		}
	}
	if p.sliceHeaderExtension {
		n, e := r.ue()
		if e != nil {
			return h, e
		}
		if uint64(n)*8 > uint64(r.end-r.pos) {
			return h, truncated("an HEVC bitstream")
		}
		if err = r.skip(int(n) * 8); err != nil {
			return h, err
		}
	}
	if _, err = r.u1(); err != nil {
		return h, err
	}
	r.byteAlign()
	h.dataOffset = r.bytePos()
	return h, nil
}
func parseIndependentSlice(r *bitReader, nh *nalHeader, s *sps, p *pps, h *sliceHeader) error {
	var err error
	if err = r.skip(int(p.extraSliceHeaderBits)); err != nil {
		return err
	}
	sliceType, err := r.ue()
	if err != nil {
		return err
	}
	if sliceType != 2 {
		return unsupported("P and B slices (inter prediction)")
	}
	if p.outputFlagPresent {
		if _, err = r.u1(); err != nil {
			return err
		}
	}
	if s.separateColourPlane {
		return unsupported("separate_colour_plane_flag")
	}
	if nh.nalUnitType != 19 && nh.nalUnitType != 20 {
		if err = r.skip(int(s.log2MaxPOCLSB)); err != nil {
			return err
		}
		flag, e := r.u1()
		if e != nil {
			return e
		}
		if flag == 0 {
			ndp := append([]uint32(nil), s.numDeltaPOCs...)
			if err = parseRPS(r, s.numSTRPS, s.numSTRPS, &ndp); err != nil {
				return err
			}
		} else if s.numSTRPS > 1 {
			if err = r.skip(int(ceilLog2(int(s.numSTRPS)))); err != nil {
				return err
			}
		}
		if s.longTermPresent {
			nSPS := uint32(0)
			if s.numLTSPS > 0 {
				if nSPS, err = r.ue(); err != nil {
					return err
				}
			}
			nPics, e := r.ue()
			if e != nil {
				return e
			}
			if nSPS > 64 || nPics > 64 {
				return malformed("too many long-term reference pictures")
			}
			for i := range nSPS + nPics {
				if i < nSPS {
					if s.numLTSPS > 1 {
						if err = r.skip(int(ceilLog2(int(s.numLTSPS)))); err != nil {
							return err
						}
					}
				} else {
					if err = r.skip(int(s.log2MaxPOCLSB)); err != nil {
						return err
					}
					if _, err = r.u1(); err != nil {
						return err
					}
				}
				flag, e := r.u1()
				if e != nil {
					return e
				}
				if flag != 0 {
					if _, err = r.ue(); err != nil {
						return err
					}
				}
			}
		}
		if s.temporalMVP {
			if _, err = r.u1(); err != nil {
				return err
			}
		}
	}
	if s.saoEnabled {
		flag, e := r.u1()
		if e != nil {
			return e
		}
		h.saoLuma = flag != 0
		if s.chromaArrayType != 0 {
			flag, e = r.u1()
			if e != nil {
				return e
			}
			h.saoChroma = flag != 0
		}
	}
	qpDelta, err := r.se()
	if err != nil {
		return err
	}
	h.qp = p.initQP + qpDelta
	bdOffset := 6 * (int32(s.bitDepthY) - 8)
	if h.qp < -bdOffset || h.qp > 51 {
		return malformed("slice QP out of range")
	}
	if p.sliceChromaQPOffsetsPresent {
		if h.cbQPOffset, err = r.se(); err != nil {
			return err
		}
		if h.crQPOffset, err = r.se(); err != nil {
			return err
		}
	}
	if p.chromaQPOffsetListEnabled {
		flag, e := r.u1()
		if e != nil {
			return e
		}
		h.cuChromaQPOffsetEnabled = flag != 0
	}
	h.deblockDisabled, h.betaOffsetDiv2, h.tcOffsetDiv2 = p.deblockDisabled, p.betaOffsetDiv2, p.tcOffsetDiv2
	overridden := false
	if p.deblockOverrideEnabled {
		flag, e := r.u1()
		if e != nil {
			return e
		}
		overridden = flag != 0
	}
	if overridden {
		flag, e := r.u1()
		if e != nil {
			return e
		}
		h.deblockDisabled = flag != 0
		if !h.deblockDisabled {
			if h.betaOffsetDiv2, err = r.se(); err != nil {
				return err
			}
			if h.tcOffsetDiv2, err = r.se(); err != nil {
				return err
			}
		}
	}
	if p.acrossSlices && (h.saoLuma || h.saoChroma || !h.deblockDisabled) {
		flag, e := r.u1()
		if e != nil {
			return e
		}
		h.acrossSlices = flag != 0
	}
	return nil
}
