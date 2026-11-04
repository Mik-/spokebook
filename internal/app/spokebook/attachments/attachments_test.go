package attachments_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/attachments"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite"
)

// halves returns a w×h image, red on the left half and blue on the right.
func halves(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{R: 255, A: 255}
			if x >= w/2 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// withOrientation inserts an EXIF segment with the orientation after the
// start of a JPEG.
func withOrientation(t *testing.T, img image.Image, o uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")  // big endian, IFD at 8
	tiff = binary.BigEndian.AppendUint16(tiff, 1) // one entry
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.BigEndian.AppendUint16(tiff, 3) // SHORT
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, o)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0) // value padding, no next IFD
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1}
	app1 = binary.BigEndian.AppendUint16(app1, uint16(len(seg)+2))
	app1 = append(app1, seg...)
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), app1...), b[2:]...)
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xc000 && g < 0x4000 && b < 0x4000
}

func isBlue(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return b > 0xc000 && r < 0x4000 && g < 0x4000
}

func TestPrepareTurnsAndShrinks(t *testing.T) {
	// Orientation 6: the camera was turned, the picture must be turned
	// clockwise, so the left (red) half ends up on top.
	f, err := attachments.Prepare("IMG_1.JPEG", withOrientation(t, halves(4000, 1000), 6))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "IMG_1.jpg" || f.ContentType != "image/jpeg" {
		t.Fatalf("got %q %q", f.Name, f.ContentType)
	}
	img := decode(t, f.Data)
	if b := img.Bounds(); b.Dx() != 500 || b.Dy() != attachments.FullEdge {
		t.Fatalf("size %v", b)
	}
	if !isRed(img.At(250, 100)) || !isBlue(img.At(250, 1900)) {
		t.Fatalf("not turned: top %v, bottom %v", img.At(250, 100), img.At(250, 1900))
	}
	if bytes.Contains(f.Data, []byte("Exif")) {
		t.Fatal("EXIF kept")
	}
	thumb := decode(t, f.Thumb)
	if b := thumb.Bounds(); b.Dx() != 200 || b.Dy() != attachments.ThumbEdge {
		t.Fatalf("thumb size %v", b)
	}
	again, err := attachments.Thumbnail(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	if b := decode(t, again).Bounds(); b != thumb.Bounds() {
		t.Fatalf("thumbnail size %v", b)
	}
}

func TestPrepareOther(t *testing.T) {
	// A transparent PNG gets a white background; small images keep their size.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 30, 20))); err != nil {
		t.Fatal(err)
	}
	f, err := attachments.Prepare(`C:\shots\screen.png`, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, f.Data)
	if r, g, b, _ := img.At(5, 5).RGBA(); f.Name != "screen.jpg" || img.Bounds().Dx() != 30 || r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Fatalf("got %q %v %v", f.Name, img.Bounds(), img.At(5, 5))
	}

	pdf := []byte("%PDF-1.7\n%âãÏÓ\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n")
	f, err = attachments.Prepare("Rechnung.pdf", pdf)
	if err != nil || f.ContentType != "application/pdf" || f.Thumb != nil || !bytes.Equal(f.Data, pdf) || f.Name != "Rechnung.pdf" {
		t.Fatalf("pdf: %+v %v", f.Name, err)
	}

	if _, err := attachments.Prepare("a.txt", []byte("hello")); !errors.Is(err, attachments.ErrType) {
		t.Fatalf("text: want ErrType, got %v", err)
	}
	if _, err := attachments.Prepare("a.jpg", []byte("\xff\xd8\xff\xe0broken")); !errors.Is(err, attachments.ErrImage) {
		t.Fatalf("broken: want ErrImage, got %v", err)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		" a\x00b.pdf ":    "ab.pdf",
		"../../etc/x.pdf": "x.pdf",
		"":                "",
	} {
		if got := attachments.CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddAndLink(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	bike, err := st.Bikes().CreateBike(ctx, store.Bike{ID: "b1", OwnerID: "o1", Name: "Gravel", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"c1", "c2"} {
		if _, err := st.Components().CreateComponent(ctx, store.Component{ID: id, OwnerID: "o1", Type: "chain", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	pdf := attachments.File{Name: "r.pdf", ContentType: "application/pdf", Data: []byte("%PDF-")}
	photo := attachments.File{Name: "p.jpg", ContentType: "image/jpeg", Data: []byte("x"), Thumb: []byte("y")}

	// The first image becomes the bike's photo, a later one doesn't.
	added, err := attachments.Add(ctx, st, "o1", store.AttachmentLink{BikeID: bike.ID}, []attachments.File{pdf, photo, photo})
	if err != nil || len(added) != 3 || added[0].Cover || !added[1].Cover || added[2].Cover {
		t.Fatalf("added %+v, %v", added, err)
	}
	if covers, _ := st.Attachments().Covers(ctx, "o1"); covers[bike.ID] != added[1].ID {
		t.Fatalf("covers %v", covers)
	}

	// A receipt for two components.
	receipt, err := attachments.Add(ctx, st, "o1", store.AttachmentLink{ComponentID: "c1"}, []attachments.File{pdf})
	if err != nil {
		t.Fatal(err)
	}
	if err := attachments.Link(ctx, st, "o1", store.AttachmentLink{ComponentID: "c2"}, []string{receipt[0].ID, receipt[0].ID}); err != nil {
		t.Fatal(err)
	}
	if l, err := st.Attachments().ListComponentAttachments(ctx, "o1", "c2"); err != nil || len(l) != 1 || l[0].ID != receipt[0].ID {
		t.Fatalf("c2 %+v, %v", l, err)
	}
	if err := attachments.Link(ctx, st, "o1", store.AttachmentLink{ComponentID: "c2"}, []string{"missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: want ErrNotFound, got %v", err)
	}

	// The last link goes only by deleting the attachment.
	if err := attachments.Unlink(ctx, st, "o1", store.AttachmentLink{AttachmentID: receipt[0].ID, ComponentID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if err := attachments.Unlink(ctx, st, "o1", store.AttachmentLink{AttachmentID: receipt[0].ID, ComponentID: "c2"}); !errors.Is(err, attachments.ErrLastLink) {
		t.Fatalf("last link: want ErrLastLink, got %v", err)
	}

	if err := attachments.Rename(ctx, st, "o1", receipt[0].ID, "  "); !errors.Is(err, attachments.ErrName) {
		t.Fatalf("empty name: %v", err)
	}
}
