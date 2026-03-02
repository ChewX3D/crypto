package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ChewX3D/wbcli/internal/app/ports"
	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

type fakeCredentialStore struct {
	backendName string
	credential  *domainauth.Credential
}

func (store *fakeCredentialStore) BackendName() string {
	return store.backendName
}

func (store *fakeCredentialStore) Save(_ context.Context, credential domainauth.Credential) error {
	copied := domainauth.Credential{APIKey: credential.APIKey, APISecret: append([]byte(nil), credential.APISecret...)}
	store.credential = &copied
	return nil
}

func (store *fakeCredentialStore) Load(_ context.Context) (domainauth.Credential, error) {
	if store.credential == nil {
		return domainauth.Credential{}, ports.ErrCredentialNotFound
	}
	return domainauth.Credential{APIKey: store.credential.APIKey, APISecret: append([]byte(nil), store.credential.APISecret...)}, nil
}

func (store *fakeCredentialStore) Exists(_ context.Context) (bool, error) {
	return store.credential != nil, nil
}

func (store *fakeCredentialStore) Delete(_ context.Context) error {
	store.credential = nil
	return nil
}

type fakeSessionStore struct {
	session *ports.SessionMetadata
}

func (store *fakeSessionStore) SaveSession(_ context.Context, session ports.SessionMetadata) error {
	copied := session
	store.session = &copied
	return nil
}

func (store *fakeSessionStore) GetSession(_ context.Context) (ports.SessionMetadata, bool, error) {
	if store.session == nil {
		return ports.SessionMetadata{}, false, nil
	}
	return *store.session, true, nil
}

func (store *fakeSessionStore) ClearSession(_ context.Context) error {
	store.session = nil
	return nil
}

type fixedClock struct {
	now time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.now
}

type fakeCredentialVerifier struct {
	err        error
	callCount  int
	lastAPIKey string
	endpoint   string
	hedgeMode  bool
}

func (verifier *fakeCredentialVerifier) Verify(_ context.Context, credential domainauth.Credential) (ports.CredentialVerificationResult, error) {
	verifier.callCount++
	verifier.lastAPIKey = credential.APIKey
	if verifier.err != nil {
		return ports.CredentialVerificationResult{}, verifier.err
	}

	return ports.CredentialVerificationResult{
		Endpoint:  verifier.endpoint,
		HedgeMode: verifier.hedgeMode,
	}, nil
}

func TestLoginServiceExecuteSuccess(t *testing.T) {
	credentialStore := &fakeCredentialStore{backendName: "os-keychain"}
	sessionStore := &fakeSessionStore{}
	credentialVerifier := &fakeCredentialVerifier{
		endpoint:  "/api/v4/collateral-account/hedge-mode",
		hedgeMode: true,
	}
	service := NewLoginService(
		credentialStore,
		sessionStore,
		fixedClock{now: time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC)},
		credentialVerifier,
	)

	secret := []byte("secret-1")
	result, err := service.Execute(context.Background(), LoginRequest{
		APIKey:    "api-key-1",
		APISecret: secret,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Backend != "os-keychain" {
		t.Fatalf("expected backend os-keychain, got %q", result.Backend)
	}
	if result.APIKeyHint == "" {
		t.Fatalf("expected api key hint")
	}
	if credentialVerifier.callCount != 1 {
		t.Fatalf("expected exactly one credential verification call, got %d", credentialVerifier.callCount)
	}
	if credentialVerifier.lastAPIKey != "api-key-1" {
		t.Fatalf("expected verifier api key api-key-1, got %q", credentialVerifier.lastAPIKey)
	}
	if credentialStore.credential == nil {
		t.Fatalf("expected credential to be stored")
	}
	if got := string(credentialStore.credential.APISecret); got != "secret-1" {
		t.Fatalf("expected stored secret secret-1, got %q", got)
	}
	if sessionStore.session == nil {
		t.Fatalf("expected session metadata to be stored")
	}
	if sessionStore.session.Backend != "os-keychain" {
		t.Fatalf("expected session backend os-keychain, got %q", sessionStore.session.Backend)
	}
	if sessionStore.session.HedgeMode == nil || !*sessionStore.session.HedgeMode {
		t.Fatalf("expected hedge mode true in stored session, got %#v", sessionStore.session.HedgeMode)
	}
	if string(secret) != "\x00\x00\x00\x00\x00\x00\x00\x00" {
		t.Fatalf("expected request secret bytes to be wiped, got %q", string(secret))
	}
}

func TestLoginServiceExecuteOverwritesExistingCredential(t *testing.T) {
	credentialStore := &fakeCredentialStore{
		backendName: "os-keychain",
		credential:  &domainauth.Credential{APIKey: "old", APISecret: []byte("old-secret")},
	}
	sessionStore := &fakeSessionStore{}
	credentialVerifier := &fakeCredentialVerifier{
		endpoint:  "/api/v4/collateral-account/hedge-mode",
		hedgeMode: false,
	}
	service := NewLoginService(credentialStore, sessionStore, fixedClock{now: time.Now()}, credentialVerifier)

	result, err := service.Execute(context.Background(), LoginRequest{
		APIKey:    "new",
		APISecret: []byte("new-secret"),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.APIKeyHint == "" {
		t.Fatalf("expected api key hint")
	}
	if credentialStore.credential == nil {
		t.Fatalf("expected credential to be stored")
	}
	if credentialStore.credential.APIKey != "new" {
		t.Fatalf("expected API key to be overwritten, got %q", credentialStore.credential.APIKey)
	}
}

func TestLoginServiceExecuteFailsWhenProbeFails(t *testing.T) {
	credentialStore := &fakeCredentialStore{backendName: "os-keychain"}
	sessionStore := &fakeSessionStore{}
	credentialVerifier := &fakeCredentialVerifier{err: ports.ErrCredentialVerifyUnauthorized}
	service := NewLoginService(credentialStore, sessionStore, fixedClock{now: time.Now()}, credentialVerifier)

	_, err := service.Execute(context.Background(), LoginRequest{
		APIKey:    "bad",
		APISecret: []byte("secret"),
	})
	if err == nil {
		t.Fatal("expected probe error")
	}
	if !errors.Is(err, ports.ErrCredentialVerifyUnauthorized) {
		t.Fatalf("expected unauthorized probe error, got %v", err)
	}
	if credentialStore.credential != nil {
		t.Fatalf("expected credential to stay unsaved on probe failure")
	}
	if sessionStore.session != nil {
		t.Fatalf("expected session metadata to stay unsaved on probe failure")
	}
}
