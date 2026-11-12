package heic

import "math"

const maxItemDepth = 8

type itemInfo struct {
	id          uint32
	itemType    string
	name        string
	contentType string
	uriType     string
	hidden      bool
}

func (i itemInfo) isHEVC() bool { return i.itemType == "hvc1" || i.itemType == "hev1" }
func (i itemInfo) isGrid() bool { return i.itemType == "grid" }

type extent struct {
	index  uint64
	offset uint64
	length uint64
}

type construction uint8

const (
	constructionFile construction = iota
	constructionIDAT
	constructionItem
)

type itemLocation struct {
	id           uint32
	construction construction
	baseOffset   uint64
	extents      []extent
}

func (l itemLocation) totalLength() (uint64, bool) {
	var total uint64
	for _, e := range l.extents {
		if e.length == 0 || math.MaxUint64-total < e.length {
			return 0, false
		}
		total += e.length
	}
	return total, true
}

type association struct {
	index     uint16
	essential bool
}

type propertyAssoc struct {
	item uint32
	list []association
}

type properties struct {
	boxes []boxHeader
	assoc []propertyAssoc
}

func (p *properties) associations(item uint32) []association {
	for _, a := range p.assoc {
		if a.item == item {
			return a.list
		}
	}
	return nil
}

func (p *properties) resolve(a association) (boxHeader, bool) {
	if a.index == 0 || int(a.index) > len(p.boxes) {
		return boxHeader{}, false
	}
	return p.boxes[a.index-1], true
}

func (p *properties) find(item uint32, code string) (boxHeader, bool) {
	for _, a := range p.associations(item) {
		if b, ok := p.resolve(a); ok && b.is(code) {
			return b, true
		}
	}
	return boxHeader{}, false
}

func (p *properties) unknownEssential(item uint32) (string, bool) {
	known := map[string]bool{"hvcC": true, "ispe": true, "pixi": true, "colr": true, "irot": true, "imir": true, "clap": true, "auxC": true, "pasp": true}
	for _, a := range p.associations(item) {
		if b, ok := p.resolve(a); ok && a.essential && !known[b.boxType] {
			return b.boxType, true
		}
	}
	return "", false
}

type refKind uint8

const (
	refDIMG refKind = iota
	refTHMB
	refCDSC
	refAUXL
	refOther
)

type itemReference struct {
	kind  refKind
	other string
	from  uint32
	to    []uint32
}

func referenceKind(code string) (refKind, string) {
	switch code {
	case "dimg":
		return refDIMG, ""
	case "thmb":
		return refTHMB, ""
	case "cdsc":
		return refCDSC, ""
	case "auxl":
		return refAUXL, ""
	default:
		return refOther, code
	}
}

func referenceTargets(refs []itemReference, from uint32, kind refKind) []uint32 {
	for _, r := range refs {
		if r.from == from && r.kind == kind {
			return r.to
		}
	}
	return nil
}

type meta struct {
	handler   string
	primary   uint32
	items     []itemInfo
	refs      []itemReference
	locations []itemLocation
	props     properties
	idat      []byte
}

func parseINFE(b boxHeader) (itemInfo, error) {
	version, flags, r, err := b.fullBox("infe")
	if err != nil {
		return itemInfo{}, err
	}
	var out itemInfo
	out.hidden = flags&1 != 0
	if version < 2 {
		id, e := r.u16("infe item id")
		if e != nil {
			return out, e
		}
		out.id = uint32(id)
		if _, e = r.u16("infe protection index"); e != nil {
			return out, e
		}
		out.itemType = "    "
	} else {
		switch version {
		case 2:
			id, e := r.u16("infe item id")
			if e != nil {
				return out, e
			}
			out.id = uint32(id)
		case 3:
			out.id, err = r.u32("infe item id")
			if err != nil {
				return out, err
			}
		default:
			return out, unsupported("infe version above 3")
		}
		if _, err = r.u16("infe protection index"); err != nil {
			return out, err
		}
		out.itemType, err = r.fourcc("infe item type")
		if err != nil {
			return out, err
		}
	}
	out.name, err = r.cstr("infe name")
	if err != nil {
		return out, err
	}
	if version < 2 || out.itemType == "mime" {
		out.contentType, _ = r.cstr("infe content type")
	} else if out.itemType == "uri " {
		out.uriType, _ = r.cstr("infe uri type")
	}
	return out, nil
}

