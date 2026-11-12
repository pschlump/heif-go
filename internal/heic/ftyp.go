package heic

type brand string

const (
	brandHEIC brand = "heic"
	brandHEIX brand = "heix"
	brandHEIM brand = "heim"
	brandHEIS brand = "heis"
	brandHEVC brand = "hevc"
	brandMIF1 brand = "mif1"
	brandMSF1 brand = "msf1"
	brandAVIF brand = "avif"
)

type fileType struct {
	major           brand
	effective       brand
	minorVersion    uint32
	compatibleCount int
}

func knownBrand(code string) (brand, bool) {
	switch code {
	case "heic", "heix", "heim", "heis", "hevc", "mif1", "msf1":
		return brand(code), true
	case "avif", "avis":
		return brandAVIF, true
	default:
		return "", false
	}
}

func parseFileType(data []byte) (fileType, error) {
	b, err := newBoxIter(data).require("ftyp")
	if err != nil {
		return fileType{}, err
	}
	r := b.reader()
	majorCode, err := r.fourcc("ftyp major brand")
	if err != nil {
		return fileType{}, err
	}
	minor, err := r.u32("ftyp minor version")
	if err != nil {
		return fileType{}, err
	}
	major, majorOK := knownBrand(majorCode)
	var fallback brand
	fallbackOK := false
	sawAVIF := false
	count := 0
	for r.remaining() >= 4 {
		code, e := r.fourcc("ftyp compatible brand")
		if e != nil {
			return fileType{}, e
		}
		count++
		if v, ok := knownBrand(code); ok {
			sawAVIF = sawAVIF || v == brandAVIF
			if !fallbackOK {
				fallback, fallbackOK = v, true
			}
		}
	}
	if !majorOK {
		if !fallbackOK {
			return fileType{}, unsupported("the file's brand is not a recognised HEIF brand")
		}
		major = fallback
	}
	effective := major
	if major == brandAVIF || sawAVIF {
		effective = brandAVIF
	}
	if effective == brandAVIF {
		return fileType{}, unsupported("this is an AVIF file; only HEVC-coded HEIC/HEIF is supported")
	}
	return fileType{major: major, effective: effective, minorVersion: minor, compatibleCount: count}, nil
}
