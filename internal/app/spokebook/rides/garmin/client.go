// Package garmin fetches rides from Garmin Connect through the Personal
// API: each user creates an API key in Garmin Connect, exchanges it for an
// access token and lists the cycling activities; the original file of each
// new one is read like an uploaded FIT file.
package garmin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BaseURL is the address of the Personal API (".cn" in China).
const BaseURL = "https://personal.connectapi.garmin.com"

// maxDownload limits an activity file as Garmin sends it.
const maxDownload = 64 << 20

var (
	// ErrUnauthorized means Garmin refused the API key or token, e.g.
	// because the key expired or was deleted.
	ErrUnauthorized = errors.New("garmin: unauthorized")
	// ErrRateLimited means Garmin asks to slow down.
	ErrRateLimited = errors.New("garmin: rate limited")
)

// Client calls the Personal API. Requests are spaced by Wait, because
// Garmin may enforce a rate limit without telling it.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Wait    time.Duration

	mu   sync.Mutex
	last time.Time
}

// NewClient returns a client for the Personal API at baseURL.
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: time.Minute}, Wait: 2 * time.Second}
}

// Activity is an activity in the list.
type Activity struct {
	ID int64 `json:"activityId"`
	// BeginTimestamp is the start in milliseconds since 1970 (UTC).
	BeginTimestamp int64  `json:"beginTimestamp"`
	Name           string `json:"activityName"`
	Manual         bool   `json:"manualActivity"`
}

// Start returns the start of the activity.
func (a Activity) Start() time.Time { return time.UnixMilli(a.BeginTimestamp).UTC() }

// Token exchanges the API key for an access token, valid for 24 hours.
func (c *Client) Token(ctx context.Context, apiKey string) (string, time.Time, error) {
	body, err := c.do(ctx, http.MethodPost, "/personalapitoken-service/personal-api/personal-api-key/token", nil,
		"ApiKey "+apiKey, 16<<10)
	if err != nil {
		return "", time.Time{}, err
	}
	token := strings.TrimSpace(string(body))
	if token == "" {
		return "", time.Time{}, errors.New("garmin: empty token")
	}
	return token, expiry(token), nil
}

// expiry reads the expiry of the JWT; without one, it assumes the
// documented 24 hours.
func expiry(token string) time.Time {
	fallback := time.Now().Add(24 * time.Hour)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fallback
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(data, &claims) != nil || claims.Exp == 0 {
		return fallback
	}
	return time.Unix(claims.Exp, 0).UTC()
}

// Activities lists the cycling activities that started on or after the
// day since (Garmin compares the local date), oldest first.
func (c *Client) Activities(ctx context.Context, token string, since time.Time, start, limit int) ([]Activity, error) {
	q := url.Values{
		"activityType": {"cycling"}, "startDate": {since.Format(time.DateOnly)}, "sortOrder": {"asc"},
		"start": {strconv.Itoa(start)}, "limit": {strconv.Itoa(limit)},
	}
	body, err := c.do(ctx, http.MethodGet, "/activitylist-service/activities/search/activities", q,
		"Bearer "+token, 16<<20)
	if err != nil {
		return nil, err
	}
	var list []Activity
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("garmin: activity list: %w", err)
	}
	return list, nil
}

// Download returns the original file of the activity as ZIP archive.
func (c *Client) Download(ctx context.Context, token string, id int64) ([]byte, error) {
	return c.do(ctx, http.MethodGet, "/download-service/files/activity/"+strconv.FormatInt(id, 10), nil,
		"Bearer "+token, maxDownload)
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, auth string, limit int64) ([]byte, error) {
	if err := c.pace(ctx); err != nil {
		return nil, err
	}
	u := c.BaseURL + path
	if q != nil {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, ErrUnauthorized
	case res.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("garmin: %s %s: status %d", method, path, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("garmin: %s %s: response too large", method, path)
	}
	return body, nil
}

// pace waits until Wait has passed since the last request.
func (c *Client) pace(ctx context.Context) error {
	c.mu.Lock()
	next := c.last.Add(c.Wait)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	c.last = next
	c.mu.Unlock()
	select {
	case <-time.After(time.Until(next)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