func parseIINF(b boxHeader) ([]itemInfo, error) {
	version, _, r, err := b.fullBox("iinf")
	if err != nil {
		return nil, err
	}
	var count uint32
	if version == 0 {
		v, e := r.u16("iinf entry count")
		count, err = uint32(v), e
	} else {
		count, err = r.u32("iinf entry count")
	}
	if err != nil {
		return nil, err
	}
	if uint64(count) > uint64(r.remaining()/8) {
		return nil, malformed("iinf declares more entries than can fit")
	}
	out := make([]itemInfo, 0, count)
	it := newBoxIter(r.rest())
	for {
		child, ok, e := it.next()
		if e != nil {
			return nil, e
		}
		if !ok {
			return out, nil
		}
		if child.is("infe") {
			info, e := parseINFE(child)
			if e != nil {
				return nil, e
			}
			out = append(out, info)
		}
	}
}

func parseILOC(b boxHeader) ([]itemLocation, error) {
	version, _, r, err := b.fullBox("iloc")
	if err != nil {
		return nil, err
	}
	if version > 2 {
		return nil, unsupported("iloc version above 2")
	}
	sizes, err := r.u8("iloc field sizes")
	if err != nil {
		return nil, err
	}
	offsetSize, lengthSize := sizes>>4, sizes&15
	sizes, err = r.u8("iloc field sizes")
	if err != nil {
		return nil, err
	}
	baseSize, indexSize := sizes>>4, uint8(0)
	if version >= 1 {
		indexSize = sizes & 15
	}
	var count uint32
	if version < 2 {
		v, e := r.u16("iloc item count")
		count, err = uint32(v), e
	} else {
		count, err = r.u32("iloc item count")
	}
	if err != nil {
		return nil, err
	}
	if uint64(count) > uint64(r.remaining()/6+1) {
		return nil, malformed("iloc declares more items than can fit")
	}
	capN := int(count)
	if capN > 4096 {
		capN = 4096
	}
	out := make([]itemLocation, 0, capN)
	for range count {
		var loc itemLocation
		if version < 2 {
			v, e := r.u16("iloc item id")
			loc.id, err = uint32(v), e
		} else {
			loc.id, err = r.u32("iloc item id")
		}
		if err != nil {
			return nil, err
		}
		if version >= 1 {
			word, e := r.u16("iloc construction method")
			if e != nil {
				return nil, e
			}
			code := uint8(word & 15)
			if code > 2 {
				return nil, unsupported("unknown iloc construction method")
			}
			loc.construction = construction(code)
		}
		if _, err = r.u16("iloc data reference index"); err != nil {
			return nil, err
		}
		loc.baseOffset, err = r.uintN(baseSize, "iloc base offset")
		if err != nil {
			return nil, err
		}
		extentCount, e := r.u16("iloc extent count")
		if e != nil {
			return nil, e
		}
		per := int(offsetSize + lengthSize + indexSize)
		if per > 0 && int(extentCount)*per > r.remaining() {
			return nil, malformed("iloc declares more extents than it holds")
		}
		loc.extents = make([]extent, 0, extentCount)
		for range extentCount {
			var ex extent
			ex.index, err = r.uintN(indexSize, "iloc extent index")
			if err == nil {
				ex.offset, err = r.uintN(offsetSize, "iloc extent offset")
			}
			if err == nil {
				ex.length, err = r.uintN(lengthSize, "iloc extent length")
			}
			if err != nil {
				return nil, err
			}
			loc.extents = append(loc.extents, ex)
		}
		out = append(out, loc)
	}
	return out, nil
}

