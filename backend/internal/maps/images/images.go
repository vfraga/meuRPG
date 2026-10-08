// Package images checks and cleans the images a master uploads to the
// campaign's gallery (MR-019).
//
// Process accepts only JPEG, PNG and WebP, recognized by their first bytes
// (never by the file name or the browser's word). It reads the image's size
// from its header before decoding anything, and refuses one that would
// take too much memory to decode (a "decompression bomb": a small file that
// claims billions of pixels). Then it decodes the image and encodes it
// again from the pixels alone. That is what removes every piece of metadata
// (EXIF with the GPS position, XMP, ICC profiles, PNG text chunks) and
// anything hidden after the image data: the new file has pixels and
// nothing else (docs/privacy.md).
//
// The package has no database, no network and no state, so its tests run
// in a second.
package images

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Limits (a proposal, question 30 of the progress doc; see
// docs/product/stories.md, MR-019).
const (
	// MaxBytes is the largest file accepted, and the largest file stored
	// after re-encoding: 10 MiB.
	MaxBytes = 10 << 20
	// MaxSide is the most pixels on either side.
	MaxSide = 8192
	// MaxPixels is the most pixels in all (40 megapixels, such as
	// 8000 x 5000).
	MaxPixels = 40_000_000
	// ThumbnailSide is the thumbnail's longer side, in pixels.
	ThumbnailSide = 480
)

// jpegQuality is the quality of every JPEG this package writes: high
// enough that a map's details survive, and much smaller than 100.
const jpegQuality = 88

// maxDecodeBytes is the most memory an image may need while it is decoded,
// turned upright and shrunk into its thumbnail, as estimated by decodeCost.
// With one image processed at a time (package maps), this keeps an upload
// well inside the server's 512 MiB (docs/operations.md).
const maxDecodeBytes = 256 << 20

// The media types Process writes.
const (
	JPEG = "image/jpeg"
	PNG  = "image/png"
)

// Why Process refuses an image. Package maps turns each one into the
// reason the app shows (UNSUPPORTED_TYPE, TOO_LARGE, DIMENSIONS, CORRUPT).
var (
	// ErrUnsupportedType: not a JPEG, a PNG or a WebP (SVG and GIF included).
	ErrUnsupportedType = errors.New("images: only JPEG, PNG and WebP images are accepted")
	// ErrTooLarge: the file, or the re-encoded image, is larger than MaxBytes.
	ErrTooLarge = errors.New("images: the image is larger than 10 MiB")
	// ErrDimensions: too many pixels (MaxSide, MaxPixels), or too much memory
	// to decode (maxDecodeBytes).
	ErrDimensions = errors.New("images: the image has too many pixels")
	// ErrCorrupt: it looks like an image of an accepted type, but cannot be
	// read.
	ErrCorrupt = errors.New("images: the image cannot be read")
)

// Result is a cleaned image, ready to store.
type Result struct {
	// ContentType is JPEG or PNG, for both Data and Thumbnail.
	ContentType string
	// Width and Height are the image's size in pixels, upright.
	Width, Height int
	// Data is the re-encoded image.
	Data []byte
	// Thumbnail is the image shrunk to ThumbnailSide pixels on its longer
	// side. A small image is its own thumbnail.
	Thumbnail []byte
	// Reference is the JPEG of at most ReferenceSide pixels that the image
	// travels as to the image model (MR-039), made from the pixels already
	// decoded. Nil for an image that fits in ReferenceSide on both sides: it
	// needs none.
	Reference []byte
}

// format is a type of image that Process accepts.
type format int

const (
	formatJPEG format = iota + 1
	formatPNG
	formatWebP
)

// Process checks data, an uploaded file, and returns the image re-encoded
// with no metadata, with its thumbnail. PNG stays PNG, so a map keeps its
// transparent parts. JPEG becomes JPEG. WebP becomes JPEG, or PNG when it
// has transparent pixels. A JPEG is turned upright as its EXIF orientation
// says, since the orientation goes away with the rest of the EXIF.
//
// The error is one of this package's Err values.
func Process(data []byte) (*Result, error) {
	if len(data) > MaxBytes {
		return nil, ErrTooLarge
	}
	f := sniff(data)
	if f == 0 {
		return nil, ErrUnsupportedType
	}

	// The header first: its size says whether decoding is safe at all.
	cfg, err := decodeConfig(f, data)
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return nil, ErrCorrupt
	}
	if cfg.Width > MaxSide || cfg.Height > MaxSide || cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrDimensions
	}
	var jh jpegHeader
	if f == formatJPEG {
		jh = readJPEGHeader(data)
	}
	if decodeCost(f, cfg, jh, readPNGInfo(data)) > maxDecodeBytes {
		return nil, ErrDimensions
	}

	img, err := decode(f, data)
	if err != nil {
		return nil, ErrCorrupt
	}
	if f == formatJPEG {
		img = upright(img, jh.orientation)
	}

	contentType := PNG
	if f == formatJPEG || (f == formatWebP && isOpaque(img)) {
		contentType = JPEG
	}
	if contentType == PNG {
		img = to8bit(img)
	}
	return finish(contentType, img, thumbnail, makeReference)
}

