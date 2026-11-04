package server

import (
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// parseDecimal parses a non-negative decimal ("1234.5" or "1234,5") and
// multiplies it by scale, e.g. 1000 for km to m or 100 for cents.
func parseDecimal(s string, scale, maxValue float64) (int64, bool) {
	v, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(s), ",", ".", 1), 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) || v > maxValue {
		return 0, false
	}
	return int64(math.Round(v * scale)), true
}

// parseNumber parses a finite decimal ("0.5" or "0,5"), also negative.
func parseNumber(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(s), ",", ".", 1), 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// parseKm parses a non-negative kilometer value into meters.
func parseKm(s string) (int64, bool) { return parseDecimal(s, 1000, 10_000_000) }

// optional applies parse to s unless s is blank, which yields nil.
func optional[T any](s string, parse func(string) (T, bool)) (*T, bool) {
	if strings.TrimSpace(s) == "" {
		return nil, true
	}
	v, ok := parse(s)
	if !ok {
		return nil, false
	}
	return &v, true
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	return t, err == nil
}

func parseCents(s string) (int64, bool) { return parseDecimal(s, 100, 100_000_000) }

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// parseCurrency accepts an ISO 4217 code in any case.
func parseCurrency(s string) (string, bool) {
	c := strings.ToUpper(strings.TrimSpace(s))
	return c, currencyRe.MatchString(c)
}

// parseInterval parses an interval from optional months (a whole number)
// and kilometers; the catalog checks the bounds.
func parseInterval(months, km string) (store.Interval, bool) {
	var iv store.Interval
	var ok bool
	if iv.Months, ok = optional(months, func(s string) (int64, bool) {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n, err == nil
	}); !ok {
		return iv, false
	}
	iv.DistanceM, ok = optional(km, parseKm)
	return iv, ok
}

// parseURL accepts an absolute http(s) URL.
func parseURL(s string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

// The following format values for prefilling form fields. Number inputs
// always use a dot as decimal separator.

func kmValue(meters int64) string {
	return strconv.FormatFloat(float64(meters)/1000, 'f', -1, 64)
}

func centsValue(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

func monthsValue(n int64) string { return strconv.FormatInt(n, 10) }

func dateValue(t time.Time) string { return t.Format(time.DateOnly) }

// optValue formats an optional value, or returns "" for nil.
func optValue[T any](v *T, format func(T) string) string {
	if v == nil {
		return ""
	}
	return format(*v)
}
