package whitebit_credentials_adapters

import (
	"context"

	"github.com/ChewX3D/crypto/internal/adapters/whitebit"
	whitebit_adapters_common "github.com/ChewX3D/crypto/internal/adapters/whitebit/adapters"
	"github.com/ChewX3D/crypto/internal/app/ports"
	domainauth "github.com/ChewX3D/crypto/internal/domain/auth"
)

// CredentialVerifierAdapter adapts auth credential verification port to WhiteBIT client endpoints.
type CredentialVerifierAdapter struct {
	client whitebit.PrivateClient
}

var _ ports.CredentialVerifier = (*CredentialVerifierAdapter)(nil)

// NewCredentialVerifierAdapter constructs CredentialVerifierAdapter.
func NewCredentialVerifierAdapter(client whitebit.PrivateClient) *CredentialVerifierAdapter {
	return &CredentialVerifierAdapter{client: client}
}

// NewDefaultCredentialVerifierAdapter constructs CredentialVerifierAdapter with default WhiteBIT client.
func NewDefaultCredentialVerifierAdapter() *CredentialVerifierAdapter {
	return NewCredentialVerifierAdapter(whitebit.NewDefaultClient())
}

// Verify checks login credentials using the documented hedge-mode endpoint.
func (adapter *CredentialVerifierAdapter) Verify(ctx context.Context, credential domainauth.Credential) (ports.CredentialVerificationResult, error) {
	if adapter == nil || adapter.client == nil {
		return ports.CredentialVerificationResult{}, &ports.APIError{
			Code:    ports.CodeUnavailable,
			Message: "credential verification failed: exchange unavailable",
			Details: "credential verifier adapter is not configured",
		}
	}

	response, err := adapter.client.GetCollateralAccountHedgeMode(ctx, credential)
	if err == nil {
		return ports.CredentialVerificationResult{
			Endpoint:  whitebit.URLPathCollateralAccountHedgeMode,
			HedgeMode: response.HedgeMode,
		}, nil
	}

	return ports.CredentialVerificationResult{},
		whitebit_adapters_common.BuildAPIError(
			err,
			whitebit.URLPathCollateralAccountHedgeMode,
			"credential verification")
}