// Encode is Process for an image the server drew itself (a generated dungeon's
// map): there is nothing to decode or sniff, so it checks the size the way Process
// does (MaxSide, MaxPixels), writes the pixels as a PNG, which carries no
// metadata, and makes the thumbnail. It makes no reference image: the dungeon
// is stored without one and gets it the first time it is used as a reference
// (Service.shrunkImage). The error is ErrDimensions or ErrTooLarge.
func Encode(img image.Image) (*Result, error) {
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 || b.Dx() > MaxSide || b.Dy() > MaxSide || b.Dx()*b.Dy() > MaxPixels {
		return nil, ErrDimensions
	}
	return finish(PNG, to8bit(img), thumbnailOfDrawing, false)
}

// MaxFitPixels is the most pixels CropFit makes: 16 megapixels, 64 MB as it is
// drawn, and more than a picture the model returns (at most 4K) can fill.
const MaxFitPixels = 16_000_000

// The most a model answer CropFit decodes may be on a side (the model returns at most
// 4K), and the most memory the whole call may need, the decoded answer and the
// working copy of the output together: the slot's budget (docs/operations.md). A
// 4096 x 4096 answer into 16 megapixels measures 151 MiB; the limit leaves it room.
const (
	maxFitAnswerSide = 4096
	maxFitBytes      = 178 << 20
)

// CropFit is Process for a picture the model made of a map's drawing padded to
// its ratio (MR-039): it decodes data (checked as Process does), takes the
// rectangle crop(w, h) gives for the picture's size, scales that to exactly outW x
// outH pixels, and stores it as a JPEG with no metadata and its thumbnail. The
// result has the map's own size, so it can stand in for the map's image and keep
// its grid and layers. The error is one of this package's Err values: ErrDimensions
// also for an output over MaxFitPixels, an answer over 4096 pixels on a side, a decode
// that with the output would pass the slot's 178 MiB, and a crop that is empty.
func CropFit(data []byte, crop func(w, h int) image.Rectangle, outW, outH int) (*Result, error) {
	if outW < 1 || outH < 1 || outW > MaxSide || outH > MaxSide || outW*outH > MaxFitPixels {
		return nil, ErrDimensions
	}
	if len(data) > MaxBytes {
		return nil, ErrTooLarge
	}
	f := sniff(data)
	if f == 0 {
		return nil, ErrUnsupportedType
	}
	cfg, err := decodeConfig(f, data)
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return nil, ErrCorrupt
	}
	if cfg.Width > maxFitAnswerSide || cfg.Height > maxFitAnswerSide {
		return nil, ErrDimensions
	}
	var jh jpegHeader
	if f == formatJPEG {
		jh = readJPEGHeader(data)
	}
	// The decoded answer and the output's working copy (4 bytes a pixel) live together.
	if cost := decodeCost(f, cfg, jh, readPNGInfo(data)) + 4*int64(outW)*int64(outH); cost > maxFitBytes {
		return nil, ErrDimensions
	}
	img, err := decode(f, data)
	if err != nil {
		return nil, ErrCorrupt
	}
	if f == formatJPEG {
		img = upright(img, jh.orientation)
	}
	b := img.Bounds()
	rect := crop(b.Dx(), b.Dy()).Add(b.Min).Intersect(b)
	if rect.Empty() {
		return nil, ErrDimensions
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, ErrCorrupt
	}
	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))
	scaleBands(dst, sub.SubImage(rect), xdraw.Src)
	return finish(JPEG, dst, thumbnail, makeReference)
}

