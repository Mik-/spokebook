package sqlite

import (
	"context"
	"database/sql"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite/db"
)

type attachmentStore struct{ q *db.Queries }

func toAttachment(r db.GetAttachmentRow, cover bool) (store.Attachment, error) {
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return store.Attachment{}, err
	}
	return store.Attachment{
		ID: r.ID, OwnerID: r.OwnerID, Name: r.Name, ContentType: r.ContentType, Size: r.Size,
		HasThumb: r.HasThumb, Cover: cover, CreatedAt: created,
	}, nil
}

func toAttachmentLink(attachmentID string, bikeID, componentID sql.NullString, cover int64) store.AttachmentLink {
	return store.AttachmentLink{
		AttachmentID: attachmentID, BikeID: bikeID.String, ComponentID: componentID.String, Cover: cover == 1,
	}
}

func (s attachmentStore) CreateAttachment(ctx context.Context, a store.Attachment) error {
	err := s.q.CreateAttachment(ctx, db.CreateAttachmentParams{
		ID: a.ID, OwnerID: a.OwnerID, Name: a.Name, ContentType: a.ContentType, Size: int64(len(a.Data)),
		Data: a.Data, Thumb: a.Thumb, CreatedAt: formatTime(a.CreatedAt),
	})
	return mapErr(err)
}

func (s attachmentStore) GetAttachment(ctx context.Context, ownerID, id string) (store.Attachment, error) {
	r, err := s.q.GetAttachment(ctx, db.GetAttachmentParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.Attachment{}, mapErr(err)
	}
	return toAttachment(r, false)
}

func (s attachmentStore) GetAttachmentData(ctx context.Context, ownerID, id string) (string, []byte, error) {
	r, err := s.q.GetAttachmentData(ctx, db.GetAttachmentDataParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return "", nil, mapErr(err)
	}
	return r.ContentType, r.Data, nil
}

func (s attachmentStore) GetAttachmentThumb(ctx context.Context, ownerID, id string) ([]byte, error) {
	thumb, err := s.q.GetAttachmentThumb(ctx, db.GetAttachmentThumbParams{OwnerID: ownerID, ID: id})
	return thumb, mapErr(err)
}

func (s attachmentStore) ListAttachments(ctx context.Context, ownerID string) ([]store.Attachment, error) {
	rows, err := s.q.ListAttachments(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.Attachment, 0, len(rows))
	for _, r := range rows {
		a, err := toAttachment(db.GetAttachmentRow(r), false)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (s attachmentStore) ListBikeAttachments(ctx context.Context, ownerID, bikeID string) ([]store.Attachment, error) {
	rows, err := s.q.ListBikeAttachments(ctx, db.ListBikeAttachmentsParams{OwnerID: ownerID, BikeID: nullString(bikeID)})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.Attachment, 0, len(rows))
	for _, r := range rows {
		a, err := toAttachment(db.GetAttachmentRow{
			ID: r.ID, OwnerID: r.OwnerID, Name: r.Name, ContentType: r.ContentType, Size: r.Size,
			HasThumb: r.HasThumb, CreatedAt: r.CreatedAt,
		}, r.Cover == 1)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (s attachmentStore) ListComponentAttachments(ctx context.Context, ownerID, componentID string) ([]store.Attachment, error) {
	rows, err := s.q.ListComponentAttachments(ctx, db.ListComponentAttachmentsParams{OwnerID: ownerID, ComponentID: nullString(componentID)})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.Attachment, 0, len(rows))
	for _, r := range rows {
		a, err := toAttachment(db.GetAttachmentRow(r), false)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (s attachmentStore) ListAttachmentLinks(ctx context.Context, ownerID, id string) ([]store.AttachmentLink, error) {
	rows, err := s.q.ListAttachmentLinks(ctx, db.ListAttachmentLinksParams{OwnerID: ownerID, AttachmentID: id})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.AttachmentLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAttachmentLink(r.AttachmentID, r.BikeID, r.ComponentID, r.Cover))
	}
	return out, nil
}

func (s attachmentStore) ListAllAttachmentLinks(ctx context.Context, ownerID string) ([]store.AttachmentLink, error) {
	rows, err := s.q.ListAllAttachmentLinks(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.AttachmentLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAttachmentLink(r.AttachmentID, r.BikeID, r.ComponentID, r.Cover))
	}
	return out, nil
}

func (s attachmentStore) Covers(ctx context.Context, ownerID string) (map[string]string, error) {
	rows, err := s.q.ListCovers(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.BikeID.String] = r.AttachmentID
	}
	return out, nil
}

func (s attachmentStore) LinkAttachment(ctx context.Context, ownerID string, l store.AttachmentLink) error {
	if l.Cover {
		if err := s.q.ClearCover(ctx, db.ClearCoverParams{OwnerID: ownerID, BikeID: nullString(l.BikeID)}); err != nil {
			return mapErr(err)
		}
	}
	err := s.q.LinkAttachment(ctx, db.LinkAttachmentParams{
		OwnerID: ownerID, AttachmentID: l.AttachmentID, BikeID: nullString(l.BikeID),
		ComponentID: nullString(l.ComponentID), Cover: boolInt(l.Cover),
	})
	return mapErr(err)
}

func (s attachmentStore) UnlinkAttachment(ctx context.Context, ownerID string, l store.AttachmentLink) error {
	n, err := s.q.UnlinkAttachment(ctx, db.UnlinkAttachmentParams{
		OwnerID: ownerID, AttachmentID: l.AttachmentID, BikeID: nullString(l.BikeID), ComponentID: nullString(l.ComponentID),
	})
	if err := rowsOrNotFound(n, err); err != nil {
		return err
	}
	return mapErr(s.q.DeleteOrphanAttachments(ctx, ownerID))
}

func (s attachmentStore) SetCover(ctx context.Context, ownerID, bikeID, attachmentID string) error {
	if err := s.q.ClearCover(ctx, db.ClearCoverParams{OwnerID: ownerID, BikeID: nullString(bikeID)}); err != nil {
		return mapErr(err)
	}
	if attachmentID == "" {
		return nil
	}
	n, err := s.q.SetCover(ctx, db.SetCoverParams{OwnerID: ownerID, BikeID: nullString(bikeID), AttachmentID: attachmentID})
	return rowsOrNotFound(n, err)
}

func (s attachmentStore) RenameAttachment(ctx context.Context, ownerID, id, name string) error {
	n, err := s.q.RenameAttachment(ctx, db.RenameAttachmentParams{Name: name, OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s attachmentStore) DeleteAttachment(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteAttachment(ctx, db.DeleteAttachmentParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}
