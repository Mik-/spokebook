package secret_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spokebook/spokebook/internal/app/spokebook/secret"
)

const key = "0123456789abcdef0123456789abcdef"

func TestSealOpen(t *testing.T) {
	box, err := secret.New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed := box.Seal([]byte("api key"), "o1/garmin")
	if bytes.Contains(sealed, []byte("api key")) {
		t.Fatal("sealed data contains the plain text")
	}
	if plain, err := box.Open(sealed, "o1/garmin"); err != nil || string(plain) != "api key" {
		t.Fatalf("open = %q, %v", plain, err)
	}
	if _, err := box.Open(sealed, "o2/garmin"); !errors.Is(err, secret.ErrOpen) {
		t.Fatalf("other purpose: %v", err)
	}
	other, err := secret.New(key + "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(sealed, "o1/garmin"); !errors.Is(err, secret.ErrOpen) {
		t.Fatalf("other key: %v", err)
	}
	if _, err := box.Open([]byte("short"), "o1/garmin"); !errors.Is(err, secret.ErrOpen) {
		t.Fatalf("short: %v", err)
	}
}

func TestShortKey(t *testing.T) {
	if _, err := secret.New("too short"); !errors.Is(err, secret.ErrShortKey) {
		t.Fatalf("err = %v", err)
	}
}
