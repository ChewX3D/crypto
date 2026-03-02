package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ChewX3D/wbcli/internal/adapters/whitebit"
	appcontainer "github.com/ChewX3D/wbcli/internal/app/application"
	"github.com/ChewX3D/wbcli/internal/app/ports"
	authservice "github.com/ChewX3D/wbcli/internal/app/services/auth"
	collateralservice "github.com/ChewX3D/wbcli/internal/app/services/collateral"
	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

func executeCommandWithInput(input string, args ...string) (string, string, error) {
	command := NewRootCmdForTest()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	command.SetOut(stdout)
	command.SetErr(stderr)
	if input != "" {
		command.SetIn(strings.NewReader(input))
	}
	command.SetArgs(args)

	err := command.Execute()

	return stdout.String(), stderr.String(), err
}

func executeCommand(args ...string) (string, string, error) {
	return executeCommandWithInput("", args...)
}

func assertUnknownAuthSubcommand(t *testing.T, subcommand string) {
	t.Helper()

	_, _, err := executeCommand("auth", subcommand)
	if err == nil {
		t.Fatalf("expected unknown command error for %q", subcommand)
	}
	if !strings.Contains(err.Error(), "unknown command \""+subcommand+"\"") {
		t.Fatalf("unexpected error for %q: %v", subcommand, err)
	}
}

func TestRootHelpShowsMainGroups(t *testing.T) {
	stdout, stderr, err := executeCommand("--help")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "auth") || !strings.Contains(stdout, "collateral") {
		t.Fatalf("expected auth and collateral in help output, got: %q", stdout)
	}

	if stderr != "" {
		t.Fatalf("expected empty stderr, got: %q", stderr)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	_, _, err := executeCommand("unknown")
	if err == nil {
		t.Fatal("expected an error for unknown command")
	}
}

func TestLegacyAuthSetCommandRemoved(t *testing.T) {
	assertUnknownAuthSubcommand(t, "set")
}

func TestLegacyAuthUseCommandRemoved(t *testing.T) {
	assertUnknownAuthSubcommand(t, "use")
}

func TestLegacyAuthListCommandRemoved(t *testing.T) {
	assertUnknownAuthSubcommand(t, "list")
}

func TestLegacyAuthCurrentCommandRemoved(t *testing.T) {
	assertUnknownAuthSubcommand(t, "current")
}

func TestLegacyAuthTestCommandRemoved(t *testing.T) {
	assertUnknownAuthSubcommand(t, "test")
}

func TestAuthLoginRejectsInvalidStdinContract(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, _, err := executeCommandWithInput("only-key\n", "auth", "login")
	if err == nil {
		t.Fatal("expected stdin parsing error")
	}
	if !strings.Contains(err.Error(), "exactly two non-empty lines") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthStatusWorksWhenLoggedOut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout, _, err := executeCommand("auth", "status")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "logged_in=false") {
		t.Fatalf("expected logged_out status, got: %q", stdout)
	}
}

func TestAuthLogoutWorksWhenLoggedIn(t *testing.T) {
	credentialStore := &testCredentialStore{
		backendName: "os-keychain",
		credential:  &domainauth.Credential{APIKey: "api-key-1", APISecret: []byte("secret-1")},
	}
	updatedAt := time.Date(2026, 2, 28, 15, 4, 5, 0, time.UTC)
	sessionStore := &testSessionStore{
		session: &ports.SessionMetadata{
			Backend:    "os-keychain",
			APIKeyHint: "ab***yz",
			CreatedAt:  updatedAt,
			UpdatedAt:  updatedAt,
		},
	}

	withApplicationFactory(t, testApplication(credentialStore, sessionStore, nil))

	stdout, stderr, err := executeCommand("auth", "logout")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "logged_out=true") {
		t.Fatalf("expected logged_out=true output, got: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got: %q", stderr)
	}
	if credentialStore.credential != nil {
		t.Fatalf("expected credential to be deleted")
	}
	if sessionStore.session != nil {
		t.Fatalf("expected session metadata to be cleared")
	}
}

func TestAuthLogoutIdempotentWhenLoggedOut(t *testing.T) {
	credentialStore := &testCredentialStore{backendName: "os-keychain"}
	sessionStore := &testSessionStore{}

	withApplicationFactory(t, testApplication(credentialStore, sessionStore, nil))

	stdout, _, err := executeCommand("auth", "logout")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "logged_out=true") {
		t.Fatalf("expected logged_out=true output, got: %q", stdout)
	}
}

