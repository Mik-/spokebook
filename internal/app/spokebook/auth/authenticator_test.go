package auth_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
)

// authenticator is a software passkey for tests: one ES256 key with
// attestation "none", user presence and verification always given.
type authenticator struct {
	t      *testing.T
	origin string
	key    *ecdsa.PrivateKey
	credID []byte
	userID []byte
	count  uint32
}

func newAuthenticator(t *testing.T, origin string) *authenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &authenticator{t: t, origin: origin, key: key, credID: []byte(rand.Text())}
}

var b64 = base64.RawURLEncoding

// options holds the fields of the creation and request options that the
// authenticator needs.
type options struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RPID      string `json:"rpId"`
		RP        struct {
			ID string `json:"id"`
		} `json:"rp"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"publicKey"`
}

func (a *authenticator) parse(raw []byte) options {
	a.t.Helper()
	var o options
	if err := json.Unmarshal(raw, &o); err != nil {
		a.t.Fatal(err)
	}
	return o
}

func (a *authenticator) clientData(typ, challenge string) []byte {
	raw, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": a.origin, "crossOrigin": false})
	return raw
}

// authData builds the authenticator data; flags UP and UV, plus AT with
// attested credential data.
func (a *authenticator) authData(rpID string, attested []byte) []byte {
	hash := sha256.Sum256([]byte(rpID))
	flags := byte(0x01 | 0x04)
	if attested != nil {
		flags |= 0x40
	}
	a.count++
	out := append(hash[:], flags)
	out = binary.BigEndian.AppendUint32(out, a.count)
	return append(out, attested...)
}

// create answers navigator.credentials.create.
func (a *authenticator) create(raw []byte) []byte {
	a.t.Helper()
	o := a.parse(raw)
	userID, err := b64.DecodeString(o.PublicKey.User.ID)
	if err != nil {
		a.t.Fatal(err)
	}
	a.userID = userID
	cose, err := webauthncbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1, -2: a.key.X.FillBytes(make([]byte, 32)), -3: a.key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		a.t.Fatal(err)
	}
	attested := make([]byte, 16) // AAGUID
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.credID)))
	attested = append(append(attested, a.credID...), cose...)
	att, err := webauthncbor.Marshal(map[string]any{
		"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(o.PublicKey.RP.ID, attested),
	})
	if err != nil {
		a.t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(a.clientData("webauthn.create", o.PublicKey.Challenge)),
			"attestationObject": b64.EncodeToString(att),
		},
	})
	return body
}

// get answers navigator.credentials.get.
func (a *authenticator) get(raw []byte) []byte {
	a.t.Helper()
	o := a.parse(raw)
	cd := a.clientData("webauthn.get", o.PublicKey.Challenge)
	ad := a.authData(o.PublicKey.RPID, nil)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		a.t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(cd),
			"authenticatorData": b64.EncodeToString(ad),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(a.userID),
		},
	})
	return body
}
