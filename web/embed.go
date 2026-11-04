// Package web bundles the embedded web assets: templates, static files and
// message catalogs.
package web

import "embed"

//go:embed templates/*.html
var Templates embed.FS

//go:embed static
var Static embed.FS

// ServiceWorker is the script served at /sw.js; it lives outside static
// because the server prepends the app version.
//
//go:embed sw.js
var ServiceWorker []byte

//go:embed locales/*.json
var Locales embed.FS