func TestAuthStatusWorksWhenLoggedIn(t *testing.T) {
	updatedAt := time.Date(2026, 2, 28, 15, 4, 5, 0, time.UTC)
	credentialStore := &testCredentialStore{backendName: "os-keychain"}
	sessionStore := &testSessionStore{
		session: &ports.SessionMetadata{
			Backend:    "os-keychain",
			APIKeyHint: "ab***yz",
			CreatedAt:  updatedAt,
			UpdatedAt:  updatedAt,
		},
	}

	withApplicationFactory(t, testApplication(credentialStore, sessionStore, nil))

	stdout, _, err := executeCommand("auth", "status")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "logged_in=true") {
		t.Fatalf("expected logged_in=true output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "backend=os-keychain") {
		t.Fatalf("expected backend in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "api_key=ab***yz") {
		t.Fatalf("expected api key hint in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "updated_at=2026-02-28T15:04:05Z") {
		t.Fatalf("expected updated_at in output, got: %q", stdout)
	}
}

func TestAuthLoginUnavailableStoreReturnsActionableError(t *testing.T) {
	withApplicationFactoryError(t, ports.ErrSecretStoreUnavailable)

	_, _, err := executeCommandWithInput("api-key-1\nsecret-1\n", "auth", "login")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "os-keychain backend is unavailable on this system; install/unlock keychain backend and retry") {
		t.Fatalf("expected actionable unavailable error, got %v", err)
	}
}

func TestAuthLoginUnauthorizedReturnsActionableError(t *testing.T) {
	credentialStore := &testCredentialStore{
		backendName: "os-keychain",
	}
	sessionStore := &testSessionStore{}
	credentialVerifier := &testCredentialVerifier{
		err: ports.NewCredentialVerificationError(
			ports.CredentialVerificationInvalidCredentials,
			"/api/v4/collateral-account/hedge-mode",
			"status 401: invalid signature",
		),
	}

	withApplicationFactory(t, testApplication(credentialStore, sessionStore, credentialVerifier))

	_, _, err := executeCommandWithInput("bad-key\nbad-secret\n", "auth", "login")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "credentials are invalid") {
		t.Fatalf("expected actionable auth-failed error, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("expected underlying reason in message, got %v", err)
	}
}

func TestAuthLoginUnauthorizedActionDeniedSuggestsEndpointAccess(t *testing.T) {
	credentialStore := &testCredentialStore{
		backendName: "os-keychain",
	}
	sessionStore := &testSessionStore{}
	credentialVerifier := &testCredentialVerifier{
		err: ports.NewCredentialVerificationError(
			ports.CredentialVerificationInsufficientAccess,
			"/api/v4/collateral-account/hedge-mode",
			"status 401: This API Key is not authorized to perform this action.",
		),
	}

	withApplicationFactory(t, testApplication(credentialStore, sessionStore, credentialVerifier))

	_, _, err := executeCommandWithInput("bad-key\nbad-secret\n", "auth", "login")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "enable access to endpoint /api/v4/collateral-account/hedge-mode") {
		t.Fatalf("expected endpoint access instruction, got %v", err)
	}
}

func TestAuthLoginInsufficientAccessIncludesProvidedEndpoint(t *testing.T) {
	credentialStore := &testCredentialStore{backendName: "os-keychain"}
	sessionStore := &testSessionStore{}
	credentialVerifier := &testCredentialVerifier{
		err: ports.NewCredentialVerificationError(
			ports.CredentialVerificationInsufficientAccess,
			"/api/v4/order/collateral/limit",
			"status 401: not authorized",
		),
	}

	withApplicationFactory(t, testApplication(credentialStore, sessionStore, credentialVerifier))

	_, _, err := executeCommandWithInput("bad-key\nbad-secret\n", "auth", "login")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "enable access to endpoint /api/v4/order/collateral/limit") {
		t.Fatalf("expected dynamic endpoint in message, got %v", err)
	}
}

func TestAuthLogoutPermissionDeniedReturnsActionableError(t *testing.T) {
	credentialStore := &testCredentialStore{
		backendName: "os-keychain",
		deleteErr:   ports.ErrSecretStorePermissionDenied,
	}
	sessionStore := &testSessionStore{}

	withApplicationFactory(t, testApplication(credentialStore, sessionStore, nil))

	_, _, err := executeCommand("auth", "logout")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "os-keychain access denied; keychain is locked or access is restricted") {
		t.Fatalf("expected actionable permission-denied error, got %v", err)
	}
}

