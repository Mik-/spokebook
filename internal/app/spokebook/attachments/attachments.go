// Package attachments checks uploaded files (photos, PDFs) and links them
// to bikes and components.
package attachments

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// MaxFileSize limits an uploaded file.
const MaxFileSize = 25 << 20

const (
	typeJPEG = "image/jpeg"
	typePDF  = "application/pdf"
	// maxName limits a file name in characters.
	maxName = 200
)

var (
	// ErrType means the file is neither a supported image (JPEG, PNG,
	// WebP, GIF) nor a PDF.
	ErrType = errors.New("attachments: unsupported file type")
	// ErrImage means the image can't be read or is too large.
	ErrImage = errors.New("attachments: invalid image")
	// ErrName means the name is empty.
	ErrName = errors.New("attachments: name required")
	// ErrLastLink means the link is the attachment's last one; it goes only
	// by deleting the attachment.
	ErrLastLink = errors.New("attachments: last link")
)

// File is an upload ready to be stored.
type File struct {
	Name        string
	ContentType string
	Data        []byte
	Thumb       []byte
}

// IsImage reports whether a stored attachment is an image.
func IsImage(contentType string) bool { return contentType == typeJPEG }

// Prepare checks an upload by its content. Images are stored as JPEG (see
// prepareImage), PDFs as they are.
func Prepare(name string, data []byte) (File, error) {
	name = CleanName(name)
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		full, thumb, err := prepareImage(data)
		if err != nil {
			return File{}, err
		}
		name = strings.TrimSuffix(name, path.Ext(name))
		if name == "" {
			name = "photo"
		}
		return File{Name: name + ".jpg", ContentType: typeJPEG, Data: full, Thumb: thumb}, nil
	case typePDF:
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return File{}, ErrType
		}
		if name == "" {
			name = "document.pdf"
		}
		return File{Name: name, ContentType: typePDF, Data: data}, nil
	}
	return File{}, ErrType
}

// CleanName keeps the base name of a file without control characters and
// shortens it to maxName characters; "" stays empty.
func CleanName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxName {
		name = string([]rune(name)[:maxName])
	}
	return name
}

// Add stores the files and links them to the bike or component of to. On
// a bike without a photo, the first image becomes it. It returns the new
// attachments in the order of files.
func Add(ctx context.Context, st store.Store, ownerID string, to store.AttachmentLink, files []File) ([]store.Attachment, error) {
	now := time.Now().UTC()
	out := make([]store.Attachment, 0, len(files))
	err := st.WithTx(ctx, func(tx store.Stores) error {
		hasCover := true
		if to.BikeID != "" {
			covers, err := tx.Attachments().Covers(ctx, ownerID)
			if err != nil {
				return err
			}
			_, hasCover = covers[to.BikeID]
		}
		for _, f := range files {
			a := store.Attachment{
				ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID, Name: f.Name, ContentType: f.ContentType,
				Size: int64(len(f.Data)), Data: f.Data, Thumb: f.Thumb, HasThumb: f.Thumb != nil, CreatedAt: now,
			}
			if err := tx.Attachments().CreateAttachment(ctx, a); err != nil {
				return err
			}
			l := store.AttachmentLink{AttachmentID: a.ID, BikeID: to.BikeID, ComponentID: to.ComponentID}
			if !hasCover && IsImage(a.ContentType) {
				l.Cover, a.Cover, hasCover = true, true, true
			}
			if err := tx.Attachments().LinkAttachment(ctx, ownerID, l); err != nil {
				return err
			}
			a.Data, a.Thumb = nil, nil
			out = append(out, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Link links existing attachments to the bike or component of to; those
// already linked are skipped.
func Link(ctx context.Context, st store.Store, ownerID string, to store.AttachmentLink, ids []string) error {
	return st.WithTx(ctx, func(tx store.Stores) error {
		for _, id := range ids {
			l := store.AttachmentLink{AttachmentID: id, BikeID: to.BikeID, ComponentID: to.ComponentID}
			if err := tx.Attachments().LinkAttachment(ctx, ownerID, l); err != nil && !errors.Is(err, store.ErrConflict) {
				return err
			}
		}
		return nil
	})
}

// Unlink removes the attachment from a bike or component, unless it is its
// last link (ErrLastLink).
func Unlink(ctx context.Context, st store.Store, ownerID string, l store.AttachmentLink) error {
	return st.WithTx(ctx, func(tx store.Stores) error {
		links, err := tx.Attachments().ListAttachmentLinks(ctx, ownerID, l.AttachmentID)
		if err != nil {
			return err
		}
		if len(links) < 2 {
			return ErrLastLink
		}
		return tx.Attachments().UnlinkAttachment(ctx, ownerID, l)
	})
}

// Rename changes the name shown and used for downloads.
func Rename(ctx context.Context, st store.Store, ownerID, id, name string) error {
	name = CleanName(name)
	if name == "" {
		return ErrName
	}
	return st.Attachments().RenameAttachment(ctx, ownerID, id, name)
}
