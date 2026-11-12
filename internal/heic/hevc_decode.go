package heic

func decodeStill(parameterSets, slices [][]byte) (frame, error) {
	sets, err := parseParameterSets(parameterSets)
	if err != nil {
		return frame{}, err
	}
	return decodePicture(&sets, slices)
}
func decodePicture(sets *parameterSets, slices [][]byte) (frame, error) {
	if len(slices) == 0 {
		return frame{}, malformed("the coded item carries no slice NAL units")
	}
	type parsedSlice struct {
		h nalHeader
		r rbsp
	}
	parsed := make([]parsedSlice, 0, len(slices))
	for _, nal := range slices {
		h, err := parseNALHeader(nal)
		if err != nil {
			return frame{}, err
		}
		if !h.isVCL() || h.layerID != 0 {
			continue
		}
		if !h.isIRAP() {
			return frame{}, unsupported("non-IRAP picture (inter prediction)")
		}
		r, err := rbspFromNAL(nal)
		if err != nil {
			return frame{}, err
		}
		parsed = append(parsed, parsedSlice{h, r})
	}
	if len(parsed) == 0 {
		return frame{}, malformed("the coded item carries no slice NAL units")
	}
	br := newBitReader(parsed[0].r.data)
	if _, err := br.u1(); err != nil {
		return frame{}, err
	}
	if parsed[0].h.isIRAP() {
		if _, err := br.u1(); err != nil {
			return frame{}, err
		}
	}
	ppsID, err := br.ue()
	if err != nil {
		return frame{}, err
	}
	p, err := sets.getPPS(ppsID)
	if err != nil {
		return frame{}, err
	}
	s, err := sets.getSPS(p.spsID)
	if err != nil {
		return frame{}, err
	}
	if s.implicitRDPCM {
		return frame{}, unsupported("implicit_rdpcm_enabled_flag")
	}
	g, err := newGeometry(s, p)
	if err != nil {
		return frame{}, err
	}
	pic, err := newPicture(s.width, s.height, s.subW, s.subH)
	if err != nil {
		return frame{}, err
	}
	factors := derivePictureScaling(s, p)
	sliceAddr := make([]uint32, g.sizeCTBs)
	for i := range sliceAddr {
		sliceAddr[i] = ^uint32(0)
	}
	fi := filterInfo{bitDepthY: s.bitDepthY, bitDepthC: s.bitDepthC, chromaArrayType: s.chromaArrayType, subW: s.subW, subH: s.subH, ctbLog2: s.log2CTB, picWCTBs: g.wCTBs, picHCTBs: g.hCTBs, pcmLoopFilterDisabled: s.pcmLoopFilterDisabled, acrossTiles: p.acrossTiles, ctb: make([]ctbFilter, g.sizeCTBs), tileID: append([]uint16(nil), g.tileID...), sliceAddr: sliceAddr}
	for i := range fi.ctb {
		fi.ctb[i] = defaultCTBFilter()
	}
	for _, ps := range parsed {
		h, err := parseSliceHeader(&ps.h, ps.r.data, s, p)
		if err != nil {
			return frame{}, err
		}
		if h.ppsID != p.id {
			return frame{}, unsupported("a picture using more than one PPS")
		}
		fi.saoEnabled = fi.saoEnabled || h.saoLuma || h.saoChroma
		if err = decodeSlice(s, p, &g, factors, &pic, &fi, h, &ps.r); err != nil {
			return frame{}, err
		}
	}
	for _, a := range fi.sliceAddr {
		if a == ^uint32(0) {
			return frame{}, malformed("picture is not fully covered by slices")
		}
	}
	deblockPicture(&pic, &fi)
	applySAO(&pic, &fi)
	return cropPicture(&pic, s), nil
}
func derivePictureScaling(s *sps, p *pps) *scalingFactors {
	if !s.scalingListEnabled {
		return nil
	}
	if p.scaling != nil {
		return deriveScalingFactors(p.scaling, s.chromaArrayType)
	}
	if s.scaling != nil {
		return deriveScalingFactors(s.scaling, s.chromaArrayType)
	}
	d := defaultScalingList()
	return deriveScalingFactors(&d, s.chromaArrayType)
}
func cropPicture(pic *picture, s *sps) frame {
	chroma, _ := chromaFromIDC(s.chromaArrayType)
	if s.crop == [4]int{} {
		return frame{width: uint32(s.width), height: uint32(s.height), bitDepth: s.bitDepthY, chroma: chroma, y: pic.y.data, cb: pic.cb.data, cr: pic.cr.data, yStride: uint32(pic.y.stride), cStride: uint32(pic.cb.stride)}
	}
	l, r, t, b := s.crop[0], s.crop[1], s.crop[2], s.crop[3]
	w, h := max(s.width-l-r, 1), max(s.height-t-b, 1)
	sw, sh := 1, 1
	if s.subW != 0 {
		sw, sh = s.subW, s.subH
	}
	y := pic.y.data[t*pic.y.stride+l:]
	cb := pic.cb.data[(t/sh)*pic.cb.stride+l/sw:]
	cr := pic.cr.data[(t/sh)*pic.cr.stride+l/sw:]
	return frame{width: uint32(w), height: uint32(h), bitDepth: s.bitDepthY, chroma: chroma, y: y, cb: cb, cr: cr, yStride: uint32(pic.y.stride), cStride: uint32(pic.cb.stride)}
}
func probeHEVC(parameterSets [][]byte) (codecInfo, error) {
	sets, err := parseParameterSets(parameterSets)
	if err != nil {
		return codecInfo{}, err
	}
	s, err := sets.firstSPS()
	if err != nil {
		return codecInfo{}, err
	}
	chroma, _ := chromaFromIDC(s.chromaArrayType)
	return codecInfo{width: uint32(max(s.width-s.crop[0]-s.crop[1], 1)), height: uint32(max(s.height-s.crop[2]-s.crop[3], 1)), bitDepth: s.bitDepthY, chroma: chroma}, nil
}
