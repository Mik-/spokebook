package i18n_test

import (
	"slices"
	"testing"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/web"
)

func load(t *testing.T) *i18n.Bundle {
	t.Helper()
	b, err := i18n.Load(web.Locales, "locales")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Every catalog must have exactly the keys of the fallback catalog.
func TestCatalogsComplete(t *testing.T) {
	b := load(t)
	want := b.Keys(i18n.Fallback)
	slices.Sort(want)
	for _, tag := range b.Languages()[1:] {
		got := b.Keys(tag)
		slices.Sort(got)
		for _, k := range want {
			if _, ok := slices.BinarySearch(got, k); !ok {
				t.Errorf("%s: missing key %q", tag, k)
			}
		}
		for _, k := range got {
			if _, ok := slices.BinarySearch(want, k); !ok {
				t.Errorf("%s: extra key %q", tag, k)
			}
		}
	}
}

func TestLocalizer(t *testing.T) {
	b := load(t)
	tests := []struct {
		accept, lang, title string
	}{
		{"de-DE,de;q=0.9,en;q=0.8", "de", "Fahrräder"},
		{"de-AT", "de", "Fahrräder"},
		{"en-US", "en", "Bikes"},
		{"fr-FR", "en", "Bikes"},
		{"", "en", "Bikes"},
	}
	for _, tt := range tests {
		l := b.Localizer(tt.accept)
		if l.Lang() != tt.lang || l.T("bikes.title") != tt.title {
			t.Errorf("%q: got %s/%q, want %s/%q", tt.accept, l.Lang(), l.T("bikes.title"), tt.lang, tt.title)
		}
	}
	if got := b.Localizer("de").T("no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key: got %q", got)
	}
}

func TestPlural(t *testing.T) {
	b := load(t)
	for _, tt := range []struct {
		lang string
		n    int
		want string
	}{
		{"de", 1, "1 Komponente verbaut."},
		{"de", 3, "3 Komponenten verbaut."},
		{"en", 0, "0 components installed."},
		{"en", 1, "1 component installed."},
	} {
		if got := b.Localizer(tt.lang).N("bike.components.count", tt.n); got != tt.want {
			t.Errorf("%s N(%d) = %q, want %q", tt.lang, tt.n, got, tt.want)
		}
	}
}

func TestFormat(t *testing.T) {
	b := load(t)
	date := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		lang     string
		meters   int64
		distance string
		date     string
	}{
		{"de", 1_234_567, "1.234,6 km", "25.10.2026"},
		{"de", 12_000, "12 km", "25.10.2026"},
		{"en", 1_234_567, "1,234.6 km", "2026-10-25"},
	}
	for _, tt := range tests {
		l := b.Localizer(tt.lang)
		if got := l.Distance(tt.meters); got != tt.distance {
			t.Errorf("%s Distance(%d) = %q, want %q", tt.lang, tt.meters, got, tt.distance)
		}
		if got := l.Date(date); got != tt.date {
			t.Errorf("%s Date = %q, want %q", tt.lang, got, tt.date)
		}
	}
	if got := b.Localizer("de").Money(124990, "EUR"); got != "1.249,90 EUR" {
		t.Errorf("de Money = %q", got)
	}
	if got := b.Localizer("en").Money(5, "USD"); got != "0.05 USD" {
		t.Errorf("en Money = %q", got)
	}
}
