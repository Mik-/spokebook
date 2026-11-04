-- +goose Up
-- Files such as photos and receipts. Images are stored re-encoded as JPEG
-- with a smaller preview (thumb); PDFs as uploaded without a preview.
CREATE TABLE attachments (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    -- File name shown and used for downloads.
    name         TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size         INTEGER NOT NULL CHECK (size >= 0),
    data         BLOB NOT NULL,
    thumb        BLOB,
    created_at   TEXT NOT NULL
) STRICT;

CREATE INDEX attachments_owner_idx ON attachments (owner_id, created_at);
CREATE UNIQUE INDEX attachments_id_owner_idx ON attachments (id, owner_id);

-- What an attachment belongs to: a bike or a component. One attachment can
-- belong to several, e.g. a receipt for several components. An attachment
-- without links is deleted.
CREATE TABLE attachment_links (
    owner_id      TEXT NOT NULL,
    attachment_id TEXT NOT NULL,
    bike_id       TEXT,
    component_id  TEXT,
    -- The bike's photo on the start page and the bike page.
    cover         INTEGER NOT NULL DEFAULT 0 CHECK (cover IN (0, 1)),
    CHECK ((bike_id IS NULL) != (component_id IS NULL)),
    CHECK (cover = 0 OR bike_id IS NOT NULL),
    FOREIGN KEY (attachment_id, owner_id) REFERENCES attachments (id, owner_id) ON DELETE CASCADE,
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id) ON DELETE CASCADE,
    FOREIGN KEY (component_id, owner_id) REFERENCES components (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE UNIQUE INDEX attachment_links_bike_idx ON attachment_links (bike_id, attachment_id) WHERE bike_id IS NOT NULL;
CREATE UNIQUE INDEX attachment_links_component_idx ON attachment_links (component_id, attachment_id) WHERE component_id IS NOT NULL;
CREATE INDEX attachment_links_attachment_idx ON attachment_links (attachment_id);
-- At most one cover per bike.
CREATE UNIQUE INDEX attachment_links_cover_idx ON attachment_links (bike_id) WHERE cover = 1;

-- +goose Down
DROP TABLE attachment_links;
DROP TABLE attachments;
