package whitebit

import (
	"context"
	"errors"
	"strings"

	"github.com/ChewX3D/wbcli/internal/app/ports"
	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

// CredentialVerifierAdapter adapts auth credential verification port to WhiteBIT client endpoints.
type CredentialVerifierAdapter struct {
	client *Client
}

// NewCredentialVerifierAdapter constructs CredentialVerifierAdapter.
func NewCredentialVerifierAdapter(client *Client) *CredentialVerifierAdapter {
	return &CredentialVerifierAdapter{client: client}
}

// NewDefaultCredentialVerifierAdapter constructs CredentialVerifierAdapter with default WhiteBIT client.
func NewDefaultCredentialVerifierAdapter() *CredentialVerifierAdapter {
	return NewCredentialVerifierAdapter(NewDefaultClient())
}

// Verify checks login credentials using the documented hedge-mode endpoint.
func (adapter *CredentialVerifierAdapter) Verify(ctx context.Context, credential domainauth.Credential) (ports.CredentialVerificationResult, error) {
	if adapter == nil || adapter.client == nil {
		return ports.CredentialVerificationResult{}, ports.NewCredentialVerificationError(
			ports.CredentialVerificationUnavailable,
			"",
			"credential verifier adapter is not configured",
		)
	}

	response, err := adapter.client.GetCollateralAccountHedgeMode(ctx, credential)
	if err == nil {
		return ports.CredentialVerificationResult{
			Endpoint:  collateralAccountHedgeModePath,
			HedgeMode: response.HedgeMode,
		}, nil
	}

	return ports.CredentialVerificationResult{}, ports.NewCredentialVerificationError(
		classifyVerificationReason(err),
		collateralAccountHedgeModePath,
		extractVerificationDetail(err),
	)
}

func classifyVerificationReason(err error) ports.CredentialVerificationReason {
	switch {
	case errors.Is(err, ErrForbidden):
		return ports.CredentialVerificationInsufficientAccess
	case errors.Is(err, ErrUnauthorized):
		if indicatesMissingEndpointAccess(err) {
			return ports.CredentialVerificationInsufficientAccess
		}

		return ports.CredentialVerificationInvalidCredentials
	default:
		return ports.CredentialVerificationUnavailable
	}
}

func extractVerificationDetail(err error) string {
	if err == nil {
		return ""
	}

	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return ""
	}

	suffixes := []string{
		": whitebit unauthorized",
		": whitebit forbidden",
	}
	for _, suffix := range suffixes {
		detail = strings.TrimSuffix(detail, suffix)
	}

	return strings.TrimSpace(detail)
}

func indicatesMissingEndpointAccess(err error) bool {
	detail := strings.ToLower(extractVerificationDetail(err))
	if detail == "" {
		return false
	}

	return strings.Contains(detail, "not authorized to perform this action") ||
		strings.Contains(detail, "not authorised to perform this action")
}