// finish encodes img as contentType, refuses a file over MaxBytes and adds the
// thumbnail, and the reference when withReference says so (the server's own
// drawings go without: nothing stores it, and a reference is made when one is
// first wanted).
func finish(contentType string, img image.Image, shrink func(image.Image) image.Image, withReference bool) (*Result, error) {
	out, err := encode(contentType, img)
	if err != nil {
		return nil, err
	}
	if len(out) > MaxBytes {
		return nil, ErrTooLarge
	}

	b := img.Bounds()
	res := &Result{ContentType: contentType, Width: b.Dx(), Height: b.Dy(), Data: out, Thumbnail: out}
	if b.Dx() > ThumbnailSide || b.Dy() > ThumbnailSide {
		if res.Thumbnail, err = encode(contentType, shrink(img)); err != nil {
			return nil, err
		}
	}
	if withReference && (b.Dx() > ReferenceSide || b.Dy() > ReferenceSide) {
		if res.Reference, err = reference(img, ReferenceSide, ReferenceQuality); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// sniff recognizes the accepted formats by their signatures, or returns 0.
func sniff(data []byte) format {
	switch {
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return formatJPEG
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return formatPNG
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return formatWebP
	default:
		return 0
	}
}

// decodeConfig reads only the image's header: its size and color model.
func decodeConfig(f format, data []byte) (image.Config, error) {
	r := bytes.NewReader(data)
	switch f {
	case formatJPEG:
		return jpeg.DecodeConfig(r)
	case formatPNG:
		return png.DecodeConfig(r)
	default:
		return webp.DecodeConfig(r)
	}
}

// decode reads the whole image. Each format's decoder is called by name,
// instead of image.Decode, so only these three formats are ever decoded,
// whatever other decoders the program happens to register.
func decode(f format, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch f {
	case formatJPEG:
		return jpeg.Decode(r)
	case formatPNG:
		return png.Decode(r)
	default:
		return webp.Decode(r)
	}
}

// decodeCost estimates, in bytes, the memory Process needs for an image:
// the decoded pixels (with the decoder's own buffers), the upright copy,
// and the thumbnail's scratch space. It errs on the high side.
func decodeCost(f format, cfg image.Config, jh jpegHeader, pi pngInfo) int64 {
	var perPixel float64
	switch f {
	case formatPNG:
		perPixel = pngBytesPerPixel(cfg.ColorModel, pi)
		if pi.interlaced {
			// image/png decodes each of the 7 passes into its own image
			// before merging them: up to the image's size once more.
			perPixel *= 2
		}
	case formatWebP:
		// A lossless WebP is decoded into one 4-byte-per-pixel buffer and
		// then copied into another.
		perPixel = 8
	case formatJPEG:
		// The decoded samples: 1.5 bytes per pixel for the usual 4:2:0
		// color JPEG, 3 without chroma subsampling, 4 for CMYK.
		perPixel = jh.samples
		if cfg.ColorModel == color.RGBAModel || cfg.ColorModel == color.CMYKModel {
			perPixel += 4 // the decoder converts them into a second image
		}
		if jh.progressive {
			// A progressive JPEG keeps every DCT coefficient (4 bytes per
			// sample) until its last scan.
			perPixel += 4 * jh.samples
		}
		if jh.orientation > 1 {
			perPixel += 8 // upright's two RGBA copies
		}
	}
	pixels := float64(cfg.Width) * float64(cfg.Height)
	return int64(pixels*perPixel) + thumbnailCost(cfg.Width, cfg.Height) + MaxBytes // the encoded file, at most MaxBytes, lives with the pixels
}

// pngBytesPerPixel is the size of a pixel once a PNG with this color model is
// decoded, and converted by to8bit when it has 16 bits. png.DecodeConfig names
// gray for a gray PNG with a tRNS chunk, but png.Decode returns NRGBA (NRGBA64
// with 16 bits) for it, so the chunk counts.
func pngBytesPerPixel(m color.Model, pi pngInfo) float64 {
	if _, ok := m.(color.Palette); ok {
		return 1
	}
	switch m {
	case color.GrayModel:
		if pi.transparency {
			if pi.bitDepth == bits16 {
				return bytesNRGBA64Plus8bit
			}
			return bytesNRGBA
		}
		return 1
	case color.Gray16Model:
		if pi.transparency {
			return bytesNRGBA64Plus8bit
		}
		return bytesGray16Plus8bit
	case color.RGBAModel, color.NRGBAModel:
		return bytesNRGBA
	default: // 16-bit color (8 bytes, and 4 more for the 8-bit copy Process stores), or something unexpected: the worst case
		return bytesNRGBA64Plus8bit
	}
}

// The bytes a pixel takes once decoded, by the type png.Decode returns.
const (
	bits16               = 16
	bytesNRGBA           = 4
	bytesGray16Plus8bit  = 6  // Gray16 (2 bytes) and the 8-bit copy (4)
	bytesNRGBA64Plus8bit = 12 // NRGBA64 (8 bytes) and the 8-bit copy (4)
)

// pngInfo is what the cost of decoding a PNG needs from the file that the
// decoder's config does not say.
type pngInfo struct {
	interlaced   bool
	transparency bool // a tRNS chunk before the pixels
	bitDepth     int
}

// readPNGInfo reads the interlace method and bit depth in a PNG's header (IHDR is
// always the first chunk: 8 bytes of signature, 8 of chunk length and type, then
// width, height, bit depth at byte 24, color type, compression and filter, and
// the interlace method at byte 28) and looks for a tRNS chunk among the chunks
// before the first IDAT, where the decoder reads it. Anything else is the zero
// value.
func readPNGInfo(data []byte) pngInfo {
	if sniff(data) != formatPNG || len(data) <= 28 {
		return pngInfo{}
	}
	pi := pngInfo{interlaced: data[28] == 1, bitDepth: int(data[24])}
	for off := 8; off+8 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[off:]))
		switch string(data[off+4 : off+8]) {
		case "tRNS":
			pi.transparency = true
			return pi
		case "IDAT", "IEND":
			return pi
		}
		if n < 0 || n > len(data) {
			return pi
		}
		off += 12 + n // length, type, data, CRC
	}
	return pi
}

