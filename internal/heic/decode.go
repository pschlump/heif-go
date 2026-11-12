package heic

import (
	"runtime"
	"sync"
)

type decodeOptions struct {
	layout                               pixelLayout
	maxPixels                            uint64
	applyTransforms, decodeAlpha, strict bool
	threads                              int
}

func defaultDecodeOptions() decodeOptions {
	return decodeOptions{layout: layoutRGB8, maxPixels: defaultMaxPixels, applyTransforms: true, decodeAlpha: true, threads: runtime.GOMAXPROCS(0)}
}

type decodedItem struct {
	picture *frame
	grid    *grid
	tiles   []frame
}

func decodeBytes(data []byte, o decodeOptions) (decodedImage, error) {
	ctx, err := openContext(data)
	if err != nil {
		return decodedImage{}, err
	}
	id := ctx.meta.primary
	p, err := ctx.props(id)
	if err != nil {
		return decodedImage{}, err
	}
	if o.strict {
		if _, ok := ctx.meta.props.unknownEssential(id); ok {
			return decodedImage{}, unsupported("the primary item carries an essential property this build does not implement")
		}
	}
	cw, ch, err := ctx.codedSize(id, &p)
	if err != nil {
		return decodedImage{}, err
	}
	if _, err = checkPixels(cw, ch, o.maxPixels); err != nil {
		return decodedImage{}, err
	}
	tw, th, err := transformedSize(cw, ch, p.transforms)
	if err != nil {
		return decodedImage{}, err
	}
	if _, err = checkPixels(tw, th, o.maxPixels); err != nil {
		return decodedImage{}, err
	}
	decoded, err := decodeItem(&ctx, id, &p, o.threads)
	if err != nil {
		return decodedImage{}, err
	}
	var alpha *frame
	if o.decodeAlpha && o.layout.hasAlpha() {
		if aux, ok, err := ctx.alphaItem(id); err != nil {
			return decodedImage{}, err
		} else if ok {
			ap, err := ctx.props(aux)
			if err != nil {
				return decodedImage{}, err
			}
			ad, err := decodeItem(&ctx, aux, &ap, o.threads)
			if err != nil {
				return decodedImage{}, err
			}
			af, err := ad.intoFrame(o.maxPixels)
			if err != nil {
				return decodedImage{}, err
			}
			alpha = &af
		}
	}
	n := defaultNCLX()
	if p.nclx != nil {
		n = *p.nclx
	}
	var src rowSource
	if decoded.picture != nil {
		src.frame = decoded.picture
	} else {
		m, err := newMosaic(*decoded.grid, decoded.tiles)
		if err != nil {
			return decodedImage{}, err
		}
		src.mosaic = &m
	}
	img, err := convertSource(&src, alpha, n, o.layout, o.maxPixels)
	if err != nil {
		return img, err
	}
	if o.applyTransforms {
		return applyTransforms(img, p.transforms)
	}
	return img, nil
}
func (d decodedItem) intoFrame(limit uint64) (frame, error) {
	if d.picture != nil {
		return *d.picture, nil
	}
	return composeGrid(*d.grid, d.tiles, limit)
}
func decodeItem(ctx *context, id uint32, p *itemProps, threads int) (decodedItem, error) {
	g, tiles, ok, err := ctx.gridForItem(id)
	if err != nil {
		return decodedItem{}, err
	}
	if !ok {
		f, err := decodeCodedItem(ctx, id, p)
		if err != nil {
			return decodedItem{}, err
		}
		return decodedItem{picture: &f}, nil
	}
	frames, err := decodeTiles(ctx, tiles, threads)
	if err != nil {
		return decodedItem{}, err
	}
	return decodedItem{grid: &g, tiles: frames}, nil
}
func decodeTiles(ctx *context, tiles []uint32, threads int) ([]frame, error) {
	frames := make([]frame, len(tiles))
	if threads <= 1 || len(tiles) < 2 {
		for i, id := range tiles {
			p, err := ctx.props(id)
			if err != nil {
				return nil, err
			}
			f, err := decodeCodedItem(ctx, id, &p)
			if err != nil {
				return nil, err
			}
			frames[i] = f
		}
		return frames, nil
	}
	workers := min(threads, len(tiles))
	jobs := make(chan int)
	errs := make([]error, len(tiles))
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for i := range jobs {
				p, err := ctx.props(tiles[i])
				if err == nil {
					frames[i], err = decodeCodedItem(ctx, tiles[i], &p)
				}
				errs[i] = err
			}
		}()
	}
	for i := range tiles {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return frames, nil
}
func decodeCodedItem(ctx *context, id uint32, p *itemProps) (frame, error) {
	if p.hvcc == nil {
		return frame{}, missingBox("hvcC")
	}
	data, err := ctx.itemData(id)
	if err != nil {
		return frame{}, err
	}
	nals, err := p.hvcc.splitNALs(data)
	if err != nil {
		return frame{}, err
	}
	params := p.hvcc.parameterSets()
	if len(params) == 0 {
		return frame{}, malformed("hvcC carries no parameter sets")
	}
	f, err := decodeStill(params, nals)
	if err != nil {
		return frame{}, err
	}
	if err = f.validate(); err != nil {
		return frame{}, err
	}
	return f, nil
}
func probeBytes(data []byte) (codecInfo, error) {
	ctx, err := openContext(data)
	if err != nil {
		return codecInfo{}, err
	}
	p, err := ctx.props(ctx.meta.primary)
	if err != nil {
		return codecInfo{}, err
	}
	w, h, err := ctx.codedSize(ctx.meta.primary, &p)
	if err != nil {
		return codecInfo{}, err
	}
	w, h, err = transformedSize(w, h, p.transforms)
	info := codecInfo{width: w, height: h}
	if depth, ok := p.bitDepth(); ok {
		info.bitDepth = depth
	}
	return info, err
}