func TestLegacyOrderRootCommandRemoved(t *testing.T) {
	_, _, err := executeCommand("order")
	if err == nil {
		t.Fatalf("expected unknown command error for removed root order command")
	}
	if !strings.Contains(err.Error(), "unknown command \"order\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollateralOrderPlaceSuccessTableOutput(t *testing.T) {
	credentialStore := &testCredentialStore{backendName: "os-keychain"}
	sessionStore := &testSessionStore{}
	placeUseCase := &testCollateralUseCases{
		result: collateralservice.PlaceOrderResult{
			RequestID:       "order-123",
			Mode:            "single",
			OrdersPlanned:   1,
			OrdersSubmitted: 1,
			OrdersFailed:    0,
			Errors:          []string{},
		},
	}

	application := testApplication(credentialStore, sessionStore, nil)
	application.Collateral = placeUseCase
	withApplicationFactory(t, application)

	stdout, _, err := executeCommand(
		"collateral", "order", "place",
		"--market", "BTC_PERP",
		"--side", "long",
		"--amount", "0.01",
		"--price", "50000",
		"--client-order-id", "bot-001",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "request_id=order-123") {
		t.Fatalf("expected request_id in table output, got: %q", stdout)
	}
	if placeUseCase.lastRequest.Side != "long" {
		t.Fatalf("expected canonical side passthrough long, got %q", placeUseCase.lastRequest.Side)
	}
	if placeUseCase.lastRequest.ClientOrderID != "bot-001" {
		t.Fatalf("expected client order id pass-through, got %q", placeUseCase.lastRequest.ClientOrderID)
	}
}

func TestCollateralOrderPlaceSuccessJSONOutput(t *testing.T) {
	credentialStore := &testCredentialStore{backendName: "os-keychain"}
	sessionStore := &testSessionStore{}
	placeUseCase := &testCollateralUseCases{
		result: collateralservice.PlaceOrderResult{
			RequestID:       "order-456",
			Mode:            "single",
			OrdersPlanned:   1,
			OrdersSubmitted: 1,
			OrdersFailed:    0,
			Errors:          []string{},
		},
	}

	application := testApplication(credentialStore, sessionStore, nil)
	application.Collateral = placeUseCase
	withApplicationFactory(t, application)

	stdout, _, err := executeCommand(
		"collateral", "order", "place",
		"--market", "BTC_PERP",
		"--side", "sell",
		"--amount", "0.01",
		"--price", "50000",
		"--output", "json",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("expected valid json output, got error: %v", err)
	}
	if payload["request_id"] != "order-456" {
		t.Fatalf("unexpected request_id: %v", payload["request_id"])
	}
	if payload["mode"] != "single" {
		t.Fatalf("unexpected mode: %v", payload["mode"])
	}
}

func TestCollateralOrderPlaceValidationFailure(t *testing.T) {
	_, _, err := executeCommand(
		"collateral", "order", "place",
		"--market", "BTC_PERP",
		"--side", "hold",
		"--amount", "0.01",
		"--price", "50000",
	)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "--side must be one of: buy, long, sell, short") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestCollateralOrderPlaceNotLoggedInErrorMapping(t *testing.T) {
	credentialStore := &testCredentialStore{backendName: "os-keychain"}
	sessionStore := &testSessionStore{}
	placeUseCase := &testCollateralUseCases{err: ports.ErrCredentialNotFound}

	application := testApplication(credentialStore, sessionStore, nil)
	application.Collateral = placeUseCase
	withApplicationFactory(t, application)

	_, _, err := executeCommand(
		"collateral", "order", "place",
		"--market", "BTC_PERP",
		"--side", "buy",
		"--amount", "0.01",
		"--price", "50000",
	)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "not logged in; run wbcli auth login first") {
		t.Fatalf("unexpected auth guidance error: %v", err)
	}
}

func TestCollateralOrderPlaceInsufficientPermissionErrorMapping(t *testing.T) {
	credentialStore := &testCredentialStore{backendName: "os-keychain"}
	sessionStore := &testSessionStore{}
	placeUseCase := &testCollateralUseCases{
		err: fmt.Errorf("whitebit api auth error: status 403: This API Key is not authorized to perform this action.: %w", whitebit.ErrForbidden),
	}

	application := testApplication(credentialStore, sessionStore, nil)
	application.Collateral = placeUseCase
	withApplicationFactory(t, application)

	_, _, err := executeCommand(
		"collateral", "order", "place",
		"--market", "BTC_PERP",
		"--side", "buy",
		"--amount", "0.01",
		"--price", "50000",
	)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "enable access to endpoint /api/v4/order/collateral/limit") {
		t.Fatalf("expected endpoint guidance in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "not authorized to perform this action") {
		t.Fatalf("expected api reason detail in error, got %v", err)
	}
}