// HasTransparencyChunk reports whether a PNG has a tRNS chunk before its pixels. For
// a gray PNG, png.DecodeConfig names gray, but png.Decode returns NRGBA (NRGBA64 with
// 16 bits): whoever sizes a decode from the header alone asks it.
func HasTransparencyChunk(data []byte) bool { return readPNGInfo(data).transparency }

// to8bit returns img with 8 bits a channel when it has 16 (a 16-bit PNG): the
// stored image never keeps them, so what is decoded later (the fog's tiles,
// MR-036) costs 4 bytes a pixel, not 8. An image that is already 8-bit is returned as it is.
func to8bit(img image.Image) image.Image {
	switch img.(type) {
	case *image.RGBA64, *image.NRGBA64, *image.Gray16:
		b := img.Bounds()
		out := image.NewNRGBA(b)
		xdraw.Draw(out, b, img, b.Min, xdraw.Src)
		return out
	}
	return img
}

// isOpaque reports whether img has no transparent pixel at all.
func isOpaque(img image.Image) bool {
	o, ok := img.(interface{ Opaque() bool })
	return ok && o.Opaque()
}

// encode writes img as contentType. Neither encoder writes any metadata.
func encode(contentType string, img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	if contentType == JPEG {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	} else {
		err = png.Encode(&buf, img)
	}
	if err != nil {
		// Writing to memory does not fail; an error here is a bug.
		return nil, errors.Join(errors.New("images: encode"), err)
	}
	return buf.Bytes(), nil
}

