package heic

var cosineTable = [33]int32{0, 90, 90, 90, 89, 88, 87, 85, 83, 82, 80, 78, 75, 73, 70, 67, 64, 61, 57, 54, 50, 46, 43, 38, 36, 31, 25, 22, 18, 13, 9, 4, 0}
var dctMatrix [32][32]int32
var dct4Flat [16]int32
var dct8Flat [64]int32
var dct16Flat [256]int32
var dct32Matrix [1024]int32
var dstMatrix = [4][4]int32{{29, 55, 74, 84}, {74, 74, 0, -74}, {84, -29, -74, 55}, {55, -84, 74, -29}}
var dstFlat = [16]int32{29, 55, 74, 84, 74, 74, 0, -74, 84, -29, -74, 55, 55, -84, 74, -29}

func init() {
	for n := range 32 {
		dctMatrix[0][n] = 64
	}
	for k := 1; k < 32; k++ {
		for n := range 32 {
			j := (k * (2*n + 1)) % 128
			if j > 64 {
				j = 128 - j
			}
			if j <= 32 {
				dctMatrix[k][n] = cosineTable[j]
			} else {
				dctMatrix[k][n] = -cosineTable[64-j]
			}
		}
	}
	for _, spec := range []struct {
		n   int
		dst []int32
	}{{4, dct4Flat[:]}, {8, dct8Flat[:]}, {16, dct16Flat[:]}, {32, dct32Matrix[:]}} {
		for k := 0; k < spec.n; k++ {
			for i := 0; i < spec.n; i++ {
				spec.dst[k*spec.n+i] = dctMatrix[k*(32/spec.n)][i]
			}
		}
	}
}

func transformMatrix(n int, trType uint8) []int32 {
	if trType == 1 {
		return dstFlat[:]
	}
	switch n {
	case 4:
		return dct4Flat[:]
	case 8:
		return dct8Flat[:]
	case 16:
		return dct16Flat[:]
	default:
		return dct32Matrix[:]
	}
}

func coefficientExtent(b []int32, n int) (int, int) {
	if n == 0 || n > 32 {
		return n, n
	}
	var masks [32]int32
	rows := 0
	for y := 0; y < n && y*n < len(b); y++ {
		any := int32(0)
		for x := 0; x < n && y*n+x < len(b); x++ {
			v := b[y*n+x]
			masks[x] |= v
			any |= v
		}
		if any != 0 {
			rows = y + 1
		}
	}
	cols := 0
	for x := range n {
		if masks[x] != 0 {
			cols = x + 1
		}
	}
	return rows, cols
}

var levelScale = [6]int64{40, 45, 51, 57, 64, 72}