func TestCollateralOrderRangeStub(t *testing.T) {
	stdout, _, err := executeCommand(
		"collateral", "order", "range",
		"--market", "BTC_PERP",
		"--side", "buy",
		"--start-price", "49000",
		"--end-price", "50000",
		"--step", "50",
		"--amount-mode", "constant",
		"--base-amount", "0.005",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "not implemented yet") {
		t.Fatalf("expected stub output, got: %q", stdout)
	}
}

func withApplicationFactory(t *testing.T, application *appcontainer.Application) {
	t.Helper()

	restore := SetApplicationFactoryForTest(func() (*appcontainer.Application, error) {
		return application, nil
	})
	t.Cleanup(restore)
}

func withApplicationFactoryError(t *testing.T, factoryErr error) {
	t.Helper()

	restore := SetApplicationFactoryForTest(func() (*appcontainer.Application, error) {
		return nil, factoryErr
	})
	t.Cleanup(restore)
}

func testApplication(
	credentialStore ports.CredentialStore,
	sessionStore ports.SessionStore,
	credentialVerifier ports.CredentialVerifier,
) *appcontainer.Application {
	if credentialVerifier == nil {
		credentialVerifier = &testCredentialVerifier{}
	}

	return appcontainer.NewWithAuthServices(
		authservice.NewLoginService(
			credentialStore,
			sessionStore,
			testClock{now: time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)},
			credentialVerifier,
		),
		authservice.NewLogoutService(credentialStore, sessionStore),
		authservice.NewStatusService(sessionStore),
	)
}

type testCollateralUseCases struct {
	result      collateralservice.PlaceOrderResult
	err         error
	lastRequest collateralservice.PlaceOrderRequest
}

func (useCases *testCollateralUseCases) PlaceOrder(
	_ context.Context,
	request collateralservice.PlaceOrderRequest,
) (collateralservice.PlaceOrderResult, error) {
	useCases.lastRequest = request
	if useCases.err != nil {
		return collateralservice.PlaceOrderResult{}, useCases.err
	}

	return useCases.result, nil
}

type testCredentialStore struct {
	backendName string
	credential  *domainauth.Credential
	saveErr     error
	loadErr     error
	existsErr   error
	deleteErr   error
}

func (store *testCredentialStore) BackendName() string {
	return store.backendName
}

func (store *testCredentialStore) Save(_ context.Context, credential domainauth.Credential) error {
	if store.saveErr != nil {
		return store.saveErr
	}

	copied := domainauth.Credential{
		APIKey:    credential.APIKey,
		APISecret: append([]byte(nil), credential.APISecret...),
	}
	store.credential = &copied
	return nil
}

func (store *testCredentialStore) Load(_ context.Context) (domainauth.Credential, error) {
	if store.loadErr != nil {
		return domainauth.Credential{}, store.loadErr
	}
	if store.credential == nil {
		return domainauth.Credential{}, ports.ErrCredentialNotFound
	}

	return domainauth.Credential{
		APIKey:    store.credential.APIKey,
		APISecret: append([]byte(nil), store.credential.APISecret...),
	}, nil
}

func (store *testCredentialStore) Exists(_ context.Context) (bool, error) {
	if store.existsErr != nil {
		return false, store.existsErr
	}

	return store.credential != nil, nil
}

func (store *testCredentialStore) Delete(_ context.Context) error {
	if store.deleteErr != nil {
		return store.deleteErr
	}
	if store.credential == nil {
		return ports.ErrCredentialNotFound
	}

	store.credential = nil
	return nil
}

type testSessionStore struct {
	session  *ports.SessionMetadata
	saveErr  error
	getErr   error
	clearErr error
}

func (store *testSessionStore) SaveSession(_ context.Context, session ports.SessionMetadata) error {
	if store.saveErr != nil {
		return store.saveErr
	}

	copied := session
	store.session = &copied
	return nil
}

func (store *testSessionStore) GetSession(_ context.Context) (ports.SessionMetadata, bool, error) {
	if store.getErr != nil {
		return ports.SessionMetadata{}, false, store.getErr
	}
	if store.session == nil {
		return ports.SessionMetadata{}, false, nil
	}

	return *store.session, true, nil
}

func (store *testSessionStore) ClearSession(_ context.Context) error {
	if store.clearErr != nil {
		return store.clearErr
	}

	store.session = nil
	return nil
}

type testClock struct {
	now time.Time
}

func (clock testClock) Now() time.Time {
	return clock.now
}

type testCredentialVerifier struct {
	err error
}

func (verifier *testCredentialVerifier) Verify(_ context.Context, _ domainauth.Credential) (ports.CredentialVerificationResult, error) {
	if verifier.err != nil {
		return ports.CredentialVerificationResult{}, verifier.err
	}

	return ports.CredentialVerificationResult{Endpoint: "/api/v4/collateral-account/hedge-mode"}, nil
}