func parseIREF(b boxHeader) ([]itemReference, error) {
	version, _, r, err := b.fullBox("iref")
	if err != nil {
		return nil, err
	}
	if version > 1 {
		return nil, unsupported("iref version above 1")
	}
	large := version == 1
	var out []itemReference
	it := newBoxIter(r.rest())
	for {
		child, ok, e := it.next()
		if e != nil {
			return nil, e
		}
		if !ok {
			return out, nil
		}
		cr := child.reader()
		var ref itemReference
		ref.kind, ref.other = referenceKind(child.boxType)
		if large {
			ref.from, err = cr.u32("iref from_item_ID")
		} else {
			var v uint16
			v, err = cr.u16("iref from_item_ID")
			ref.from = uint32(v)
		}
		if err != nil {
			return nil, err
		}
		count, e := cr.u16("iref reference count")
		if e != nil {
			return nil, e
		}
		width := 2
		if large {
			width = 4
		}
		if int(count)*width > cr.remaining() {
			return nil, malformed("iref declares more references than it holds")
		}
		ref.to = make([]uint32, 0, count)
		for range count {
			if large {
				v, e := cr.u32("iref to_item_ID")
				if e != nil {
					return nil, e
				}
				ref.to = append(ref.to, v)
			} else {
				v, e := cr.u16("iref to_item_ID")
				if e != nil {
					return nil, e
				}
				ref.to = append(ref.to, uint32(v))
			}
		}
		out = append(out, ref)
	}
}

func parseIPMA(b boxHeader, out *[]propertyAssoc) error {
	version, flags, r, err := b.fullBox("ipma")
	if err != nil {
		return err
	}
	wide := flags&1 != 0
	count, err := r.u32("ipma entry count")
	if err != nil {
		return err
	}
	if uint64(count) > uint64(r.remaining()/3+1) {
		return malformed("ipma declares more entries than can fit")
	}
	for range count {
		var item uint32
		if version < 1 {
			v, e := r.u16("ipma item id")
			item, err = uint32(v), e
		} else {
			item, err = r.u32("ipma item id")
		}
		if err != nil {
			return err
		}
		n, e := r.u8("ipma association count")
		if e != nil {
			return e
		}
		list := make([]association, 0, n)
		for range n {
			if wide {
				w, e := r.u16("ipma association")
				if e != nil {
					return e
				}
				list = append(list, association{index: w & 0x7fff, essential: w&0x8000 != 0})
			} else {
				w, e := r.u8("ipma association")
				if e != nil {
					return e
				}
				list = append(list, association{index: uint16(w & 0x7f), essential: w&0x80 != 0})
			}
		}
		found := false
		for i := range *out {
			if (*out)[i].item == item {
				(*out)[i].list = append((*out)[i].list, list...)
				found = true
				break
			}
		}
		if !found {
			*out = append(*out, propertyAssoc{item: item, list: list})
		}
	}
	return nil
}

func parseIPRP(b boxHeader) (properties, error) {
	var out properties
	it := b.children()
	for {
		child, ok, err := it.next()
		if err != nil {
			return out, err
		}
		if !ok {
			return out, nil
		}
		if child.is("ipco") {
			pit := child.children()
			for {
				p, ok, e := pit.next()
				if e != nil {
					return out, e
				}
				if !ok {
					break
				}
				out.boxes = append(out.boxes, p)
			}
		} else if child.is("ipma") {
			if err := parseIPMA(child, &out.assoc); err != nil {
				return out, err
			}
		}
	}
}

