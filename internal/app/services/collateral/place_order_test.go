package collateral

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ChewX3D/wbcli/internal/app/ports"
	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

type fakeClock struct {
	now time.Time
}

func (clock fakeClock) Now() time.Time {
	return clock.now
}

type fakeCredentialStore struct {
	loadCredential domainauth.Credential
	loadErr        error
}

func (store *fakeCredentialStore) BackendName() string {
	return "os-keychain"
}

func (store *fakeCredentialStore) Save(context.Context, domainauth.Credential) error {
	return nil
}

func (store *fakeCredentialStore) Load(context.Context) (domainauth.Credential, error) {
	if store.loadErr != nil {
		return domainauth.Credential{}, store.loadErr
	}

	return store.loadCredential, nil
}

func (store *fakeCredentialStore) Exists(context.Context) (bool, error) {
	return true, nil
}

func (store *fakeCredentialStore) Delete(context.Context) error {
	return nil
}

type fakeSessionStore struct {
	session *ports.SessionMetadata
}

func (store *fakeSessionStore) SaveSession(_ context.Context, session ports.SessionMetadata) error {
	copied := session
	if session.HedgeMode != nil {
		value := *session.HedgeMode
		copied.HedgeMode = &value
	}
	store.session = &copied
	return nil
}

func (store *fakeSessionStore) GetSession(_ context.Context) (ports.SessionMetadata, bool, error) {
	if store.session == nil {
		return ports.SessionMetadata{}, false, nil
	}
	copied := *store.session
	if store.session.HedgeMode != nil {
		value := *store.session.HedgeMode
		copied.HedgeMode = &value
	}
	return copied, true, nil
}

func (store *fakeSessionStore) ClearSession(context.Context) error {
	store.session = nil
	return nil
}

type fakeOrderExecutor struct {
	lastCredential    domainauth.Credential
	requests          []ports.CollateralLimitOrderRequest
	placeErrors       []error
	getHedgeModeValue bool
	getHedgeModeErr   error
	getHedgeModeCalls int
}

func (executor *fakeOrderExecutor) GetCollateralAccountHedgeMode(
	_ context.Context,
	credential domainauth.Credential,
) (bool, error) {
	executor.getHedgeModeCalls++
	executor.lastCredential = domainauth.Credential{
		APIKey:    credential.APIKey,
		APISecret: append([]byte(nil), credential.APISecret...),
	}
	if executor.getHedgeModeErr != nil {
		return false, executor.getHedgeModeErr
	}

	return executor.getHedgeModeValue, nil
}

func (executor *fakeOrderExecutor) PlaceCollateralLimitOrder(
	_ context.Context,
	credential domainauth.Credential,
	request ports.CollateralLimitOrderRequest,
) (json.RawMessage, error) {
	executor.lastCredential = domainauth.Credential{
		APIKey:    credential.APIKey,
		APISecret: append([]byte(nil), credential.APISecret...),
	}
	executor.requests = append(executor.requests, request)

	if len(executor.placeErrors) > 0 {
		err := executor.placeErrors[0]
		executor.placeErrors = executor.placeErrors[1:]
		if err != nil {
			return nil, err
		}
	}

	return json.RawMessage(`{"status":"ok"}`), nil
}

func boolPtr(value bool) *bool {
	allocated := value
	return &allocated
}

