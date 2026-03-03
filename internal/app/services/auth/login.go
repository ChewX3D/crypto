package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ChewX3D/wbcli/internal/app/ports"
	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
	"github.com/ChewX3D/wbcli/internal/ptrutil"
)

// LoginRequest is input for auth login use-case.
type LoginRequest struct {
	APIKey    string
	APISecret []byte
}

// LoginResult is safe output for auth login use-case.
type LoginResult struct {
	Backend    string
	APIKeyHint string
	SavedAt    string
}

// LoginService stores credentials securely for single-session auth.
type LoginService struct {
	credentialStore    ports.CredentialStore
	sessionStore       ports.SessionStore
	clock              ports.Clock
	credentialVerifier ports.CredentialVerifier
}

// NewLoginService constructs LoginService.
func NewLoginService(
	credentialStore ports.CredentialStore,
	sessionStore ports.SessionStore,
	clock ports.Clock,
	credentialVerifier ports.CredentialVerifier,
) *LoginService {
	return &LoginService{
		credentialStore:    credentialStore,
		sessionStore:       sessionStore,
		clock:              clock,
		credentialVerifier: credentialVerifier,
	}
}

// Execute validates input and writes auth material into stores.
func (service *LoginService) Execute(ctx context.Context, request LoginRequest) (LoginResult, error) {
	credential := domainauth.Credential{
		APIKey:    request.APIKey,
		APISecret: request.APISecret,
	}
	if err := credential.Validate(); err != nil {
		return LoginResult{}, err
	}
	defer domainauth.WipeBytes(request.APISecret)

	if service.credentialVerifier == nil {
		return LoginResult{}, &ports.APIError{
			Code:    ports.CodeUnavailable,
			Message: "credential verification failed: exchange unavailable",
			Details: "credential verifier is not configured",
		}
	}
	verificationResult, err := service.credentialVerifier.Verify(ctx, credential)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify credential: %w", err)
	}
	slog.Info("auth credential verified", "endpoint", verificationResult.Endpoint)

	previous, found, err := service.sessionStore.GetSession(ctx)
	if err != nil {
		return LoginResult{}, fmt.Errorf("read session metadata: %w", err)
	}

	now := service.clock.Now().UTC()
	session := ports.SessionMetadata{
		Backend:    service.credentialStore.BackendName(),
		APIKeyHint: domainauth.APIKeyHint(request.APIKey),
		HedgeMode:  ptrutil.Ptr(verificationResult.HedgeMode),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if found {
		session.CreatedAt = previous.CreatedAt
	}

	if err := service.credentialStore.Save(ctx, credential); err != nil {
		return LoginResult{}, fmt.Errorf("save credential: %w", err)
	}
	if err := service.sessionStore.SaveSession(ctx, session); err != nil {
		return LoginResult{}, fmt.Errorf("save session metadata: %w", err)
	}

	return LoginResult{
		Backend:    session.Backend,
		APIKeyHint: session.APIKeyHint,
		SavedAt:    now.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}
