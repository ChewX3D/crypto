package whitebit_adapters_common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ChewX3D/crypto/internal/adapters/whitebit"
	"github.com/ChewX3D/crypto/internal/app/ports"
)

// BuildAPIError converts a WhiteBIT transport error into a ports.APIError with a user-facing
// message and full details. endpoint is the API path that failed; operation is a short
// description used in the message (e.g. "credential verification", "order placement").
func BuildAPIError(err error, endpoint, operation string) *ports.APIError {
	detail := extractDetail(err)

	switch {
	case errors.Is(err, whitebit.ErrForbidden) || (errors.Is(err, whitebit.ErrUnauthorized) && indicatesMissingEndpointAccess(err)):
		return &ports.APIError{
			Code:    ports.CodeForbidden,
			Message: operation + " failed: API token lacks endpoint permission",
			Details: fmt.Sprintf(
				"enable access to endpoint %s in your WhiteBIT API key settings. reason: %s",
				endpoint, detail,
			),
		}
	case errors.Is(err, whitebit.ErrUnauthorized):
		return &ports.APIError{
			Code:    ports.CodeUnauthorized,
			Message: operation + " failed: credentials are invalid",
			Details: fmt.Sprintf("endpoint: %s. reason: %s", endpoint, detail),
		}
	default:
		return &ports.APIError{
			Code:    ports.CodeUnavailable,
			Message: operation + " failed: exchange unavailable",
			Details: fmt.Sprintf("endpoint: %s. reason: %s", endpoint, err.Error()),
		}
	}
}

// extractDetail strips WhiteBIT-specific error suffixes to surface the exchange message.
func extractDetail(err error) string {
	if err == nil {
		return ""
	}

	detail := strings.TrimSpace(err.Error())
	for _, suffix := range []string{": whitebit unauthorized", ": whitebit forbidden"} {
		detail = strings.TrimSuffix(detail, suffix)
	}

	return strings.TrimSpace(detail)
}

// indicatesMissingEndpointAccess reports whether the error message signals that the API
// token exists but lacks permission for the requested endpoint.
func indicatesMissingEndpointAccess(err error) bool {
	detail := strings.ToLower(extractDetail(err))

	return strings.Contains(detail, "not authorized to perform this action") ||
		strings.Contains(detail, "not authorised to perform this action")
}
