package heic

type sps struct {
	id                                                                                                                                    uint32
	separateColourPlane                                                                                                                   bool
	chromaArrayType                                                                                                                       uint8
	subW, subH, width, height                                                                                                             int
	crop                                                                                                                                  [4]int
	bitDepthY, bitDepthC                                                                                                                  uint8
	log2MaxPOCLSB                                                                                                                         uint32
	log2MinCB, log2CTB, log2MinTB, log2MaxTB                                                                                              int
	maxTrDepthIntra                                                                                                                       uint32
	scalingListEnabled                                                                                                                    bool
	scaling                                                                                                                               *scalingListData
	saoEnabled, pcmEnabled                                                                                                                bool
	pcmBitDepthY, pcmBitDepthC                                                                                                            uint8
	log2MinPCMCB, log2MaxPCMCB                                                                                                            int
	pcmLoopFilterDisabled                                                                                                                 bool
	numSTRPS                                                                                                                              uint32
	numDeltaPOCs                                                                                                                          []uint32
	longTermPresent                                                                                                                       bool
	numLTSPS                                                                                                                              uint32
	temporalMVP, strongIntraSmoothing, intraSmoothingDisabled, persistentRice, transformSkipContext, transformSkipRotation, implicitRDPCM bool
}

func (s *sps) widthInCTBs() int  { return (s.width + (1 << s.log2CTB) - 1) >> s.log2CTB }
func (s *sps) heightInCTBs() int { return (s.height + (1 << s.log2CTB) - 1) >> s.log2CTB }
func (s *sps) sizeInCTBs() int   { return s.widthInCTBs() * s.heightInCTBs() }

type pps struct {
	id, spsID                                                               uint32
	dependentSlices, outputFlagPresent                                      bool
	extraSliceHeaderBits                                                    uint32
	signDataHiding, cabacInitPresent                                        bool
	initQP                                                                  int32
	constrainedIntraPred, transformSkip, cuQPDeltaEnabled                   bool
	diffCUQPDeltaDepth                                                      uint32
	cbQPOffset, crQPOffset                                                  int32
	sliceChromaQPOffsetsPresent, transquantBypass, tilesEnabled, wppEnabled bool
	numTileCols, numTileRows                                                int
	uniformSpacing                                                          bool
	columnWidths, rowHeights                                                []int
	acrossTiles, acrossSlices, deblockOverrideEnabled, deblockDisabled      bool
	betaOffsetDiv2, tcOffsetDiv2                                            int32
	scaling                                                                 *scalingListData
	sliceHeaderExtension                                                    bool
	log2MaxTransformSkip                                                    int
	chromaQPOffsetListEnabled                                               bool
	diffCUChromaQPOffsetDepth                                               uint32
	cbQPOffsetList, crQPOffsetList                                          []int32
	saoOffsetScaleLuma, saoOffsetScaleChroma                                uint32
}

func newPPS() pps {
	return pps{numTileCols: 1, numTileRows: 1, uniformSpacing: true, acrossTiles: true, log2MaxTransformSkip: 2}
}

