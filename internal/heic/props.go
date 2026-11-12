package heic

import "strings"

type ispe struct{ width, height uint32 }
type pixi struct{ bitsPerChannel []uint8 }
type pasp struct{ hSpacing, vSpacing uint32 }

type rotation uint8

const (
	rotateNone rotation = iota
	rotateCCW90
	rotateCCW180
	rotateCCW270
)

func (r rotation) swapsAxes() bool { return r == rotateCCW90 || r == rotateCCW270 }

type mirror uint8

const (
	mirrorLeftRight mirror = iota
	mirrorTopBottom
)

type rationalU struct{ n, d uint32 }
type rationalI struct {
	n int32
	d uint32
}
type clap struct {
	width, height rationalU
	horizOff      rationalI
	vertOff       rationalI
}

type auxKind uint8

const (
	auxAlpha auxKind = iota
	auxDepth
	auxOther
)

type auxC struct {
	urn  string
	kind auxKind
}

type transformKind uint8

const (
	transformCrop transformKind = iota
	transformRotate
	transformMirror
)

type transform struct {
	kind transformKind
	crop clap
	rot  rotation
	mir  mirror
}

type matrixCoefficients uint16

const (
	matrixIdentity matrixCoefficients = 0
	matrixBT709    matrixCoefficients = 1
	matrixUnknown  matrixCoefficients = 2
	matrixBT601    matrixCoefficients = 5
	matrix240M     matrixCoefficients = 7
	matrixBT2020   matrixCoefficients = 9
)

type sampleRange uint8

const (
	rangeLimited sampleRange = iota
	rangeFull
)

type nclx struct {
	primaries   uint16
	transfer    uint16
	matrix      matrixCoefficients
	matrixCode  uint16
	sampleRange sampleRange
}

func defaultNCLX() nclx {
	return nclx{primaries: 1, transfer: 13, matrix: matrixBT709, matrixCode: 1, sampleRange: rangeLimited}
}

type itemProps struct {
	hvcc          *hevcConfig
	ispe          *ispe
	pixi          *pixi
	nclx          *nclx
	icc           []byte
	iccRestricted bool
	transforms    []transform
	auxc          *auxC
	pasp          *pasp
}

func (p itemProps) bitDepth() (uint8, bool) {
	if p.pixi != nil && len(p.pixi.bitsPerChannel) > 0 {
		return p.pixi.bitsPerChannel[0], true
	}
	if p.hvcc != nil {
		return p.hvcc.bitDepthLuma, true
	}
	return 0, false
}

func gatherProps(props *properties, item uint32) (itemProps, error) {
	var out itemProps
	for _, a := range props.associations(item) {
		b, ok := props.resolve(a)
		if !ok {
			continue
		}
		var err error
		switch b.boxType {
		case "hvcC":
			out.hvcc, err = parseHVCC(b)
		case "ispe":
			var v ispe
			_, _, r, e := b.fullBox("ispe")
			if e != nil {
				err = e
				break
			}
			v.width, err = r.u32("ispe width")
			if err == nil {
				v.height, err = r.u32("ispe height")
			}
			if err == nil && (v.width == 0 || v.height == 0) {
				err = malformed("ispe declares a zero dimension")
			}
			out.ispe = &v
		case "pixi":
			var v pixi
			_, _, r, e := b.fullBox("pixi")
			if e != nil {
				err = e
				break
			}
			n, e := r.u8("pixi channel count")
			if e != nil {
				err = e
				break
			}
			if int(n) > r.remaining() {
				err = malformed("pixi declares more channels than it holds")
				break
			}
			v.bitsPerChannel = make([]uint8, n)
			for i := range v.bitsPerChannel {
				v.bitsPerChannel[i], err = r.u8("pixi channel depth")
				if err != nil {
					break
				}
			}
			out.pixi = &v
		case "colr":
			r := b.reader()
			kind, e := r.fourcc("colr colour type")
			if e != nil {
				err = e
				break
			}
			switch kind {
			case "nclx":
				var v nclx
				v.primaries, err = r.u16("colr primaries")
				if err == nil {
					v.transfer, err = r.u16("colr transfer")
				}
				if err == nil {
					v.matrixCode, err = r.u16("colr matrix")
				}
				v.matrix = matrixCoefficients(v.matrixCode)
				if v.matrixCode == 6 {
					v.matrix = matrixBT601
				}
				if err == nil {
					flags, e := r.u8("colr range flag")
					err = e
					if flags&0x80 != 0 {
						v.sampleRange = rangeFull
					}
				}
				out.nclx = &v
			case "rICC", "prof":
				out.icc, out.iccRestricted = r.rest(), kind == "rICC"
			case "nclc":
				err = unsupported("the `nclc` colour type belongs to QuickTime, not HEIF")
			default:
				err = malformed("unknown colr colour type")
			}
		case "irot":
			r := b.reader()
			v, e := r.u8("irot angle")
			err = e
			out.transforms = append(out.transforms, transform{kind: transformRotate, rot: rotation(v & 3)})
		case "imir":
			r := b.reader()
			v, e := r.u8("imir axis")
			err = e
			out.transforms = append(out.transforms, transform{kind: transformMirror, mir: mirror(v & 1)})
		case "clap":
			r := b.reader()
			var c clap
			c.width.n, err = r.u32("clap width numerator")
			if err == nil {
				c.width.d, err = r.u32("clap width denominator")
			}
			if err == nil {
				c.height.n, err = r.u32("clap height numerator")
			}
			if err == nil {
				c.height.d, err = r.u32("clap height denominator")
			}
			if err == nil {
				v, e := r.u32("clap horizontal offset numerator")
				c.horizOff.n, err = int32(v), e
			}
			if err == nil {
				c.horizOff.d, err = r.u32("clap horizontal offset denominator")
			}
			if err == nil {
				v, e := r.u32("clap vertical offset numerator")
				c.vertOff.n, err = int32(v), e
			}
			if err == nil {
				c.vertOff.d, err = r.u32("clap vertical offset denominator")
			}
			if err == nil && (c.width.d == 0 || c.height.d == 0 || c.horizOff.d == 0 || c.vertOff.d == 0) {
				err = malformed("clap has a zero denominator")
			}
			out.transforms = append(out.transforms, transform{kind: transformCrop, crop: c})
		case "auxC":
			_, _, r, e := b.fullBox("auxC")
			if e != nil {
				err = e
				break
			}
			urn, e := r.cstr("auxC aux type")
			err = e
			v := auxC{urn: urn, kind: auxOther}
			if strings.HasSuffix(urn, "auxiliary:alpha") || urn == "urn:mpeg:hevc:2015:auxid:1" {
				v.kind = auxAlpha
			} else if strings.HasSuffix(urn, "auxiliary:depth") || urn == "urn:mpeg:hevc:2015:auxid:2" {
				v.kind = auxDepth
			}
			out.auxc = &v
		case "pasp":
			r := b.reader()
			var v pasp
			v.hSpacing, err = r.u32("pasp hSpacing")
			if err == nil {
				v.vSpacing, err = r.u32("pasp vSpacing")
			}
			out.pasp = &v
		}
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
