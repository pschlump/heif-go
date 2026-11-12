package heic

import "testing"

type testRNG uint32

func (r *testRNG) next() uint32 {
	x := uint32(*r)
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	*r = testRNG(x)
	return x
}
func (r *testRNG) coeff() int32 { return int32(r.next()%65536) - 32768 }

func referenceInverse(d []int32, n int, trType uint8, bitDepth uint8) []int32 {
	g := make([]int64, n*n)
	for x := 0; x < n; x++ {
		for i := 0; i < n; i++ {
			var sum int64
			for j := 0; j < n; j++ {
				sum += int64(matrixAt(n, j, i, trType)) * int64(d[j*n+x])
			}
			v := (sum + 64) >> 7
			g[i*n+x] = max(-32768, min(v, 32767))
		}
	}
	bd := int64(20 - bitDepth)
	out := make([]int32, n*n)
	for y := 0; y < n; y++ {
		for i := 0; i < n; i++ {
			var sum int64
			for j := 0; j < n; j++ {
				sum += int64(matrixAt(n, j, i, trType)) * g[y*n+j]
			}
			out[y*n+i] = int32((sum + (1 << (bd - 1))) >> bd)
		}
	}
	return out
}

func TestInverseTransformMatchesReference(t *testing.T) {
	rng := testRNG(0x12345679)
	for _, n := range []int{4, 8, 16, 32} {
		for _, trType := range []uint8{0, 1} {
			if trType == 1 && n != 4 {
				continue
			}
			for _, depth := range []uint8{8, 10} {
				for iteration := 0; iteration < 50; iteration++ {
					d := make([]int32, n*n)
					for i := range d {
						d[i] = rng.coeff()
					}
					want := referenceInverse(d, n, trType, depth)
					got := append([]int32(nil), d...)
					inverseTransform(got, n, trType, depth)
					for i := range got {
						if got[i] != want[i] {
							t.Fatalf("n=%d type=%d depth=%d iteration=%d index=%d: got %d want %d", n, trType, depth, iteration, i, got[i], want[i])
						}
					}
				}
			}
		}
	}
}

func TestDCOnlyInverseTransformIsFlat(t *testing.T) {
	for _, n := range []int{4, 8, 16, 32} {
		for _, v := range []int32{1, -1, 100, -4096, 32767, -32768} {
			b := make([]int32, n*n)
			b[0] = v
			inverseTransform(b, n, 0, 8)
			g := max(-32768, min((64*v+64)>>7, 32767))
			want := (64*g + 1<<11) >> 12
			for i, got := range b {
				if got != want {
					t.Fatalf("n=%d v=%d index=%d: got %d want %d", n, v, i, got, want)
				}
			}
		}
	}
}

func TestTransformLayouts(t *testing.T) {
	steps := decodedImage{data: []byte{0, 1, 2, 10, 11, 12}, width: 3, height: 2, layout: layoutGray8}
	rotated, err := applyTransforms(steps, []transform{{kind: transformRotate, rot: rotateCCW90}})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{2, 12, 1, 11, 0, 10}
	if rotated.width != 2 || rotated.height != 3 {
		t.Fatalf("rotated size = %dx%d", rotated.width, rotated.height)
	}
	for i := range want {
		if rotated.data[i] != want[i] {
			t.Fatalf("rotation[%d] = %d, want %d", i, rotated.data[i], want[i])
		}
	}
	mirrored, err := applyTransforms(steps, []transform{{kind: transformMirror, mir: mirrorLeftRight}})
	if err != nil {
		t.Fatal(err)
	}
	want = []byte{2, 1, 0, 12, 11, 10}
	for i := range want {
		if mirrored.data[i] != want[i] {
			t.Fatalf("mirror[%d] = %d, want %d", i, mirrored.data[i], want[i])
		}
	}
}