// thumbnailSize is the thumbnail's size for a w x h image: ThumbnailSide
// pixels on the longer side, the shorter one in proportion (at least 1).
func thumbnailSize(w, h int) (int, int) {
	if w >= h {
		return ThumbnailSide, max(1, (h*ThumbnailSide+w/2)/w)
	}
	return max(1, (w*ThumbnailSide+h/2)/h), ThumbnailSide
}

// thumbnailCost is the memory the thumbnail needs for a w x h image: the
// thumbnail itself and, for the scaler of x/image/draw (a scratch row of four
// float64s per destination column and source row), the source rows of one band of
// scaleBands.
func thumbnailCost(w, h int) int64 {
	if w <= ThumbnailSide && h <= ThumbnailSide {
		return 0
	}
	tw, th := thumbnailSize(w, h)
	bandSource := int64(scaleBandRows)*int64(h)/int64(th) + 2
	return int64(tw)*bandSource*32 + int64(tw)*int64(th)*4
}

// thumbnail shrinks img with the Catmull-Rom filter, which keeps a map's
// grid lines crisp where a cheaper filter would blur or jag them. It scales in
// bands (scaleBands), so its scratch space is a band's, not the image's.
func thumbnail(img image.Image) image.Image {
	b := img.Bounds()
	tw, th := thumbnailSize(b.Dx(), b.Dy())
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	scaleBands(dst, img, xdraw.Src)
	return dst
}

// scaleBandRows is how many destination rows scaleBands scales at a time.
const scaleBandRows = 32

// scaleBands scales img into all of dst with the Catmull-Rom filter, a band of
// 32 destination rows at a time. x/image/draw's scaler keeps a scratch buffer of
// the destination's width by the source rows it is given, four float64s each:
// for a whole 8000 x 5000 image into 480 columns that is 77 MB, and into 1024
// columns 163 MB. A band gives it only the source rows of its destination rows
// (a few MB), so a big image costs its own pixels and nothing more. The seams
// between bands only clamp the filter at the band's edge: a slightly narrower
// average there, invisible in a thumbnail or a reference.
func scaleBands(dst *image.RGBA, img image.Image, op xdraw.Op) {
	b := img.Bounds()
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	for y0 := 0; y0 < h; y0 += scaleBandRows {
		y1 := min(y0+scaleBandRows, h)
		// The source rows of this band of the destination (the last band ends on
		// the last source row).
		sy0 := b.Min.Y + y0*b.Dy()/h
		sy1 := b.Min.Y + y1*b.Dy()/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		xdraw.CatmullRom.Scale(dst, image.Rect(0, y0, w, y1), img, image.Rect(b.Min.X, sy0, b.Max.X, sy1), op, nil)
	}
}

// makeReference switches the reference step of Process off, for the memory
// measure to compare an upload with and without it (measure_test.go).
var makeReference = true

// The reference image (MR-039): the small JPEG a stored image travels as when
// the image model gets it as a reference.
const (
	// ReferenceSide is its longer side, in pixels.
	ReferenceSide = 1024
	// ReferenceQuality is its JPEG quality.
	ReferenceQuality = 85
)

