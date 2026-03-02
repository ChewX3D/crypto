package ports

import (
	"context"
	"errors"
	"time"

	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

var (
	// ErrCredentialNotFound indicates missing credentials in secure store.
	ErrCredentialNotFound = errors.New("credential not found")
	// ErrSecretStoreUnavailable indicates unavailable secure backend.
	ErrSecretStoreUnavailable = errors.New("secret store unavailable")
	// ErrSecretStorePermissionDenied indicates denied backend access.
	ErrSecretStorePermissionDenied = errors.New("secret store permission denied")
	// ErrCredentialVerifyUnauthorized indicates rejected WhiteBIT credentials.
	ErrCredentialVerifyUnauthorized = errors.New("credential verification unauthorized")
	// ErrCredentialVerifyForbidden indicates missing endpoint permission for credential.
	ErrCredentialVerifyForbidden = errors.New("credential verification forbidden")
	// ErrCredentialVerifyUnavailable indicates temporary WhiteBIT or transport failure.
	ErrCredentialVerifyUnavailable = errors.New("credential verification unavailable")
)

// CredentialVerificationReason classifies verification failures for user-facing messaging.
type CredentialVerificationReason string

const (
	// CredentialVerificationInvalidCredentials means key/secret pair is not accepted.
	CredentialVerificationInvalidCredentials CredentialVerificationReason = "invalid_credentials"
	// CredentialVerificationInsufficientAccess means token exists but lacks required endpoint permission.
	CredentialVerificationInsufficientAccess CredentialVerificationReason = "insufficient_access"
	// CredentialVerificationUnavailable means remote verification could not be completed reliably.
	CredentialVerificationUnavailable CredentialVerificationReason = "unavailable"
)

// CredentialVerificationError carries normalized, endpoint-aware verification failure details.
type CredentialVerificationError struct {
	Reason   CredentialVerificationReason
	Endpoint string
	Detail   string
}

// NewCredentialVerificationError constructs CredentialVerificationError.
func NewCredentialVerificationError(
	reason CredentialVerificationReason,
	endpoint string,
	detail string,
) *CredentialVerificationError {
	return &CredentialVerificationError{
		Reason:   reason,
		Endpoint: endpoint,
		Detail:   detail,
	}
}

// Error returns compact failure description.
func (err *CredentialVerificationError) Error() string {
	if err == nil {
		return ""
	}

	if err.Detail == "" {
		return string(err.Reason)
	}

	return string(err.Reason) + ": " + err.Detail
}

// Unwrap maps structured reason to existing sentinel errors.
func (err *CredentialVerificationError) Unwrap() error {
	if err == nil {
		return nil
	}

	switch err.Reason {
	case CredentialVerificationInvalidCredentials:
		return ErrCredentialVerifyUnauthorized
	case CredentialVerificationInsufficientAccess:
		return ErrCredentialVerifyForbidden
	case CredentialVerificationUnavailable:
		return ErrCredentialVerifyUnavailable
	default:
		return nil
	}
}

// SessionMetadata holds non-secret auth session information.
type SessionMetadata struct {
	Backend    string
	APIKeyHint string
	HedgeMode  *bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CredentialStore persists and retrieves secret credentials as a single slot.
type CredentialStore interface {
	BackendName() string
	Save(ctx context.Context, credential domainauth.Credential) error
	Load(ctx context.Context) (domainauth.Credential, error)
	Exists(ctx context.Context) (bool, error)
	Delete(ctx context.Context) error
}

// SessionStore persists and retrieves non-secret session metadata.
type SessionStore interface {
	SaveSession(ctx context.Context, session SessionMetadata) error
	GetSession(ctx context.Context) (SessionMetadata, bool, error)
	ClearSession(ctx context.Context) error
}

// Clock supplies deterministic time in services.
type Clock interface {
	Now() time.Time
}

// CredentialVerificationResult contains non-secret verification metadata.
type CredentialVerificationResult struct {
	Endpoint  string
	HedgeMode bool
}

// CredentialVerifier verifies credential validity and required permission against WhiteBIT.
type CredentialVerifier interface {
	Verify(ctx context.Context, credential domainauth.Credential) (CredentialVerificationResult, error)
}
