package heic

import (
	"os"
	"testing"
)

func TestMosaicMatchesComposedFrame(t *testing.T) {
	data, err := os.ReadFile("../../note/heic-rs/tests/fixtures/photo-2048.heic")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := openContext(data)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ctx.props(ctx.meta.primary)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeItem(&ctx, ctx.meta.primary, &p, 1)
	if err != nil {
		t.Fatal(err)
	}
	composed, err := decoded.intoFrame(defaultMaxPixels)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newMosaic(*decoded.grid, decoded.tiles)
	if err != nil {
		t.Fatal(err)
	}
	n := defaultNCLX()
	if p.nclx != nil {
		n = *p.nclx
	}
	fromTiles, err := convertSource(&rowSource{mosaic: &m}, nil, n, layoutRGB8, defaultMaxPixels)
	if err != nil {
		t.Fatal(err)
	}
	fromFrame, err := convertFrame(&composed, nil, n, layoutRGB8, defaultMaxPixels)
	if err != nil {
		t.Fatal(err)
	}
	for i := range fromTiles.data {
		if fromTiles.data[i] != fromFrame.data[i] {
			t.Fatalf("first difference at %d: mosaic %d, composed %d", i, fromTiles.data[i], fromFrame.data[i])
		}
	}
}