// Shrink makes the reference of a stored image: a JPEG of at most side pixels
// on the longer side (quality 1 to 100), flattened onto white when it has
// transparent pixels. resized says that the image was bigger than side, so the
// result is worth keeping for the next time (package maps stores it). It is
// never what is stored as the image. data is checked like Process checks an
// upload (type, size, memory), so the caller runs it one at a time, as it runs
// Process, and its memory peak is the decoded image and the file, as an upload's.
//
// An image that already fits and is a JPEG comes back as it is: a stored image
// has no metadata. The error is one of this package's Err values.
func Shrink(data []byte, side, quality int) (out []byte, resized bool, err error) {
	f := sniff(data)
	if f == 0 {
		return nil, false, ErrUnsupportedType
	}
	cfg, err := decodeConfig(f, data)
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return nil, false, ErrCorrupt
	}
	if cfg.Width > MaxSide || cfg.Height > MaxSide || cfg.Width*cfg.Height > MaxPixels {
		return nil, false, ErrDimensions
	}
	if f == formatJPEG && cfg.Width <= side && cfg.Height <= side {
		return data, false, nil
	}
	var jh jpegHeader
	if f == formatJPEG {
		jh = readJPEGHeader(data)
	}
	if decodeCost(f, cfg, jh, readPNGInfo(data)) > maxDecodeBytes {
		return nil, false, ErrDimensions
	}
	img, err := decode(f, data)
	if err != nil {
		return nil, false, ErrCorrupt
	}
	out, err = reference(img, side, quality)
	b := img.Bounds()
	return out, b.Dx() > side || b.Dy() > side, err
}

// reference shrinks a decoded image into a JPEG of at most side pixels on the
// longer side. It scales straight from img into the small destination, in bands
// of destination rows: the scaler's scratch space is one band's (a few MB), not
// the whole image's, and no full-size copy of img is made. Process uses it too,
// with the image it has already decoded, so an upload needs no second decode.
func reference(img image.Image, side, quality int) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > side || h > side {
		if w >= h {
			w, h = side, max(1, (h*side+w/2)/w)
		} else {
			w, h = max(1, (w*side+h/2)/h), side
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
	scaleBands(dst, img, xdraw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, errors.Join(errors.New("images: encode"), err)
	}
	return buf.Bytes(), nil
}

// thumbnailOfDrawing is thumbnail for what Encode gets, the server's own drawings: a
// palette image takes the faster box filter below. An uploaded palette PNG keeps
// using thumbnail (Process), whose Catmull-Rom filter keeps a map's grid lines crisp.
func thumbnailOfDrawing(img image.Image) image.Image {
	if p, ok := img.(*image.Paletted); ok {
		return thumbnailPaletted(p)
	}
	return thumbnail(img)
}

// thumbnailPaletted is thumbnail for an image of a palette, such as a generated
// dungeon's map (Encode): it averages the source pixels each thumbnail pixel
// covers (a box filter) in one pass, because the scaler above reads a palette image
// pixel by pixel through the generic interface, which takes seconds for 30
// megapixels. A palette image is a flat drawing, so the box filter is as good.
func thumbnailPaletted(src *image.Paletted) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := thumbnailSize(w, h)
	pal := make([][4]uint32, len(src.Palette))
	for i, c := range src.Palette {
		r, g, bl, a := c.RGBA() // premultiplied, 16 bits
		pal[i] = [4]uint32{r, g, bl, a}
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for dy := range th {
		y0, y1 := dy*h/th, max((dy+1)*h/th, dy*h/th+1)
		for dx := range tw {
			x0, x1 := dx*w/tw, max((dx+1)*w/tw, dx*w/tw+1)
			var sum [4]uint64
			for y := y0; y < y1; y++ {
				row := src.Pix[(b.Min.Y+y-src.Rect.Min.Y)*src.Stride+(b.Min.X+x0-src.Rect.Min.X):][:x1-x0]
				for _, i := range row {
					c := pal[i]
					sum[0], sum[1], sum[2], sum[3] = sum[0]+uint64(c[0]), sum[1]+uint64(c[1]), sum[2]+uint64(c[2]), sum[3]+uint64(c[3])
				}
			}
			n := uint64((y1 - y0) * (x1 - x0)) //nolint:gosec // G115: a positive count of pixels
			o := dst.PixOffset(dx, dy)
			for k := range 4 {
				dst.Pix[o+k] = uint8(sum[k] / n >> 8) //nolint:gosec // G115: the mean of 16-bit values, shifted to 8 bits
			}
		}
	}
	return dst
}
