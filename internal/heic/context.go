package heic

import "slices"

type context struct {
	file []byte
	ftyp fileType
	meta meta
}

func openContext(data []byte) (context, error) {
	ft, err := parseFileType(data)
	if err != nil {
		return context{}, err
	}
	m, err := parseMeta(data)
	if err != nil {
		return context{}, err
	}
	return context{file: data, ftyp: ft, meta: m}, nil
}
func (c *context) props(item uint32) (itemProps, error) { return gatherProps(&c.meta.props, item) }
func (c *context) itemData(item uint32) ([]byte, error) { return c.meta.itemData(c.file, item) }
func (c *context) gridForItem(item uint32) (grid, []uint32, bool, error) {
	info := c.meta.item(item)
	if info == nil {
		return grid{}, nil, false, missingItem(item)
	}
	if !info.isGrid() {
		if info.itemType == "iovl" {
			return grid{}, nil, false, unsupported("`iovl` overlay derivation is not implemented; only `grid` is")
		}
		return grid{}, nil, false, nil
	}
	data, err := c.itemData(item)
	if err != nil {
		return grid{}, nil, false, err
	}
	g, err := parseGrid(data)
	if err != nil {
		return grid{}, nil, false, err
	}
	tiles := referenceTargets(c.meta.refs, item, refDIMG)
	if uint64(len(tiles)) != g.tileCount() {
		return grid{}, nil, false, malformed("the grid's tile count disagrees with the length of its dimg reference")
	}
	if slices.Contains(tiles, item) {
		return grid{}, nil, false, cyclicDerivation(item)
	}
	for _, t := range tiles {
		ti := c.meta.item(t)
		if ti == nil {
			return grid{}, nil, false, missingItem(t)
		}
		if ti.isGrid() || len(referenceTargets(c.meta.refs, t, refDIMG)) > 0 {
			return grid{}, nil, false, cyclicDerivation(t)
		}
	}
	return g, tiles, true, nil
}
func (c *context) codedSize(item uint32, p *itemProps) (uint32, uint32, error) {
	if p.ispe != nil {
		return p.ispe.width, p.ispe.height, nil
	}
	if g, _, ok, err := c.gridForItem(item); err != nil {
		return 0, 0, err
	} else if ok {
		return g.outputWidth, g.outputHeight, nil
	}
	if p.hvcc == nil {
		return 0, 0, missingBox("ispe")
	}
	info, err := probeHEVC(p.hvcc.parameterSets())
	return info.width, info.height, err
}
func (c *context) alphaItem(item uint32) (uint32, bool, error) {
	for _, r := range c.meta.refs {
		if r.kind != refAUXL || !slices.Contains(r.to, item) {
			continue
		}
		p, err := c.props(r.from)
		if err != nil {
			return 0, false, err
		}
		if p.auxc != nil && p.auxc.kind == auxAlpha {
			return r.from, true, nil
		}
	}
	return 0, false, nil
}
func (c *context) describedBy(item uint32, kind string) (uint32, bool) {
	for _, r := range c.meta.refs {
		if r.kind == refCDSC && slices.Contains(r.to, item) {
			if info := c.meta.item(r.from); info != nil && info.itemType == kind {
				return r.from, true
			}
		}
	}
	return 0, false
}
func (c *context) exif(item uint32) ([]byte, bool, error) {
	id, ok := c.describedBy(item, "Exif")
	if !ok {
		return nil, false, nil
	}
	data, err := c.itemData(id)
	if err != nil {
		return nil, false, err
	}
	out, err := exifTIFF(data)
	return out, true, err
}
func (c *context) primaryEXIF() ([]byte, error) {
	data, ok, err := c.exif(c.meta.primary)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, missingItem(0)
	}
	return data, nil
}
