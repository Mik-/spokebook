package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// MaxPasskeyNameLength is the maximum length of a passkey's name.
const MaxPasskeyNameLength = 64

// ceremonyTimeout is how long the browser has to answer a challenge.
const ceremonyTimeout = 5 * time.Minute

// maxCeremonies bounds the open challenges kept in memory.
const maxCeremonies = 1000

var (
	// ErrPasskey means the browser's answer was invalid, expired or for an
	// unknown passkey.
	ErrPasskey = errors.New("auth: passkey failed")
	// ErrBusy means too many open challenges.
	ErrBusy = errors.New("auth: too many passkey requests")
)

// Passkeys runs the WebAuthn ceremonies to register passkeys and log in
// with them. Logins use discoverable credentials, so no name is needed.
type Passkeys struct {
	wa *webauthn.WebAuthn
	st store.Store

	mu sync.Mutex
	// open challenges by their base64url value; each is used once
	ceremonies map[string]ceremony
}

type ceremony struct {
	data webauthn.SessionData
	// registration only
	userID string
	name   string
}

// NewPasskeys configures WebAuthn for the app's public URL, e.g.
// "https://bikes.example.com"; its host is the relying party ID.
func NewPasskeys(st store.Store, baseURL string) (*Passkeys, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("auth: invalid base URL %q", baseURL)
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "Spokebook",
		RPOrigins:     []string{u.Scheme + "://" + u.Host},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout},
		},
	})
	if err != nil {
		return nil, err
	}
	return &Passkeys{wa: wa, st: st, ceremonies: make(map[string]ceremony)}, nil
}

// webauthnUser adapts a user and their passkeys to webauthn.User. The user
// handle is the user ID.
type webauthnUser struct {
	store.User
	creds []webauthn.Credential
}

func (u webauthnUser) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u webauthnUser) WebAuthnName() string                       { return u.Name }
func (u webauthnUser) WebAuthnDisplayName() string                { return u.Name }
func (u webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (p *Passkeys) loadUser(ctx context.Context, u store.User) (webauthnUser, []store.Passkey, error) {
	keys, err := p.st.Users().ListPasskeys(ctx, u.ID)
	if err != nil {
		return webauthnUser{}, nil, err
	}
	wu := webauthnUser{User: u}
	for _, k := range keys {
		var c webauthn.Credential
		if err := json.Unmarshal(k.Credential, &c); err != nil {
			return webauthnUser{}, nil, fmt.Errorf("passkey %s: %w", k.ID, err)
		}
		wu.creds = append(wu.creds, c)
	}
	return wu, keys, nil
}

// remember stores an open challenge.
func (p *Passkeys) remember(c ceremony) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.ceremonies) >= maxCeremonies {
		now := time.Now()
		for k, v := range p.ceremonies {
			if now.After(v.data.Expires) {
				delete(p.ceremonies, k)
			}
		}
		if len(p.ceremonies) >= maxCeremonies {
			return ErrBusy
		}
	}
	p.ceremonies[c.data.Challenge] = c
	return nil
}

// take removes and returns the open challenge.
func (p *Passkeys) take(challenge string) (ceremony, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.ceremonies[challenge]
	delete(p.ceremonies, challenge)
	return c, ok
}

// ValidatePasskeyName trims the name and checks it.
func ValidatePasskeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrNameRequired
	case utf8.RuneCountInString(name) > MaxPasskeyNameLength:
		return "", ErrNameTooLong
	}
	return name, nil
}

// BeginRegistration returns the options for navigator.credentials.create
// as JSON. Passkeys the user has already are excluded.
func (p *Passkeys) BeginRegistration(ctx context.Context, u store.User, name string) ([]byte, error) {
	name, err := ValidatePasskeyName(name)
	if err != nil {
		return nil, err
	}
	wu, _, err := p.loadUser(ctx, u)
	if err != nil {
		return nil, err
	}
	opts, data, err := p.wa.BeginRegistration(wu, webauthn.WithExclusions(webauthn.Credentials(wu.creds).CredentialDescriptors()))
	if err != nil {
		return nil, err
	}
	if err := p.remember(ceremony{data: *data, userID: u.ID, name: name}); err != nil {
		return nil, err
	}
	return json.Marshal(opts)
}

// FinishRegistration checks the browser's answer and stores the passkey.
func (p *Passkeys) FinishRegistration(ctx context.Context, u store.User, body io.Reader) (store.Passkey, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBody(body)
	if err != nil {
		return store.Passkey{}, fmt.Errorf("%w: %w", ErrPasskey, err)
	}
	c, ok := p.take(parsed.Response.CollectedClientData.Challenge)
	if !ok || c.userID != u.ID {
		return store.Passkey{}, ErrPasskey
	}
	wu, _, err := p.loadUser(ctx, u)
	if err != nil {
		return store.Passkey{}, err
	}
	cred, err := p.wa.CreateCredential(wu, c.data, parsed)
	if err != nil {
		return store.Passkey{}, fmt.Errorf("%w: %w", ErrPasskey, err)
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return store.Passkey{}, err
	}
	k := store.Passkey{
		ID: uuid.Must(uuid.NewV7()).String(), UserID: u.ID, CredentialID: cred.ID, Name: c.name,
		Credential: raw, CreatedAt: time.Now().UTC(),
	}
	if err := p.st.Users().CreatePasskey(ctx, k); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return store.Passkey{}, ErrPasskey
		}
		return store.Passkey{}, err
	}
	return k, nil
}

// BeginLogin returns the options for navigator.credentials.get as JSON.
func (p *Passkeys) BeginLogin() ([]byte, error) {
	opts, data, err := p.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, err
	}
	if err := p.remember(ceremony{data: *data}); err != nil {
		return nil, err
	}
	return json.Marshal(opts)
}

// FinishLogin checks the browser's answer and returns the user. It updates
// the passkey's sign count and resets the user's failed logins.
func (p *Passkeys) FinishLogin(ctx context.Context, body io.Reader) (store.User, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBody(body)
	if err != nil {
		return store.User{}, fmt.Errorf("%w: %w", ErrPasskey, err)
	}
	c, ok := p.take(parsed.Response.CollectedClientData.Challenge)
	if !ok || c.userID != "" {
		return store.User{}, ErrPasskey
	}
	var user store.User
	var keys []store.Passkey
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		k, err := p.st.Users().GetPasskeyByCredentialID(ctx, rawID)
		if err != nil {
			return nil, err
		}
		if k.UserID != string(userHandle) {
			return nil, ErrPasskey
		}
		if user, err = p.st.Users().GetUser(ctx, k.UserID); err != nil {
			return nil, err
		}
		var wu webauthnUser
		wu, keys, err = p.loadUser(ctx, user)
		return wu, err
	}
	cred, err := p.wa.ValidateDiscoverableLogin(handler, c.data, parsed)
	if err != nil {
		return store.User{}, fmt.Errorf("%w: %w", ErrPasskey, err)
	}
	if cred.Authenticator.CloneWarning {
		return store.User{}, fmt.Errorf("%w: sign count went backwards", ErrPasskey)
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return store.User{}, err
	}
	for _, k := range keys {
		if string(k.CredentialID) == string(cred.ID) {
			if err := p.st.Users().UpdatePasskeyCredential(ctx, user.ID, k.ID, raw, time.Now().UTC()); err != nil {
				return store.User{}, err
			}
		}
	}
	return user, resetFailures(ctx, p.st, user)
}
