package attachments

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // decode GIF uploads
	"image/jpeg"
	_ "image/png" // decode PNG uploads

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decode WebP uploads
)

const (
	// FullEdge and ThumbEdge are the longest edges in pixels of a stored
	// image and of its preview.
	FullEdge  = 2000
	ThumbEdge = 800
	// maxPixels refuses images whose decoded size would exhaust memory.
	maxPixels    = 50_000_000
	fullQuality  = 85
	thumbQuality = 80
)

// prepareImage decodes an image, turns it upright by its EXIF orientation,
// shrinks it to FullEdge and encodes it as JPEG, which drops all metadata
// (location, camera ...). It also returns the preview.
func prepareImage(data []byte) (full, thumb []byte, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil, nil, fmt.Errorf("%w: %dx%d pixels", ErrImage, cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrImage, err)
	}
	// The long edge doesn't change by turning, so scale first: it makes
	// turning cheaper.
	img := orient(scale(src, FullEdge), jpegOrientation(data))
	if full, err = encode(img, fullQuality); err != nil {
		return nil, nil, err
	}
	if thumb, err = encode(scale(img, ThumbEdge), thumbQuality); err != nil {
		return nil, nil, err
	}
	return full, thumb, nil
}

// Thumbnail returns the preview of a stored image.
func Thumbnail(data []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrImage, err)
	}
	return encode(scale(img, ThumbEdge), thumbQuality)
}

func encode(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// scale shrinks img so that its longest edge is at most edge pixels, on a
// white background (JPEG has no transparency).
func scale(img image.Image, edge int) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > edge || h > edge {
		if w >= h {
			w, h = edge, max(1, h*edge/b.Dx())
		} else {
			w, h = max(1, w*edge/b.Dy()), edge
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// orient turns and mirrors img as the EXIF orientation (1 to 8) says, so
// it is upright.
func orient(img *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return img
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			copy(dst.Pix[dst.PixOffset(dx, dy):][:4], img.Pix[img.PixOffset(x, y):][:4])
		}
	}
	return dst
}

// jpegOrientation reads the EXIF orientation of a JPEG; 1 (upright) if
// there is none.
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		switch {
		case marker == 0xFF: // fill byte
			i++
			continue
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7): // no length
			i += 2
			continue
		case marker == 0xDA || marker == 0xD9: // image data starts
			return 1
		}
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return 1
		}
		if seg := b[i+4 : i+2+n]; marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += 2 + n
	}
	return 1
}

// tiffOrientation reads tag 0x0112 from the first IFD of EXIF data.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	for k := range int(bo.Uint16(t[off:])) {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}
