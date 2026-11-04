// Package i18n loads message catalogs and picks the language for a request.
// Catalogs are flat JSON files mapping message IDs to texts, one per language
// (e.g. en.json). English is the fallback for missing languages and keys.
package i18n

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

// Fallback is the language used when nothing else matches.
var Fallback = language.English

type catalog map[string]string

// Bundle holds all loaded catalogs.
type Bundle struct {
	tags     []language.Tag
	catalogs []catalog
	matcher  language.Matcher
}

// Load reads every *.json file in dir; the file name is the language tag.
// A catalog for Fallback is required.
func Load(fsys fs.FS, dir string) (*Bundle, error) {
	files, err := fs.Glob(fsys, path.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	b := &Bundle{}
	for _, f := range files {
		tag, err := language.Parse(strings.TrimSuffix(path.Base(f), ".json"))
		if err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", f, err)
		}
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var c catalog
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", f, err)
		}
		// The matcher prefers the first tag, so the fallback goes first.
		if tag == Fallback {
			b.tags = append([]language.Tag{tag}, b.tags...)
			b.catalogs = append([]catalog{c}, b.catalogs...)
		} else {
			b.tags = append(b.tags, tag)
			b.catalogs = append(b.catalogs, c)
		}
	}
	if len(b.tags) == 0 || b.tags[0] != Fallback {
		return nil, fmt.Errorf("i18n: no catalog for fallback language %s", Fallback)
	}
	b.matcher = language.NewMatcher(b.tags)
	return b, nil
}

// Languages returns the loaded languages, fallback first.
func (b *Bundle) Languages() []language.Tag { return b.tags }

// Keys returns the message IDs of a language's catalog.
func (b *Bundle) Keys(tag language.Tag) []string {
	for i, t := range b.tags {
		if t == tag {
			keys := make([]string, 0, len(b.catalogs[i]))
			for k := range b.catalogs[i] {
				keys = append(keys, k)
			}
			return keys
		}
	}
	return nil
}

// Localizer picks the best language for an Accept-Language header value.
func (b *Bundle) Localizer(acceptLanguage string) *Localizer {
	desired, _, _ := language.ParseAcceptLanguage(acceptLanguage)
	_, i, _ := b.matcher.Match(desired...)
	return &Localizer{
		tag: b.tags[i], msgs: b.catalogs[i], fallback: b.catalogs[0],
		printer:  message.NewPrinter(b.tags[i]),
		collator: collate.New(b.tags[i], collate.IgnoreCase),
	}
}

// Localizer translates messages into one language.
type Localizer struct {
	tag      language.Tag
	msgs     catalog
	fallback catalog
	printer  *message.Printer
	collator *collate.Collator
}

// Compare compares two strings in the language's sort order, ignoring case.
func (l *Localizer) Compare(a, b string) int { return l.collator.CompareString(a, b) }

// Lang returns the BCP 47 tag, e.g. for the html lang attribute.
func (l *Localizer) Lang() string { return l.tag.String() }

// Distance formats meters as kilometers with at most one decimal,
// e.g. "1.234,5 km" in German.
func (l *Localizer) Distance(meters int64) string {
	return l.printer.Sprintf("%v km", number.Decimal(float64(meters)/1000, number.MaxFractionDigits(1)))
}

// Number formats a decimal number with up to four decimals, e.g. "0,5".
func (l *Localizer) Number(f float64) string {
	return l.printer.Sprint(number.Decimal(f, number.MaxFractionDigits(4)))
}

// Money formats an amount in cents with its currency code, e.g.
// "1.249,90 EUR" in German.
func (l *Localizer) Money(cents int64, currency string) string {
	return l.printer.Sprintf("%v %s",
		number.Decimal(float64(cents)/100, number.MinFractionDigits(2), number.MaxFractionDigits(2)), currency)
}

// Date formats the date part of t using the catalog's "format.date" layout.
func (l *Localizer) Date(t time.Time) string {
	return t.Format(l.T("format.date"))
}

// T returns the text for id, formatted with args like fmt.Sprintf. Missing
// IDs fall back to English, then to the ID itself.
func (l *Localizer) T(id string, args ...any) string {
	msg, ok := l.msgs[id]
	if !ok {
		msg, ok = l.fallback[id]
	}
	if !ok {
		return id
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// N returns the text for n of something: the message id+".one" for n == 1,
// otherwise id+".other", formatted with n and args. These two forms cover
// English and German.
func (l *Localizer) N(id string, n int, args ...any) string {
	form := ".other"
	if n == 1 {
		form = ".one"
	}
	return l.T(id+form, append([]any{n}, args...)...)
}

type ctxKey struct{}

// WithLocalizer stores l in ctx.
func WithLocalizer(ctx context.Context, l *Localizer) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the Localizer stored in ctx.
func FromContext(ctx context.Context) *Localizer {
	l, _ := ctx.Value(ctxKey{}).(*Localizer)
	return l
}