func dequantize(levels []int32, n int, qp int32, bitDepth uint8, factors []uint8) {
	if n == 0 {
		return
	}
	log2n := 0
	for 1<<log2n < n {
		log2n++
	}
	bdShift := int32(bitDepth) + int32(log2n) - 5
	if bdShift < 1 || bdShift > 40 || len(levels) < n*n {
		return
	}
	rows, cols := coefficientExtent(levels, n)
	qp = max(qp, 0)
	ls := levelScale[qp%6]
	shift := uint(qp / 6)
	round := int64(1) << (bdShift - 1)
	for y := range rows {
		for x := range cols {
			var t int64
			if factors == nil {
				t = ((int64(levels[y*n+x]) * ls) << 4) << shift
			} else {
				if len(factors) < n*n {
					return
				}
				t = (int64(levels[y*n+x]) * ls * int64(factors[y*n+x])) << shift
			}
			t += round
			v := t >> bdShift
			levels[y*n+x] = int32(max(-32768, min(v, 32767)))
		}
	}
}
func matrixAt(n, k, i int, trType uint8) int32 {
	if trType == 1 {
		return dstMatrix[k][i]
	}
	return dctMatrix[k*(32/n)][i]
}
func inverseTransform(block []int32, n int, trType uint8, bitDepth uint8) {
	if bitDepth < 8 || bitDepth > 16 || (n != 4 && n != 8 && n != 16 && n != 32) || len(block) < n*n || (trType == 1 && n != 4) {
		return
	}
	if n == 4 {
		inverseTransform4(block[:16], trType, uint(20-bitDepth))
		return
	}
	if n == 8 {
		inverseTransform8(block[:64], uint(20-bitDepth))
		return
	}
	bdShift := uint(20 - bitDepth)
	m := transformMatrix(n, trType)
	bounded := n > 4
	rows, cols := n, n
	if bounded {
		rows, cols = coefficientExtent(block, n)
	}
	rnd := int32(1) << (bdShift - 1)
	if bounded && rows == 1 {
		var g0 [32]int32
		for x := range cols {
			g0[x] = max(-32768, min((m[0]*block[x]+64)>>7, 32767))
		}
		var row [32]int32
		for i := range n {
			acc := int32(0)
			for j := 0; j < cols; j++ {
				c := g0[j]
				if c != 0 {
					acc += m[j*n+i] * c
				}
			}
			row[i] = (acc + rnd) >> bdShift
		}
		for y := range n {
			copy(block[y*n:(y+1)*n], row[:n])
		}
		return
	}
	var g [32][32]int32
	for x := range cols {
		for y := range n {
			acc := int32(0)
			for j := range rows {
				c := block[j*n+x]
				if !bounded || c != 0 {
					acc += m[j*n+y] * c
				}
			}
			g[y][x] = max(-32768, min((acc+64)>>7, 32767))
		}
	}
	for y := range n {
		if bounded && cols == 1 {
			v := (m[0]*g[y][0] + rnd) >> bdShift
			for x := range n {
				block[y*n+x] = v
			}
			continue
		}
		for i := range n {
			acc := int32(0)
			for j := range cols {
				c := g[y][j]
				if !bounded || c != 0 {
					acc += m[j*n+i] * c
				}
			}
			block[y*n+i] = (acc + rnd) >> bdShift
		}
	}
}
func inverseTransform8(block []int32, bdShift uint) {
	const n = 8
	m := dct8Flat[:]
	rows, cols := coefficientExtent(block, n)
	rnd := int32(1) << (bdShift - 1)
	if rows == 1 {
		var g0 [n]int32
		for x := 0; x < cols; x++ {
			g0[x] = max(-32768, min((m[0]*block[x]+64)>>7, 32767))
		}
		var row [n]int32
		for i := 0; i < n; i++ {
			acc := int32(0)
			for j := 0; j < cols; j++ {
				if g0[j] != 0 {
					acc += m[j*n+i] * g0[j]
				}
			}
			row[i] = (acc + rnd) >> bdShift
		}
		for y := 0; y < n; y++ {
			copy(block[y*n:(y+1)*n], row[:])
		}
		return
	}
	var g [n * n]int32
	for x := 0; x < cols; x++ {
		for y := 0; y < n; y++ {
			acc := int32(0)
			for j := 0; j < rows; j++ {
				c := block[j*n+x]
				if c != 0 {
					acc += m[j*n+y] * c
				}
			}
			g[y*n+x] = max(-32768, min((acc+64)>>7, 32767))
		}
	}
	for y := 0; y < n; y++ {
		if cols == 1 {
			v := (m[0]*g[y*n] + rnd) >> bdShift
			for x := 0; x < n; x++ {
				block[y*n+x] = v
			}
			continue
		}
		for i := 0; i < n; i++ {
			acc := int32(0)
			for j := 0; j < cols; j++ {
				c := g[y*n+j]
				if c != 0 {
					acc += m[j*n+i] * c
				}
			}
			block[y*n+i] = (acc + rnd) >> bdShift
		}
	}
}

func inverseTransform4(block []int32, trType uint8, bdShift uint) {
	m := dct4Flat[:]
	if trType == 1 {
		m = dstFlat[:]
	}
	var g [16]int32
	for x := 0; x < 4; x++ {
		for i := 0; i < 4; i++ {
			acc := m[i]*block[x] + m[4+i]*block[4+x] + m[8+i]*block[8+x] + m[12+i]*block[12+x]
			g[4*i+x] = max(-32768, min((acc+64)>>7, 32767))
		}
	}
	rnd := int32(1) << (bdShift - 1)
	for y := 0; y < 4; y++ {
		for i := 0; i < 4; i++ {
			acc := m[i]*g[4*y] + m[4+i]*g[4*y+1] + m[8+i]*g[4*y+2] + m[12+i]*g[4*y+3]
			block[4*y+i] = (acc + rnd) >> bdShift
		}
	}
}

func transformSkip(block []int32, n int, bitDepth uint8, rotate bool) {
	if bitDepth < 8 || bitDepth > 16 || len(block) < n*n {
		return
	}
	log2n := 0
	switch n {
	case 4:
		log2n = 2
	case 8:
		log2n = 3
	case 16:
		log2n = 4
	case 32:
		log2n = 5
	default:
		return
	}
	if rotate {
		for i, j := 0, n*n-1; i < j; i, j = i+1, j-1 {
			block[i], block[j] = block[j], block[i]
		}
	}
	bdShift := uint(20 - bitDepth)
	tsShift := uint(5 + log2n)
	rnd := int32(1) << (bdShift - 1)
	for i := range n * n {
		block[i] = (block[i]<<tsShift + rnd) >> bdShift
	}
}
