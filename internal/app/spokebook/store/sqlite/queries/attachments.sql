-- name: CreateAttachment :exec
INSERT INTO attachments (id, owner_id, name, content_type, size, data, thumb, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAttachment :one
SELECT id, owner_id, name, content_type, size, thumb IS NOT NULL AS has_thumb, created_at
FROM attachments WHERE owner_id = ? AND id = ?;

-- name: GetAttachmentData :one
SELECT content_type, data FROM attachments WHERE owner_id = ? AND id = ?;

-- name: GetAttachmentThumb :one
SELECT thumb FROM attachments WHERE owner_id = ? AND id = ? AND thumb IS NOT NULL;

-- name: ListAttachments :many
SELECT id, owner_id, name, content_type, size, thumb IS NOT NULL AS has_thumb, created_at
FROM attachments WHERE owner_id = ? ORDER BY created_at DESC, id;

-- name: ListBikeAttachments :many
SELECT a.id, a.owner_id, a.name, a.content_type, a.size, a.thumb IS NOT NULL AS has_thumb, a.created_at, l.cover
FROM attachments a JOIN attachment_links l ON l.attachment_id = a.id
WHERE l.owner_id = ? AND l.bike_id = ?
ORDER BY l.cover DESC, a.created_at DESC, a.id;

-- name: ListComponentAttachments :many
SELECT a.id, a.owner_id, a.name, a.content_type, a.size, a.thumb IS NOT NULL AS has_thumb, a.created_at
FROM attachments a JOIN attachment_links l ON l.attachment_id = a.id
WHERE l.owner_id = ? AND l.component_id = ?
ORDER BY a.created_at DESC, a.id;

-- name: ListAttachmentLinks :many
SELECT attachment_id, bike_id, component_id, cover FROM attachment_links
WHERE owner_id = ? AND attachment_id = ?
ORDER BY bike_id IS NULL, bike_id, component_id;

-- name: ListAllAttachmentLinks :many
SELECT attachment_id, bike_id, component_id, cover FROM attachment_links
WHERE owner_id = ?
ORDER BY attachment_id, bike_id IS NULL, bike_id, component_id;

-- name: ListCovers :many
SELECT bike_id, attachment_id FROM attachment_links WHERE owner_id = ? AND cover = 1;

-- name: LinkAttachment :exec
INSERT INTO attachment_links (owner_id, attachment_id, bike_id, component_id, cover) VALUES (?, ?, ?, ?, ?);

-- name: UnlinkAttachment :execrows
DELETE FROM attachment_links
WHERE owner_id = ? AND attachment_id = ? AND (bike_id = ? OR component_id = ?);

-- name: ClearCover :exec
UPDATE attachment_links SET cover = 0 WHERE owner_id = ? AND bike_id = ? AND cover = 1;

-- name: SetCover :execrows
UPDATE attachment_links SET cover = 1 WHERE owner_id = ? AND bike_id = ? AND attachment_id = ?;

-- name: RenameAttachment :execrows
UPDATE attachments SET name = ? WHERE owner_id = ? AND id = ?;

-- name: DeleteAttachment :execrows
DELETE FROM attachments WHERE owner_id = ? AND id = ?;

-- name: DeleteOrphanAttachments :exec
DELETE FROM attachments
WHERE attachments.owner_id = ? AND NOT EXISTS (SELECT 1 FROM attachment_links l WHERE l.attachment_id = attachments.id);
