package heic

type parameterSets struct {
	vps []uint32
	sps []sps
	pps []pps
}

func parseParameterSets(nals [][]byte) (parameterSets, error) {
	var out parameterSets
	for _, nal := range nals {
		h, err := parseNALHeader(nal)
		if err != nil {
			return out, err
		}
		if h.layerID != 0 {
			continue
		}
		r, err := rbspFromNAL(nal)
		if err != nil {
			return out, err
		}
		switch h.nalUnitType {
		case nalVPS:
			id, err := parseVPS(r.data)
			if err != nil {
				return out, err
			}
			for i := range out.vps {
				if out.vps[i] == id {
					out.vps = append(out.vps[:i], out.vps[i+1:]...)
					break
				}
			}
			out.vps = append(out.vps, id)
		case nalSPS:
			s, err := parseSPS(r.data)
			if err != nil {
				return out, err
			}
			for i := range out.sps {
				if out.sps[i].id == s.id {
					out.sps = append(out.sps[:i], out.sps[i+1:]...)
					break
				}
			}
			out.sps = append(out.sps, s)
		case nalPPS:
			p, err := parsePPS(r.data)
			if err != nil {
				return out, err
			}
			for i := range out.pps {
				if out.pps[i].id == p.id {
					out.pps = append(out.pps[:i], out.pps[i+1:]...)
					break
				}
			}
			out.pps = append(out.pps, p)
		}
	}
	return out, nil
}
func (s *parameterSets) getPPS(id uint32) (*pps, error) {
	for i := range s.pps {
		if s.pps[i].id == id {
			return &s.pps[i], nil
		}
	}
	return nil, missingParameterSet("PPS")
}
func (s *parameterSets) getSPS(id uint32) (*sps, error) {
	for i := range s.sps {
		if s.sps[i].id == id {
			return &s.sps[i], nil
		}
	}
	return nil, missingParameterSet("SPS")
}
func (s *parameterSets) firstSPS() (*sps, error) {
	if len(s.pps) > 0 {
		return s.getSPS(s.pps[0].spsID)
	}
	if len(s.sps) > 0 {
		return &s.sps[0], nil
	}
	return nil, missingParameterSet("SPS")
}
