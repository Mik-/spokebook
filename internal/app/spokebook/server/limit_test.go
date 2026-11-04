package server

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted, err := ParseTrustedProxies("192.168.1.10, 172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, remote, forwarded, want string
	}{
		{"direct", "203.0.113.5:1234", "", "203.0.113.5"},
		{"untrusted sender is not believed", "203.0.113.5:1234", "198.51.100.1", "203.0.113.5"},
		{"proxy", "192.168.1.10:1234", "198.51.100.1", "198.51.100.1"},
		{"forged entries left of the proxy are ignored", "192.168.1.10:1234", "10.0.0.1, 198.51.100.1", "198.51.100.1"},
		{"proxy chain", "172.17.0.1:1234", "198.51.100.1, 192.168.1.10", "198.51.100.1"},
		{"proxy without header", "192.168.1.10:1234", "", "192.168.1.10"},
		{"garbage", "192.168.1.10:1234", "not an ip", "not an ip"},
		{"mapped IPv4", "[::ffff:192.168.1.10]:1234", "198.51.100.1", "198.51.100.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/login", nil)
			r.RemoteAddr = tt.remote
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if got := clientIP(r, trusted); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "192.168.1.10:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	if got := clientIP(r, nil); got != "192.168.1.10" {
		t.Errorf("without trusted proxies: got %q", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	if p, err := ParseTrustedProxies(""); err != nil || len(p) != 0 {
		t.Errorf("empty: %v %v", p, err)
	}
	if p, err := ParseTrustedProxies("10.0.0.1/8, ::1"); err != nil || len(p) != 2 || p[0].String() != "10.0.0.0/8" {
		t.Errorf("got %v %v", p, err)
	}
	for _, bad := range []string{"proxy.local", "10.0.0.0/33"} {
		if _, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}