func parseMeta(file []byte) (meta, error) {
	b, err := newBoxIter(file).require("meta")
	if err != nil {
		return meta{}, err
	}
	r := b.reader()
	if _, _, err = r.fullBox("meta"); err != nil {
		return meta{}, err
	}
	out := meta{handler: "    "}
	primarySet := false
	it := newBoxIter(r.rest())
	for {
		child, ok, e := it.next()
		if e != nil {
			return out, e
		}
		if !ok {
			break
		}
		switch child.boxType {
		case "hdlr":
			_, _, hr, e := child.fullBox("hdlr")
			if e == nil {
				e = hr.skip(4, "hdlr pre_defined")
			}
			if e == nil {
				out.handler, e = hr.fourcc("hdlr handler type")
			}
			if e != nil {
				return out, e
			}
		case "pitm":
			version, _, pr, e := child.fullBox("pitm")
			if e != nil {
				return out, e
			}
			if version == 0 {
				v, e := pr.u16("pitm item id")
				out.primary, err = uint32(v), e
			} else {
				out.primary, err = pr.u32("pitm item id")
			}
			primarySet = err == nil
		case "iinf":
			out.items, err = parseIINF(child)
		case "iref":
			out.refs, err = parseIREF(child)
		case "iloc":
			out.locations, err = parseILOC(child)
		case "iprp":
			out.props, err = parseIPRP(child)
		case "idat":
			out.idat = child.payload
		}
		if err != nil {
			return out, err
		}
	}
	if out.handler != "pict" {
		return out, unsupported("the meta handler is not `pict`; this is not a still-image HEIF file")
	}
	if !primarySet {
		if len(out.items) != 1 {
			return out, missingBox("pitm")
		}
		out.primary = out.items[0].id
	}
	return out, nil
}

func (m *meta) item(id uint32) *itemInfo {
	for i := range m.items {
		if m.items[i].id == id {
			return &m.items[i]
		}
	}
	return nil
}

func (m *meta) location(id uint32) *itemLocation {
	for i := range m.locations {
		if m.locations[i].id == id {
			return &m.locations[i]
		}
	}
	return nil
}

func sliceAt(data []byte, start, length uint64, what string) ([]byte, error) {
	if start > uint64(len(data)) {
		return nil, truncated(what)
	}
	rest := data[int(start):]
	if length == 0 {
		return rest, nil
	}
	if length > uint64(len(rest)) {
		return nil, truncated(what)
	}
	return rest[:int(length)], nil
}

func addOffsets(a, b uint64) (uint64, bool) {
	if math.MaxUint64-a < b {
		return 0, false
	}
	return a + b, true
}

func (m *meta) itemData(file []byte, id uint32) ([]byte, error) {
	return m.itemDataAt(file, id, 0)
}

func (m *meta) itemDataAt(file []byte, id uint32, depth int) ([]byte, error) {
	if depth > maxItemDepth {
		return nil, cyclicDerivation(id)
	}
	loc := m.location(id)
	if loc == nil {
		return nil, missingItem(id)
	}
	if len(loc.extents) == 0 {
		return nil, malformed("item location lists no extents")
	}
	if len(loc.extents) == 1 {
		return m.extentBytes(file, loc, loc.extents[0], depth)
	}
	out := make([]byte, 0)
	for _, ex := range loc.extents {
		part, err := m.extentBytes(file, loc, ex, depth)
		if err != nil {
			return nil, err
		}
		if len(part) > len(file)-len(out) {
			return nil, malformed("item data is larger than the file that holds it")
		}
		out = append(out, part...)
	}
	return out, nil
}

func (m *meta) extentBytes(file []byte, loc *itemLocation, ex extent, depth int) ([]byte, error) {
	start, ok := addOffsets(loc.baseOffset, ex.offset)
	if !ok {
		return nil, malformed("item offset overflows")
	}
	switch loc.construction {
	case constructionFile:
		return sliceAt(file, start, ex.length, "item extent in file")
	case constructionIDAT:
		if m.idat == nil {
			return nil, missingBox("idat")
		}
		return sliceAt(m.idat, start, ex.length, "item extent in idat")
	case constructionItem:
		if ex.index > math.MaxUint32 {
			return nil, malformed("iloc extent index is not an item id")
		}
		source := uint32(ex.index)
		if source == loc.id {
			return nil, cyclicDerivation(loc.id)
		}
		base, err := m.itemDataAt(file, source, depth+1)
		if err != nil {
			return nil, err
		}
		part, err := sliceAt(base, start, ex.length, "item extent in item")
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), part...), nil
	default:
		return nil, unsupported("unknown iloc construction method")
	}
}