func parsePTL(r *bitReader, profilePresent bool, maxSubMinus1 uint32) error {
	if profilePresent {
		if err := r.skip(3); err != nil {
			return err
		}
		if _, err := r.u(5); err != nil {
			return err
		}
		if err := r.skip(32 + 4 + 43 + 1); err != nil {
			return err
		}
	}
	if _, err := r.u(8); err != nil {
		return err
	}
	n := int(maxSubMinus1)
	var prof, lvl [8]bool
	for i := range n {
		a, err := r.u1()
		if err != nil {
			return err
		}
		b, err := r.u1()
		if err != nil {
			return err
		}
		prof[i], lvl[i] = a != 0, b != 0
	}
	if n > 0 {
		if err := r.skip((8 - n) * 2); err != nil {
			return err
		}
	}
	for i := range n {
		if prof[i] {
			if err := r.skip(2 + 1 + 5 + 32 + 4 + 43 + 1); err != nil {
				return err
			}
		}
		if lvl[i] {
			if err := r.skip(8); err != nil {
				return err
			}
		}
	}
	return nil
}
func parseVPS(data []byte) (uint32, error) { return newBitReader(data).u(4) }
func parseRPS(r *bitReader, idx, numSets uint32, ndp *[]uint32) error {
	inter := false
	var err error
	if idx != 0 {
		v, e := r.u1()
		inter, err = v != 0, e
		if err != nil {
			return err
		}
	}
	var numDelta uint32
	if inter {
		deltaIdx := uint32(1)
		if idx == numSets {
			v, e := r.ue()
			if e != nil {
				return e
			}
			deltaIdx = v + 1
		}
		if deltaIdx > idx {
			return malformed("delta_idx_minus1 out of range")
		}
		ref := int(idx - deltaIdx)
		if ref >= len(*ndp) {
			return malformed("bad RefRpsIdx")
		}
		refNDP := (*ndp)[ref]
		if _, err = r.u1(); err != nil {
			return err
		}
		if _, err = r.ue(); err != nil {
			return err
		}
		for range refNDP + 1 {
			used, e := r.u1()
			if e != nil {
				return e
			}
			keep := used != 0
			if !keep {
				v, e := r.u1()
				if e != nil {
					return e
				}
				keep = v != 0
			}
			if keep {
				numDelta++
			}
		}
	} else {
		neg, e := r.ue()
		if e != nil {
			return e
		}
		pos, e := r.ue()
		if e != nil {
			return e
		}
		if neg > 16 || pos > 16 {
			return malformed("too many short-term reference pictures")
		}
		for range neg + pos {
			if _, err = r.ue(); err != nil {
				return err
			}
			if _, err = r.u1(); err != nil {
				return err
			}
		}
		numDelta = neg + pos
	}
	if idx < numSets {
		*ndp = append(*ndp, numDelta)
	}
	return nil
}
func parseHRD(r *bitReader, common bool, maxSub uint32) error {
	nal, vcl, sub := false, false, false
	var err error
	if common {
		a, e := r.u1()
		if e != nil {
			return e
		}
		nal = a != 0
		a, e = r.u1()
		if e != nil {
			return e
		}
		vcl = a != 0
		if nal || vcl {
			a, e = r.u1()
			if e != nil {
				return e
			}
			sub = a != 0
			if sub {
				if err = r.skip(8 + 5 + 1 + 5); err != nil {
					return err
				}
			}
			if err = r.skip(8); err != nil {
				return err
			}
			if sub {
				if err = r.skip(4); err != nil {
					return err
				}
			}
			if err = r.skip(15); err != nil {
				return err
			}
		}
	}
	for range maxSub + 1 {
		a, e := r.u1()
		if e != nil {
			return e
		}
		fixedGeneral := a != 0
		fixedWithin := true
		if !fixedGeneral {
			a, e = r.u1()
			if e != nil {
				return e
			}
			fixedWithin = a != 0
		}
		low := false
		if fixedWithin {
			if _, err = r.ue(); err != nil {
				return err
			}
		} else {
			a, e = r.u1()
			if e != nil {
				return e
			}
			low = a != 0
		}
		cpb := uint32(0)
		if !low {
			cpb, err = r.ue()
			if err != nil {
				return err
			}
		}
		sets := 0
		if nal {
			sets++
		}
		if vcl {
			sets++
		}
		for range sets {
			for range cpb + 1 {
				if _, err = r.ue(); err != nil {
					return err
				}
				if _, err = r.ue(); err != nil {
					return err
				}
				if sub {
					if _, err = r.ue(); err != nil {
						return err
					}
					if _, err = r.ue(); err != nil {
						return err
					}
				}
				if _, err = r.u1(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func parseVUI(r *bitReader, maxSub uint32) error {
	flag, err := r.u1()
	if err != nil {
		return err
	}
	if flag != 0 {
		idc, e := r.u(8)
		if e != nil {
			return e
		}
		if idc == 255 {
			if err = r.skip(32); err != nil {
				return err
			}
		}
	}
	if flag, err = r.u1(); err != nil {
		return err
	} else if flag != 0 {
		if err = r.skip(1); err != nil {
			return err
		}
	}
	if flag, err = r.u1(); err != nil {
		return err
	} else if flag != 0 {
		if err = r.skip(4); err != nil {
			return err
		}
		if flag, err = r.u1(); err != nil {
			return err
		} else if flag != 0 {
			if err = r.skip(24); err != nil {
				return err
			}
		}
	}
	if flag, err = r.u1(); err != nil {
		return err
	} else if flag != 0 {
		if _, err = r.ue(); err != nil {
			return err
		}
		if _, err = r.ue(); err != nil {
			return err
		}
	}
	if err = r.skip(3); err != nil {
		return err
	}
	if flag, err = r.u1(); err != nil {
		return err
	} else if flag != 0 {
		for range 4 {
			if _, err = r.ue(); err != nil {
				return err
			}
		}
	}
	if flag, err = r.u1(); err != nil {
		return err
	} else if flag != 0 {
		if err = r.skip(64); err != nil {
			return err
		}
		if flag, err = r.u1(); err != nil {
			return err
		} else if flag != 0 {
			if _, err = r.ue(); err != nil {
				return err
			}
		}
		if flag, err = r.u1(); err != nil {
			return err
		} else if flag != 0 {
			if err = parseHRD(r, true, maxSub); err != nil {
				return err
			}
		}
	}
	if flag, err = r.u1(); err != nil {
		return err
	} else if flag != 0 {
		if err = r.skip(3); err != nil {
			return err
		}
		for range 5 {
			if _, err = r.ue(); err != nil {
				return err
			}
		}
	}
	return nil
}
func parseSPS(data []byte) (sps, error) {
	r := newBitReader(data)
	var s sps
	var err error
	if err = r.skip(4); err != nil {
		return s, err
	}
	maxSub, err := r.u(3)
	if err != nil {
		return s, err
	}
	if err = r.skip(1); err != nil {
		return s, err
	}
	if err = parsePTL(r, true, maxSub); err != nil {
		return s, err
	}
	if s.id, err = r.ue(); err != nil {
		return s, err
	}
	chroma, err := r.ue()
	if err != nil {
		return s, err
	}
	if chroma > 3 {
		return s, malformed("chroma_format_idc > 3")
	}
	if chroma == 3 {
		v, e := r.u1()
		if e != nil {
			return s, e
		}
		s.separateColourPlane = v != 0
	}
	s.chromaArrayType = uint8(chroma)
	if s.separateColourPlane {
		s.chromaArrayType = 0
	}
	switch s.chromaArrayType {
	case 1:
		s.subW, s.subH = 2, 2
	case 2:
		s.subW, s.subH = 2, 1
	case 3:
		s.subW, s.subH = 1, 1
	}
	v, err := r.ue()
	if err != nil {
		return s, err
	}
	s.width = int(v)
	v, err = r.ue()
	if err != nil {
		return s, err
	}
	s.height = int(v)
	if s.width == 0 || s.height == 0 || s.width > 65535 || s.height > 65535 {
		return s, malformed("implausible picture dimensions")
	}
	flag, err := r.u1()
	if err != nil {
		return s, err
	}
	if flag != 0 {
		cw, ch := 1, 1
		if s.subW != 0 {
			cw, ch = s.subW, s.subH
		}
		for i, unit := range []int{cw, cw, ch, ch} {
			v, e := r.ue()
			if e != nil {
				return s, e
			}
			if uint64(v)*uint64(unit) > uint64(^uint(0)>>1) {
				return s, malformed("conformance window larger than the picture")
			}
			s.crop[i] = int(v) * unit
		}
		if s.crop[0]+s.crop[1] >= s.width || s.crop[2]+s.crop[3] >= s.height {
			return s, malformed("conformance window larger than the picture")
		}
	}
	v, err = r.ue()
	if err != nil {
		return s, err
	}
	s.bitDepthY = uint8(v + 8)
	v, err = r.ue()
	if err != nil {
		return s, err
	}
	s.bitDepthC = uint8(v + 8)
	if s.bitDepthY < 8 || s.bitDepthY > 16 || s.bitDepthC < 8 || s.bitDepthC > 16 {
		return s, malformed("bit depth out of range")
	}
	if s.log2MaxPOCLSB, err = r.ue(); err != nil {
		return s, err
	}
	s.log2MaxPOCLSB += 4
	if s.log2MaxPOCLSB > 16 {
		return s, malformed("log2_max_pic_order_cnt_lsb too large")
	}
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	first := maxSub
	if flag != 0 {
		first = 0
	}
	for range maxSub - first + 1 {
		for range 3 {
			if _, err = r.ue(); err != nil {
				return s, err
			}
		}
	}
	v, err = r.ue()
	if err != nil {
		return s, err
	}
	s.log2MinCB = int(v) + 3
	v, err = r.ue()
	if err != nil {
		return s, err
	}
	s.log2CTB = s.log2MinCB + int(v)
	v, err = r.ue()
	if err != nil {
		return s, err
	}
	s.log2MinTB = int(v) + 2
	v, err = r.ue()
	if err != nil {
		return s, err
	}
	s.log2MaxTB = s.log2MinTB + int(v)
	if s.log2CTB > 6 || s.log2MinCB < 3 || s.log2MaxTB > 5 || s.log2MinTB >= s.log2MinCB {
		return s, malformed("implausible block size configuration")
	}
	if _, err = r.ue(); err != nil {
		return s, err
	}
	if s.maxTrDepthIntra, err = r.ue(); err != nil {
		return s, err
	}
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	s.scalingListEnabled = flag != 0
	if s.scalingListEnabled {
		flag, err = r.u1()
		if err != nil {
			return s, err
		}
		if flag != 0 {
			d := defaultScalingList()
			if err = parseScalingList(r, &d); err != nil {
				return s, err
			}
			s.scaling = &d
		}
	}
	if _, err = r.u1(); err != nil {
		return s, err
	}
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	s.saoEnabled = flag != 0
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	s.pcmEnabled = flag != 0
	s.pcmBitDepthY, s.pcmBitDepthC, s.log2MinPCMCB, s.log2MaxPCMCB = 8, 8, 3, 3
	if s.pcmEnabled {
		v, e := r.u(4)
		if e != nil {
			return s, e
		}
		s.pcmBitDepthY = uint8(v) + 1
		v, e = r.u(4)
		if e != nil {
			return s, e
		}
		s.pcmBitDepthC = uint8(v) + 1
		v, e = r.ue()
		if e != nil {
			return s, e
		}
		s.log2MinPCMCB = int(v) + 3
		v, e = r.ue()
		if e != nil {
			return s, e
		}
		s.log2MaxPCMCB = s.log2MinPCMCB + int(v)
		flag, err = r.u1()
		if err != nil {
			return s, err
		}
		s.pcmLoopFilterDisabled = flag != 0
		if s.pcmBitDepthY > s.bitDepthY || s.pcmBitDepthC > s.bitDepthC {
			return s, malformed("PCM bit depth greater than the sample bit depth")
		}
		if s.log2MaxPCMCB > 5 {
			return s, malformed("PCM block size out of range")
		}
	}
	if s.numSTRPS, err = r.ue(); err != nil {
		return s, err
	}
	if s.numSTRPS > 64 {
		return s, malformed("num_short_term_ref_pic_sets > 64")
	}
	for i := range s.numSTRPS {
		if err = parseRPS(r, i, s.numSTRPS, &s.numDeltaPOCs); err != nil {
			return s, err
		}
	}
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	s.longTermPresent = flag != 0
	if s.longTermPresent {
		if s.numLTSPS, err = r.ue(); err != nil {
			return s, err
		}
		for range s.numLTSPS {
			if err = r.skip(int(s.log2MaxPOCLSB)); err != nil {
				return s, err
			}
			if _, err = r.u1(); err != nil {
				return s, err
			}
		}
	}
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	s.temporalMVP = flag != 0
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	s.strongIntraSmoothing = flag != 0
	flag, err = r.u1()
	if err != nil {
		return s, err
	}
	if flag != 0 {
		if err = parseVUI(r, maxSub); err != nil {
			return s, err
		}
	}
	if r.moreRBSPData() {
		flag, err = r.u1()
		if err != nil {
			return s, err
		}
		if flag != 0 {
			if err = parseSPSExtensions(r, &s); err != nil {
				return s, err
			}
		}
	}
	return s, nil
}
func parseSPSExtensions(r *bitReader, s *sps) error {
	rangeExt, err := r.u1()
	if err != nil {
		return err
	}
	multi, err := r.u1()
	if err != nil {
		return err
	}
	threeD, err := r.u1()
	if err != nil {
		return err
	}
	scc, err := r.u1()
	if err != nil {
		return err
	}
	if err = r.skip(4); err != nil {
		return err
	}
	if rangeExt != 0 {
		flags := make([]uint32, 9)
		for i := range flags {
			flags[i], err = r.u1()
			if err != nil {
				return err
			}
		}
		s.transformSkipRotation = flags[0] != 0
		s.transformSkipContext = flags[1] != 0
		s.implicitRDPCM = flags[2] != 0
		s.intraSmoothingDisabled = flags[5] != 0
		s.persistentRice = flags[7] != 0
		if flags[4] != 0 {
			return unsupported("extended_precision_processing_flag")
		}
		if flags[8] != 0 {
			return unsupported("cabac_bypass_alignment_enabled_flag")
		}
		if flags[3] != 0 {
			return unsupported("explicit_rdpcm_enabled_flag")
		}
	}
	if multi != 0 || threeD != 0 || scc != 0 {
		return unsupported("SPS multilayer, 3D or SCC extension")
	}
	return nil
}
func parsePPS(data []byte) (pps, error) {
	r := newBitReader(data)
	p := newPPS()
	var err error
	if p.id, err = r.ue(); err != nil {
		return p, err
	}
	if p.spsID, err = r.ue(); err != nil {
		return p, err
	}
	boolFields := []*bool{&p.dependentSlices, &p.outputFlagPresent}
	for _, dst := range boolFields {
		v, e := r.u1()
		if e != nil {
			return p, e
		}
		*dst = v != 0
	}
	if p.extraSliceHeaderBits, err = r.u(3); err != nil {
		return p, err
	}
	for _, dst := range []*bool{&p.signDataHiding, &p.cabacInitPresent} {
		v, e := r.u1()
		if e != nil {
			return p, e
		}
		*dst = v != 0
	}
	for range 2 {
		if _, err = r.ue(); err != nil {
			return p, err
		}
	}
	qp, err := r.se()
	if err != nil {
		return p, err
	}
	p.initQP = qp + 26
	if p.initQP < -48 || p.initQP > 51 {
		return p, malformed("init_qp out of range")
	}
	for _, dst := range []*bool{&p.constrainedIntraPred, &p.transformSkip, &p.cuQPDeltaEnabled} {
		v, e := r.u1()
		if e != nil {
			return p, e
		}
		*dst = v != 0
	}
	if p.cuQPDeltaEnabled {
		if p.diffCUQPDeltaDepth, err = r.ue(); err != nil {
			return p, err
		}
	}
	if p.cbQPOffset, err = r.se(); err != nil {
		return p, err
	}
	if p.crQPOffset, err = r.se(); err != nil {
		return p, err
	}
	flag, err := r.u1()
	if err != nil {
		return p, err
	}
	p.sliceChromaQPOffsetsPresent = flag != 0
	if err = r.skip(2); err != nil {
		return p, err
	}
	for _, dst := range []*bool{&p.transquantBypass, &p.tilesEnabled, &p.wppEnabled} {
		v, e := r.u1()
		if e != nil {
			return p, e
		}
		*dst = v != 0
	}
	if p.tilesEnabled {
		v, e := r.ue()
		if e != nil {
			return p, e
		}
		p.numTileCols = int(v) + 1
		v, e = r.ue()
		if e != nil {
			return p, e
		}
		p.numTileRows = int(v) + 1
		if p.numTileCols > 1024 || p.numTileRows > 1024 {
			return p, malformed("implausible tile count")
		}
		flag, err = r.u1()
		if err != nil {
			return p, err
		}
		p.uniformSpacing = flag != 0
		if !p.uniformSpacing {
			for range p.numTileCols - 1 {
				v, e := r.ue()
				if e != nil {
					return p, e
				}
				p.columnWidths = append(p.columnWidths, int(v)+1)
			}
			for range p.numTileRows - 1 {
				v, e := r.ue()
				if e != nil {
					return p, e
				}
				p.rowHeights = append(p.rowHeights, int(v)+1)
			}
		}
		flag, err = r.u1()
		if err != nil {
			return p, err
		}
		p.acrossTiles = flag != 0
	}
	flag, err = r.u1()
	if err != nil {
		return p, err
	}
	p.acrossSlices = flag != 0
	flag, err = r.u1()
	if err != nil {
		return p, err
	}
	if flag != 0 {
		flag, err = r.u1()
		if err != nil {
			return p, err
		}
		p.deblockOverrideEnabled = flag != 0
		flag, err = r.u1()
		if err != nil {
			return p, err
		}
		p.deblockDisabled = flag != 0
		if !p.deblockDisabled {
			if p.betaOffsetDiv2, err = r.se(); err != nil {
				return p, err
			}
			if p.tcOffsetDiv2, err = r.se(); err != nil {
				return p, err
			}
		}
	}
	flag, err = r.u1()
	if err != nil {
		return p, err
	}
	if flag != 0 {
		d := defaultScalingList()
		if err = parseScalingList(r, &d); err != nil {
			return p, err
		}
		p.scaling = &d
	}
	if _, err = r.u1(); err != nil {
		return p, err
	}
	if _, err = r.ue(); err != nil {
		return p, err
	}
	flag, err = r.u1()
	if err != nil {
		return p, err
	}
	p.sliceHeaderExtension = flag != 0
	if r.moreRBSPData() {
		flag, err = r.u1()
		if err != nil {
			return p, err
		}
		if flag != 0 {
			if err = parsePPSExtensions(r, &p); err != nil {
				return p, err
			}
		}
	}
	return p, nil
}
func parsePPSExtensions(r *bitReader, p *pps) error {
	rangeExt, err := r.u1()
	if err != nil {
		return err
	}
	other, err := r.u(3)
	if err != nil {
		return err
	}
	if err = r.skip(4); err != nil {
		return err
	}
	if rangeExt != 0 {
		if p.transformSkip {
			v, e := r.ue()
			if e != nil {
				return e
			}
			p.log2MaxTransformSkip = int(v) + 2
		}
		flag, e := r.u1()
		if e != nil {
			return e
		}
		if flag != 0 {
			return unsupported("cross_component_prediction_enabled_flag")
		}
		flag, e = r.u1()
		if e != nil {
			return e
		}
		p.chromaQPOffsetListEnabled = flag != 0
		if p.chromaQPOffsetListEnabled {
			if p.diffCUChromaQPOffsetDepth, err = r.ue(); err != nil {
				return err
			}
			n, e := r.ue()
			if e != nil {
				return e
			}
			if n >= 6 {
				return malformed("chroma_qp_offset_list_len_minus1 > 5")
			}
			for range n + 1 {
				a, e := r.se()
				if e != nil {
					return e
				}
				b, e := r.se()
				if e != nil {
					return e
				}
				p.cbQPOffsetList = append(p.cbQPOffsetList, a)
				p.crQPOffsetList = append(p.crQPOffsetList, b)
			}
		}
		if p.saoOffsetScaleLuma, err = r.ue(); err != nil {
			return err
		}
		if p.saoOffsetScaleChroma, err = r.ue(); err != nil {
			return err
		}
	}
	if other != 0 {
		return unsupported("PPS multilayer, 3D or SCC extension")
	}
	return nil
}
