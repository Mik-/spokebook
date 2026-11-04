# CLAUDE.md

## Project

**Spokebook**: self-hosted web app for documenting bicycle maintenance. It manages bikes, components
(purchase, price, mileage at installation, wear limits), a catalog of maintenance steps ordered by
phase and assembly group and filtered by level (torques, wear limits) and performed maintenance.
Mileage comes from uploaded FIT, GPX and TCX files and from Garmin Connect. Open source under AGPL-3.0.

**The full concept is in [docs/konzept.md](docs/konzept.md) (German). Read it before larger
changes.**

## Language

Everything is in English: code, comments, commit messages, README and other docs. The only exceptions
for now are `docs/konzept.md` and its appendix `docs/konzept-schritte.md`, which stay German. IDs in
code and database (value kinds, phases, assembly groups, levels, value keys ...) are always English.

The UI is multilingual (English and German). Never hard-code user-facing text: put it in the
catalogs `web/locales/<lang>.json` under a message ID (`area.thing`, e.g. `bikes.field.name`) and
use `{{t "id"}}` in templates or `i18n.FromContext(ctx).T("id")` in handlers. Texts with a count use
`id.one` and `id.other` and `{{tn "id" n}}` (`Localizer.N`). Every catalog must
contain the same keys (enforced by a test). The language comes from `Accept-Language`; English is the
fallback.

## Stack

- Go, only `net/http` with ServeMux routing from the standard library, no web framework
- Server-side HTML (`html/template`), htmx and Pico CSS (both vendored in `web/static/vendor`, pinned
  version) plus a small `app.css`; no SPA, no JS build, no offline mode. Every form works without
  JavaScript; htmx only improves it (e.g. autosave of an open maintenance) and is loaded via the
  `htmx` partial, whose config keeps it within the CSP (no inline styles, no eval)

## UI conventions

- Every page below a main list starts with breadcrumbs (`<nav aria-label="breadcrumb">`).
- Page head: `<div class="page-head">` with the title and the actions on the right. "+ New" is a
  filled button, other actions are `role="button" class="outline"`.
- Creating, editing and actions such as install or retire happen on their own form pages, started
  from buttons; no forms below lists. Form submit buttons are filled.
- Actions in table rows are compact outline buttons in `<td class="row-action">`.
- Destructive actions (delete, retire) use `class="outline danger"` and sit at the end of the page;
  deleting asks on a confirmation page.
- Required fields carry `<span class="req" aria-hidden="true">*</span>`; field pairs use
  `<div class="grid">`; tables sit in `<div class="overflow-auto">` and have a header row.
- On phones, list tables become cards: the first cell is the title, every other cell (except
  `row-action`) carries `data-label="{{t "<column head id>"}}"`. Form tables use `class="value-rows"`
  and keep their columns. Check new pages at 375 px width (headless chromium is in the dev container).
- SQLite with sqlc (queries) and goose (migrations, embedded, run at startup)
- Live reload with air; development in the dev container (`.devcontainer/`)

## Conventions

- **Data layer**: Business logic only uses the interfaces in package `store`. SQLite code lives
  exclusively in `store/sqlite`, with queries and migrations in `store/sqlite/migrations` (full paths
  see directory layout). Never expose SQLite types or errors; return own errors such as
  `store.ErrNotFound`. Postgres is neither implemented nor tested, but the abstraction must allow it.
- **Types**: IDs as UUIDv7, times in UTC, distances in meters (int), money in cents (int) plus
  currency.
- Every domain table has an `owner_id`, even though there is only one user for now.
- A maintenance copies the catalog steps when it starts, so its history stays independent of later
  catalog changes.
- Multi-step writes run in a transaction (`WithTx`).
- **Security**: only parameterized queries (sqlc), templates with auto-escaping, no inline scripts
  (strict CSP), CSRF protection for all state-changing requests, never log secrets.
- Tests run against the store interfaces with SQLite (in-memory or temporary file).
- Format with gofumpt, lint with golangci-lint.
- Commit messages follow Conventional Commits (`feat:`, `fix:`, ...); semantic-release derives the
  next version from them on every push to `main`.
- After generating code (`sqlc generate`), commit the generated files.

## Directory layout

Following [golang-standards/project-layout](https://github.com/golang-standards/project-layout):

- `cmd/spokebook`: entry point, only calls `spokebook.Run`
- `internal/app/spokebook`: wiring (configuration, database, HTTP server)
- `internal/app/spokebook/server`: HTTP handlers; they parse forms and render, but contain no business
  rules
- `internal/app/spokebook/bikes`, `.../odometer`, `.../components`, `.../catalog`, `.../maintenance`, `.../due`, `.../rides`: business logic per area (validation across
  rows, transactions, computed values such as mileage, wear and due dates); used by handlers and later by imports
- `internal/app/spokebook/rides/garmin`: fetches rides from Garmin Connect (Personal API, daily job)
- `internal/app/spokebook/secret`: seals credentials with `APP_SECRET_KEY`
- `internal/app/spokebook/attachments`: checks uploaded photos and PDFs (photos shrunk, turned by EXIF
  and re-encoded as JPEG without metadata) and links them to bikes and components
- `internal/app/spokebook/rides/parse`: reads rides (start, end, distance, bike) from FIT, GPX and TCX
  files; the odometer of a bike is computed from manual readings and rides (`odometer.Timeline`)
- `internal/app/spokebook/auth`: accounts, argon2id passwords, lockout, sessions, setup token,
  passkeys (go-webauthn); handlers get the logged-in user via `ownerID(ctx)`
- `internal/app/spokebook/backup`: export and import of all data of an owner as versioned JSON (its
  own file format types, independent of `store`; raise `Version` for incompatible changes)
- `internal/app/spokebook/values`: checks values (key, kind, label, from/to, unit, hint) of components,
  component types and steps
- `internal/app/spokebook/i18n`: message catalogs and language selection
- `internal/app/spokebook/store`: data layer interfaces
- `internal/app/spokebook/store/sqlite`: SQLite implementation, queries, migrations, sqlc code
- `web`: embedded templates, static files and message catalogs (`web/locales`)
- `docs`: concept

No `pkg/`, the app is not a library.

## Development

- Configuration via environment variables: `APP_ADDR` (default `localhost:8080`), `APP_DB_PATH`
  (`data/dev.sqlite`), `APP_BASE_URL` (`http://localhost:8080`, for passkeys and secure cookies), `APP_TRUSTED_PROXIES`
  (reverse proxies whose `X-Forwarded-For` is believed, for the login rate limit), `APP_LOG_LEVEL`
  (`info`; `debug` logs every request), `APP_SECRET_KEY` (at least 32 characters; encrypts credentials
  of ride sources such as the Garmin API key; without it Garmin Connect is disabled).
- Without an account, the server logs a setup link with a one-time token at startup.
- Always open the app via `http://localhost:8080` (not 127.0.0.1), otherwise passkeys won't work.
- `data/` holds local databases and is not checked in.