func TestPlaceOrderServiceExecuteUsesSessionHedgeModeTrue(t *testing.T) {
	credentialStore := &fakeCredentialStore{
		loadCredential: domainauth.Credential{
			APIKey:    "public-key",
			APISecret: []byte("secret-key"),
		},
	}
	sessionStore := &fakeSessionStore{session: &ports.SessionMetadata{HedgeMode: boolPtr(true)}}
	orderExecutor := &fakeOrderExecutor{}
	service := NewPlaceOrderService(
		credentialStore,
		sessionStore,
		orderExecutor,
		fakeClock{now: time.Date(2026, 3, 2, 10, 0, 0, 123, time.UTC)},
	)

	result, err := service.Execute(context.Background(), PlaceOrderRequest{
		Market:        "BTC_PERP",
		Side:          "long",
		Amount:        "0.01",
		Price:         "50000",
		ClientOrderID: "client-001",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Mode != "single" {
		t.Fatalf("expected mode single, got %q", result.Mode)
	}
	if orderExecutor.getHedgeModeCalls != 0 {
		t.Fatalf("expected no hedge mode fetch when cached, got %d", orderExecutor.getHedgeModeCalls)
	}
	if len(orderExecutor.requests) != 1 {
		t.Fatalf("expected one place request, got %d", len(orderExecutor.requests))
	}
	if got := orderExecutor.requests[0].Side; got != "buy" {
		t.Fatalf("expected buy side, got %q", got)
	}
	if got := orderExecutor.requests[0].PositionSide; got != "long" {
		t.Fatalf("expected long position side, got %q", got)
	}
	if !orderExecutor.requests[0].PostOnly {
		t.Fatalf("expected postOnly=true")
	}
}

func TestPlaceOrderServiceExecuteUsesSessionHedgeModeFalse(t *testing.T) {
	credentialStore := &fakeCredentialStore{
		loadCredential: domainauth.Credential{APIKey: "public-key", APISecret: []byte("secret-key")},
	}
	sessionStore := &fakeSessionStore{session: &ports.SessionMetadata{HedgeMode: boolPtr(false)}}
	orderExecutor := &fakeOrderExecutor{}
	service := NewPlaceOrderService(credentialStore, sessionStore, orderExecutor, fakeClock{now: time.Now()})

	_, err := service.Execute(context.Background(), PlaceOrderRequest{
		Market: "BTC_PERP",
		Side:   "long",
		Amount: "0.01",
		Price:  "50000",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(orderExecutor.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(orderExecutor.requests))
	}
	if got := orderExecutor.requests[0].Side; got != "buy" {
		t.Fatalf("expected buy side, got %q", got)
	}
	if got := orderExecutor.requests[0].PositionSide; got != "" {
		t.Fatalf("expected empty position side in one-way mode, got %q", got)
	}
}

func TestPlaceOrderServiceExecuteRefreshesMissingHedgeMode(t *testing.T) {
	credentialStore := &fakeCredentialStore{
		loadCredential: domainauth.Credential{APIKey: "public-key", APISecret: []byte("secret-key")},
	}
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	sessionStore := &fakeSessionStore{session: &ports.SessionMetadata{
		Backend:    "os-keychain",
		APIKeyHint: "ab***yz",
		CreatedAt:  now,
		UpdatedAt:  now,
	}}
	orderExecutor := &fakeOrderExecutor{getHedgeModeValue: true}
	service := NewPlaceOrderService(credentialStore, sessionStore, orderExecutor, fakeClock{now: now.Add(time.Minute)})

	_, err := service.Execute(context.Background(), PlaceOrderRequest{
		Market: "BTC_PERP",
		Side:   "short",
		Amount: "0.01",
		Price:  "50000",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if orderExecutor.getHedgeModeCalls != 1 {
		t.Fatalf("expected one hedge mode fetch, got %d", orderExecutor.getHedgeModeCalls)
	}
	if sessionStore.session == nil || sessionStore.session.HedgeMode == nil || !*sessionStore.session.HedgeMode {
		t.Fatalf("expected persisted hedge_mode=true, got %#v", sessionStore.session)
	}
	if got := orderExecutor.requests[0].PositionSide; got != "short" {
		t.Fatalf("expected short position side in hedge mode, got %q", got)
	}
}

func TestPlaceOrderServiceExecuteRetriesOnHedgeModeMismatch(t *testing.T) {
	credentialStore := &fakeCredentialStore{
		loadCredential: domainauth.Credential{APIKey: "public-key", APISecret: []byte("secret-key")},
	}
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	sessionStore := &fakeSessionStore{session: &ports.SessionMetadata{
		Backend:    "os-keychain",
		APIKeyHint: "ab***yz",
		HedgeMode:  boolPtr(false),
		CreatedAt:  now,
		UpdatedAt:  now,
	}}
	orderExecutor := &fakeOrderExecutor{
		placeErrors:       []error{errors.New("whitebit api business rule error: status 422: hedgeMode: Order's position side does not match user's setting"), nil},
		getHedgeModeValue: true,
	}
	service := NewPlaceOrderService(credentialStore, sessionStore, orderExecutor, fakeClock{now: now.Add(2 * time.Minute)})

	_, err := service.Execute(context.Background(), PlaceOrderRequest{
		Market: "BTC_PERP",
		Side:   "long",
		Amount: "0.01",
		Price:  "50000",
	})
	if err != nil {
		t.Fatalf("expected no error after retry, got %v", err)
	}
	if orderExecutor.getHedgeModeCalls != 1 {
		t.Fatalf("expected one hedge mode refresh, got %d", orderExecutor.getHedgeModeCalls)
	}
	if len(orderExecutor.requests) != 2 {
		t.Fatalf("expected two place attempts, got %d", len(orderExecutor.requests))
	}
	if orderExecutor.requests[0].PositionSide != "" {
		t.Fatalf("expected first attempt without position side, got %q", orderExecutor.requests[0].PositionSide)
	}
	if orderExecutor.requests[1].PositionSide != "long" {
		t.Fatalf("expected retry with long position side, got %q", orderExecutor.requests[1].PositionSide)
	}
}

func TestPlaceOrderServiceExecuteCredentialLoadFailure(t *testing.T) {
	credentialStore := &fakeCredentialStore{loadErr: ports.ErrCredentialNotFound}
	sessionStore := &fakeSessionStore{}
	orderExecutor := &fakeOrderExecutor{}
	service := NewPlaceOrderService(credentialStore, sessionStore, orderExecutor, fakeClock{now: time.Now()})

	_, err := service.Execute(context.Background(), PlaceOrderRequest{
		Market: "BTC_PERP",
		Side:   "buy",
		Amount: "0.01",
		Price:  "50000",
	})
	if !errors.Is(err, ports.ErrCredentialNotFound) {
		t.Fatalf("expected credential not found, got %v", err)
	}
}

func TestPlaceOrderServiceExecuteExecutorFailure(t *testing.T) {
	credentialStore := &fakeCredentialStore{
		loadCredential: domainauth.Credential{APIKey: "public-key", APISecret: []byte("secret-key")},
	}
	sessionStore := &fakeSessionStore{session: &ports.SessionMetadata{HedgeMode: boolPtr(false)}}
	orderExecutor := &fakeOrderExecutor{placeErrors: []error{errors.New("exchange rejected request")}}
	service := NewPlaceOrderService(credentialStore, sessionStore, orderExecutor, fakeClock{now: time.Now()})

	_, err := service.Execute(context.Background(), PlaceOrderRequest{
		Market: "BTC_PERP",
		Side:   "buy",
		Amount: "0.01",
		Price:  "50000",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("expected non-empty error")
	}
}
