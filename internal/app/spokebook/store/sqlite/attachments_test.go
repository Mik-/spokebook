package sqlite_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func TestAttachments(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	as := s.Attachments()
	bike, err := s.Bikes().CreateBike(ctx, store.Bike{ID: newID(), OwnerID: "o1", Name: "Gravel", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var comps []store.Component
	for range 2 {
		c, err := s.Components().CreateComponent(ctx, store.Component{ID: newID(), OwnerID: "o1", Type: "chain", CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		comps = append(comps, c)
	}
	photo := store.Attachment{
		ID: newID(), OwnerID: "o1", Name: "bike.jpg", ContentType: "image/jpeg",
		Data: []byte("full"), Thumb: []byte("small"), CreatedAt: time.Now(),
	}
	receipt := store.Attachment{
		ID: newID(), OwnerID: "o1", Name: "receipt.pdf", ContentType: "application/pdf",
		Data: []byte("%PDF-"), CreatedAt: time.Now().Add(time.Second),
	}
	for _, a := range []store.Attachment{photo, receipt} {
		if err := as.CreateAttachment(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	links := []store.AttachmentLink{
		{AttachmentID: photo.ID, BikeID: bike.ID, Cover: true},
		{AttachmentID: receipt.ID, BikeID: bike.ID},
		{AttachmentID: receipt.ID, ComponentID: comps[0].ID},
		{AttachmentID: receipt.ID, ComponentID: comps[1].ID},
	}
	for _, l := range links {
		if err := as.LinkAttachment(ctx, "o1", l); err != nil {
			t.Fatal(err)
		}
	}
	if err := as.LinkAttachment(ctx, "o1", links[2]); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("link twice: want ErrConflict, got %v", err)
	}
	if err := as.LinkAttachment(ctx, "other", store.AttachmentLink{AttachmentID: photo.ID, ComponentID: comps[0].ID}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other owner: want ErrNotFound, got %v", err)
	}

	// The cover comes first; the data stays out of lists.
	list, err := as.ListBikeAttachments(ctx, "o1", bike.ID)
	if err != nil || len(list) != 2 || list[0].ID != photo.ID || !list[0].Cover || list[1].Cover || list[0].Data != nil || !list[0].HasThumb || list[1].HasThumb {
		t.Fatalf("bike attachments %+v, %v", list, err)
	}
	if list[0].Size != 4 {
		t.Fatalf("size %d", list[0].Size)
	}
	covers, err := as.Covers(ctx, "o1")
	if err != nil || covers[bike.ID] != photo.ID {
		t.Fatalf("covers %v, %v", covers, err)
	}
	ct, data, err := as.GetAttachmentData(ctx, "o1", photo.ID)
	if err != nil || ct != "image/jpeg" || !bytes.Equal(data, photo.Data) {
		t.Fatalf("data %q %q %v", ct, data, err)
	}
	if thumb, err := as.GetAttachmentThumb(ctx, "o1", photo.ID); err != nil || string(thumb) != "small" {
		t.Fatalf("thumb %q %v", thumb, err)
	}
	if _, err := as.GetAttachmentThumb(ctx, "o1", receipt.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("pdf thumb: want ErrNotFound, got %v", err)
	}
	if _, _, err := as.GetAttachmentData(ctx, "other", photo.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other owner: want ErrNotFound, got %v", err)
	}

	// Moving the cover; a bike has at most one.
	if err := as.LinkAttachment(ctx, "o1", store.AttachmentLink{AttachmentID: receipt.ID, BikeID: bike.ID, Cover: true}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("link again: %v", err)
	}
	if err := as.SetCover(ctx, "o1", bike.ID, receipt.ID); err != nil {
		t.Fatal(err)
	}
	if covers, _ := as.Covers(ctx, "o1"); covers[bike.ID] != receipt.ID {
		t.Fatalf("covers %v", covers)
	}
	if err := as.SetCover(ctx, "o1", bike.ID, comps[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cover not linked: want ErrNotFound, got %v", err)
	}
	if err := as.SetCover(ctx, "o1", bike.ID, ""); err != nil {
		t.Fatal(err)
	}
	if covers, _ := as.Covers(ctx, "o1"); len(covers) != 0 {
		t.Fatalf("covers %v", covers)
	}

	// Unlinking keeps the attachment while it has other links.
	if err := as.UnlinkAttachment(ctx, "o1", links[2]); err != nil {
		t.Fatal(err)
	}
	if l, err := as.ListAttachmentLinks(ctx, "o1", receipt.ID); err != nil || len(l) != 2 || l[0].BikeID != bike.ID || l[1].ComponentID != comps[1].ID {
		t.Fatalf("links %+v, %v", l, err)
	}
	if err := as.UnlinkAttachment(ctx, "o1", links[2]); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unlink twice: want ErrNotFound, got %v", err)
	}
	// Deleting the component and the bike deletes what only they had.
	if err := s.Components().DeleteComponent(ctx, "o1", comps[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := as.GetAttachment(ctx, "o1", receipt.ID); err != nil {
		t.Fatalf("receipt still on the bike: %v", err)
	}
	if err := s.Bikes().DeleteBike(ctx, "o1", bike.ID); err != nil {
		t.Fatal(err)
	}
	if all, err := as.ListAttachments(ctx, "o1"); err != nil || len(all) != 0 {
		t.Fatalf("left %+v, %v", all, err)
	}
}

func TestDeleteAttachment(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	as := s.Attachments()
	c, err := s.Components().CreateComponent(ctx, store.Component{ID: newID(), OwnerID: "o1", Type: "chain", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	a := store.Attachment{ID: newID(), OwnerID: "o1", Name: "a.pdf", ContentType: "application/pdf", Data: []byte("x"), CreatedAt: time.Now()}
	if err := as.CreateAttachment(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := as.LinkAttachment(ctx, "o1", store.AttachmentLink{AttachmentID: a.ID, ComponentID: c.ID}); err != nil {
		t.Fatal(err)
	}
	if err := as.RenameAttachment(ctx, "o1", a.ID, "b.pdf"); err != nil {
		t.Fatal(err)
	}
	if got, err := as.GetAttachment(ctx, "o1", a.ID); err != nil || got.Name != "b.pdf" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := as.DeleteAttachment(ctx, "o1", a.ID); err != nil {
		t.Fatal(err)
	}
	if l, err := as.ListComponentAttachments(ctx, "o1", c.ID); err != nil || len(l) != 0 {
		t.Fatalf("left %+v, %v", l, err)
	}
	if err := as.DeleteAttachment(ctx, "o1", a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete twice: want ErrNotFound, got %v", err)
	}
}
