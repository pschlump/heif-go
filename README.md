# heif-go

**A pure Go HEIC/HEIF decoder with a `goheif`-compatible API.**

`heif-go` converts HEIC and HEIF still images into Go `image.Image` values, ready for JPEG encoding or other image processing. It includes the container parser and HEVC still-picture decoder—there is no C library, cgo bridge, Rust component, external Go dependency, or C cross-toolchain to install.

The API is compatible with [`github.com/adrium/goheif`](https://github.com/adrium/goheif), while the decoder is a Go port of the permissively licensed [`heic-rs`](https://github.com/tbraun96/heic-rs) implementation.

The project was created for **Agentic Quartz Corp's real-estate agent product** to provide permissively licensed HEIC/HEIF support without depending on GNU-licensed decoder libraries. It is intended to be suitable for proprietary as well as open-source products.

## Highlights

- Pure, portable Go; Linux/amd64 builds work with `CGO_ENABLED=0`.
- Familiar `Decode`, `DecodeConfig`, and `ExtractExif` functions.
- Registers the `heic` format with Go's `image` package.
- Single-item images and grids of independently coded tiles.
- Parallel tile decoding and parallel colour conversion.
- Rotation, mirroring, and clean-aperture cropping.
- Non-premultiplied RGBA output through `image.NRGBA`.
- EXIF extraction and auxiliary alpha-plane decoding.
- Bounded allocations with a default 256-megapixel limit.
- MIT licensed, including the upstream MIT copyright notice.

## Installation

```sh
go get github.com/pschlump/heif-go
```

The module requires Go 1.23 or newer.

## Example: HEIC to JPEG

```go
package main

import (
	"image/jpeg"
	"log"
	"os"

	goheif "github.com/pschlump/heif-go"
)

func main() {
	input, err := os.Open("photo.heic")
	if err != nil {
		log.Fatal(err)
	}
	defer input.Close()

	img, err := goheif.Decode(input)
	if err != nil {
		log.Fatal(err)
	}

	output, err := os.Create("photo.jpg")
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	if err := jpeg.Encode(output, img, &jpeg.Options{Quality: 90}); err != nil {
		log.Fatal(err)
	}
}
```

`Decode` returns an `image.Image` backed by `image.NRGBA`, so it works with `image/jpeg`, `image/png`, and the rest of Go's image ecosystem. JPEG encoding is performed by the standard library, not by this package.

## Example: `image.Decode` and `DecodeConfig`

Importing the package registers HEIC with the standard `image` package:

```go
package main

import (
	"fmt"
	"image"
	"log"
	"os"

	_ "github.com/pschlump/heif-go"
)

func main() {
	input, err := os.Open("photo.heic")
	if err != nil {
		log.Fatal(err)
	}
	defer input.Close()

	config, err := image.DecodeConfig(input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%dx%d\n", config.Width, config.Height)

	if _, err := input.Seek(0, 0); err != nil {
		log.Fatal(err)
	}
	img, format, err := image.Decode(input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("format=%s bounds=%v\n", format, img.Bounds())
}
```

The registered format name is `"heic"`.

## Example: EXIF

```go
exif, err := goheif.ExtractExif(input)
if err == goheif.ErrNoEXIF {
	// The image has no EXIF item.
} else if err != nil {
	log.Fatal(err)
}
```

The returned bytes are the TIFF payload from the primary image's EXIF item, including its TIFF header. `ExtractExif` accepts an `io.ReaderAt`, such as an open `os.File` or `bytes.Reader`.

## API

| Function or variable | Purpose |
| --- | --- |
| `Decode(io.Reader) (image.Image, error)` | Decode the primary image to non-premultiplied RGBA pixels. |
| `DecodeConfig(io.Reader) (image.Config, error)` | Read displayed dimensions without decoding pixels. |
| `ExtractExif(io.ReaderAt) ([]byte, error)` | Extract the primary image's EXIF TIFF payload. |
| `ErrNoEXIF` | Returned when no EXIF item exists. |
| `SafeEncoding` | Retained for `goheif` source compatibility; it has no effect in the pure Go implementation. |

## Supported input

- HEIC/HEIF brands based on HEVC still images.
- HEVC Main, Main 10, and Main Still Picture decoding paths.
- Monochrome, 4:2:0, 4:2:2, and 4:4:4 chroma layouts.
- Single-picture items and `grid` derivations, including cropped edge tiles.
- `irot`, `imir`, and `clap` display transforms in association order.
- BT.601, BT.709, BT.2020, and SMPTE 240M matrices; full and limited sample ranges.
- EXIF and auxiliary alpha items.

The supplied end-to-end reference corpus contains 8-bit 4:2:0 images. The decoder also contains the ported high-bit-depth and alternate-chroma paths, but those are not exercised by that corpus.

Not supported: AVIF/AV1, animation or multi-picture sequences, P/B inter prediction, dependent slices, `iovl` overlays, separate colour planes, and unsupported HEVC range/screen-content extensions. Unsupported coding tools return errors rather than silently producing incorrect pixels.

## Correctness

The test suite includes container and codec unit tests, real-file decoding, parallel-versus-serial comparisons, transforms, EXIF, truncation, known fuzz regressions, and JPEG conversion. Reference PNGs were produced by Apple's platform HEIC decoder.

Constant-chroma fixtures must match exactly. The other fixtures require at least 45 dB RGB PSNR and 55 dB BT.601 luma PSNR.

| Fixture | RGB PSNR | Luma PSNR |
| --- | ---: | ---: |
| flat-white-16 | exact | exact |
| checker-64 | exact | exact |
| checker-1024, 2×2 grid | exact | exact |
| flat-64 | 49.80 dB | 58.95 dB |
| rgb-strips-96 | 49.11 dB | 57.62 dB |
| gradient-512 | 47.76 dB | 60.01 dB |
| photo-2048, 4×3 grid | 48.22 dB | 59.90 dB |
| rotated-90 | 48.24 dB | 59.96 dB |
| with-exif | 48.19 dB | 59.89 dB |

Validation completed on macOS/arm64 and Linux/amd64:

```sh
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -fuzz FuzzDecode -fuzztime=10s .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c .
```

Approximately 2.8 million fuzz executions completed without a crash. The statically linked Linux test binaries were also copied to and executed successfully on the production Linode host.

## Performance

### Whole-file public API benchmarks

These measurements run `goheif.Decode`, including container parsing, HEVC decoding, grid handling, and colour conversion. JPEG encoding and file I/O are excluded.

- **macOS:** Apple M4 Max, 14 CPUs available to Go.
- **Linode:** AMD EPYC 7713, 2 CPUs available to Go, Linux/amd64, statically linked `CGO_ENABLED=0` test binary.
- Go values are `testing.B` averages. Rust values are the best of 9 runs, or best of 50 for the final photo measurement.
- Output is NRGBA for this Go API, RGB8 for `heic-rs`, and YCbCr for the Go/C implementation. These columns therefore compare complete decoder workloads, not identical output layouts.

| Fixture | heif-go, M4 Max | heic-rs, M4 Max | Go/C, M4 Max | heif-go, Linode |
| --- | ---: | ---: | ---: | ---: |
| flat-64 | 0.061 ms | 0.02 ms | — | 0.262 ms |
| gradient-512 | 4.52 ms | 1.11 ms | — | 16.45 ms |
| checker-1024 | 1.72 ms | 0.91 ms | — | 17.41 ms |
| photo-2048 | 6.85 ms | 2.12 ms | — | 94.47 ms |
| camel | 76.34 ms | 40.83 ms | 42.86 ms | 252.76 ms |

The implementation is correct and portable, but these measurements also show that it is currently slower than the Rust and Go/C reference decoders. Performance work is ongoing.

### RGB8 serial and pooled comparison

For a closer output-format comparison, the internal benchmark produces RGB8 with both implementations:

| Fixture | heif-go serial | heic-rs serial | heif-go pooled | heic-rs pooled |
| --- | ---: | ---: | ---: | ---: |
| gradient-512 | 4.09 ms | 0.97 ms | 4.13 ms | 1.11 ms |
| checker-1024 | 4.59 ms | 2.48 ms | 1.69 ms | 0.91 ms |
| photo-2048 | 38.72 ms | 14.86 ms | 6.57 ms | 2.12 ms |

A single coded picture has no tiles to decode in parallel. Larger grids scale much better because their tiles are independent.

### Allocation profiles

`B/op` is memory allocated by the Go decoder per operation. It does not include native allocations made by the Go/C implementation.

| Fixture | heif-go, M4 Max | heif-go, Linode | Linode allocations/op |
| --- | ---: | ---: | ---: |
| flat-64 | 67 KB | 67 KB | 101 |
| gradient-512 | 2.02 MB | 2.02 MB | 110 |
| checker-1024 | 8.23 MB | 8.03 MB | 304 |
| photo-2048 | 24.61 MB | 24.21 MB | 719 |
| camel | 14.16 MB | 13.91 MB | 138 |

For the camel workload, Go/C reports 403 KB and 280 allocations on the Go side, plus uncounted native allocations. Process-wide peak RSS was 110.6 MB for heif-go and 99.9 MB for Go/C. On the photo workload, peak RSS was 131.0 MB for heif-go and 38.4 MB for heic-rs. Peak RSS includes runtime and test-process overhead and is not a per-decode allocation measurement.

Run the benchmarks with:

```sh
go test -run '^$' -bench BenchmarkDecode -benchtime=1s -benchmem .
go test -run '^$' -bench 'BenchmarkDecodeRGB8|BenchmarkColorConvertRGB8' -benchtime=1s -benchmem ./internal/heic
```

## Why pure Go?

The original `goheif` package wraps `libde265`. That works, but it requires a C toolchain for native and cross-compilation. A pure Go decoder removes that build dependency and the C memory-safety boundary, and produces a statically linked binary with the ordinary Go toolchain.

For example, an application that previously needed Zig for HEIC support can now use:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/server
```

The decoder performs bounds checks throughout its parsers and limits declared image dimensions before allocating output pixels.

## License

MIT. See [LICENSE](LICENSE).

The HEIC container and HEVC decoder were ported from `heic-rs`, whose MIT copyright notice is retained. This license does not grant patent rights in the HEVC standard itself; codec patent considerations are separate from the software license.
