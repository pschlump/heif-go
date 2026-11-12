package heic

var defaultIntraScaling = [64]uint8{16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 17, 16, 17, 16, 17, 18, 17, 18, 18, 17, 18, 21, 19, 20, 21, 20, 19, 21, 24, 22, 22, 24, 24, 22, 22, 24, 25, 25, 27, 30, 27, 25, 25, 29, 31, 35, 35, 31, 29, 36, 41, 44, 41, 36, 47, 54, 54, 47, 65, 70, 65, 88, 88, 115}
var defaultInterScaling = [64]uint8{16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 17, 17, 17, 17, 17, 18, 18, 18, 18, 18, 18, 20, 20, 20, 20, 20, 20, 20, 24, 24, 24, 24, 24, 24, 24, 24, 25, 25, 25, 25, 25, 25, 25, 28, 28, 28, 28, 28, 28, 33, 33, 33, 33, 33, 41, 41, 41, 41, 54, 54, 54, 71, 71, 91}

type scalingListData struct {
	lists [4][6][64]uint8
	dc    [2][6]uint8
}

func defaultScalingList() scalingListData {
	var d scalingListData
	for s := range 4 {
		for m := range 6 {
			for i := range 64 {
				d.lists[s][m][i] = 16
			}
			if s > 0 {
				if m < 3 {
					d.lists[s][m] = defaultIntraScaling
				} else {
					d.lists[s][m] = defaultInterScaling
				}
			}
		}
	}
	for s := range 2 {
		for m := range 6 {
			d.dc[s][m] = 16
		}
	}
	return d
}
func parseScalingList(r *bitReader, d *scalingListData) error {
	for sizeID := range 4 {
		step := 1
		if sizeID == 3 {
			step = 3
		}
		for matrixID := 0; matrixID < 6; matrixID += step {
			coefNum := 64
			if sizeID == 0 {
				coefNum = 16
			}
			flag, err := r.u1()
			if err != nil {
				return err
			}
			if flag == 0 {
				delta, err := r.ue()
				if err != nil {
					return err
				}
				if delta == 0 {
					if matrixID < 3 {
						d.lists[sizeID][matrixID] = defaultIntraScaling
					} else {
						d.lists[sizeID][matrixID] = defaultInterScaling
					}
					if sizeID > 1 {
						d.dc[sizeID-2][matrixID] = 16
					}
				} else {
					src := matrixID - int(delta)*step
					if src < 0 {
						return malformed("scaling_list_pred_matrix_id_delta")
					}
					d.lists[sizeID][matrixID] = d.lists[sizeID][src]
					if sizeID > 1 {
						d.dc[sizeID-2][matrixID] = d.dc[sizeID-2][src]
					}
				}
			} else {
				next := int32(8)
				if sizeID > 1 {
					v, e := r.se()
					if e != nil {
						return e
					}
					dc := v + 8
					if dc < 1 || dc > 255 {
						return malformed("scaling_list_dc_coef out of range")
					}
					d.dc[sizeID-2][matrixID] = uint8(dc)
					next = dc
				}
				for i := range coefNum {
					delta, e := r.se()
					if e != nil {
						return e
					}
					next = (next + delta + 256) % 256
					if next < 0 {
						next += 256
					}
					d.lists[sizeID][matrixID][i] = uint8(next)
				}
			}
		}
	}
	for sizeID := range 4 {
		n := 64
		if sizeID == 0 {
			n = 16
		}
		for m := range 6 {
			for i := range n {
				if d.lists[sizeID][m][i] == 0 {
					return malformed("scaling list contains a zero coefficient")
				}
			}
		}
	}
	return nil
}

type scalingFactors struct {
	f4  [6][16]uint8
	f8  [6][64]uint8
	f16 [6][256]uint8
	f32 [6][1024]uint8
}

func (s *scalingFactors) get(log2Size, matrixID int) []uint8 {
	switch log2Size {
	case 2:
		return s.f4[matrixID][:]
	case 3:
		return s.f8[matrixID][:]
	case 4:
		return s.f16[matrixID][:]
	default:
		return s.f32[matrixID][:]
	}
}
func deriveScalingFactors(d *scalingListData, chromaArrayType uint8) *scalingFactors {
	sf := new(scalingFactors)
	s4, s8 := scanOrder(2, 0), scanOrder(3, 0)
	for m := range 6 {
		for i := range 16 {
			p := s4[i]
			sf.f4[m][int(p.y)*4+int(p.x)] = d.lists[0][m][i]
		}
		for i := range 64 {
			p := s8[i]
			sf.f8[m][int(p.y)*8+int(p.x)] = d.lists[1][m][i]
		}
		upsampleScaling(&d.lists[2][m], sf.f16[m][:], 16, 2, s8)
		sf.f16[m][0] = d.dc[0][m]
		src, dc := &d.lists[2][m], d.dc[0][m]
		if m == 0 || m == 3 || chromaArrayType != 3 {
			idx := 0
			if m >= 3 {
				idx = 3
			}
			src = &d.lists[3][idx]
			dc = d.dc[1][idx]
		}
		upsampleScaling(src, sf.f32[m][:], 32, 4, s8)
		sf.f32[m][0] = dc
	}
	return sf
}
func upsampleScaling(list *[64]uint8, out []uint8, n, f int, scan *scanTable) {
	for i := range 64 {
		p := scan[i]
		x, y := int(p.x)*f, int(p.y)*f
		v := list[i]
		for j := range f {
			row := (y+j)*n + x
			for k := range f {
				out[row+k] = v
			}
		}
	}
}
