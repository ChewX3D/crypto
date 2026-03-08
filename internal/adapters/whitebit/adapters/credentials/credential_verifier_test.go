package whitebit_credentials_adapters

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChewX3D/crypto/internal/adapters/whitebit"
	"github.com/ChewX3D/crypto/internal/app/ports"
	domainauth "github.com/ChewX3D/crypto/internal/domain/auth"
)

type fixedNonceSource struct {
	value int64
}

func (source fixedNonceSource) Next() int64 {
	return source.value
}

func TestCredentialVerifierAdapterVerifyMapsErrors(t *testing.T) {
	testCases := []struct {
		name         string
		statusCode   int
		expectedCode ports.ErrorCode
	}{
		{
			name:         "unauthorized",
			statusCode:   http.StatusUnauthorized,
			expectedCode: ports.CodeUnauthorized,
		},
		{
			name:         "forbidden",
			statusCode:   http.StatusForbidden,
			expectedCode: ports.CodeForbidden,
		},
		{
			name:         "unavailable",
			statusCode:   http.StatusServiceUnavailable,
			expectedCode: ports.CodeUnavailable,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(testCase.statusCode)
			}))
			defer server.Close()

			client := whitebit.NewClient(server.URL, server.Client(), fixedNonceSource{value: 1})
			adapter := NewCredentialVerifierAdapter(client)
			_, err := adapter.Verify(context.Background(), domainauth.Credential{
				APIKey:    "public-key",
				APISecret: []byte("secret-key"),
			})
			var apiErr *ports.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected APIError, got %T: %v", err, err)
			}
			if apiErr.Code != testCase.expectedCode {
				t.Fatalf("expected code %v, got %v", testCase.expectedCode, apiErr.Code)
			}
		})
	}
}

func TestCredentialVerifierAdapterVerifyReturnsEndpointOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"hedgeMode":true}`))
	}))
	defer server.Close()

	client := whitebit.NewClient(server.URL, server.Client(), fixedNonceSource{value: 1})
	adapter := NewCredentialVerifierAdapter(client)

	result, err := adapter.Verify(context.Background(), domainauth.Credential{
		APIKey:    "public-key",
		APISecret: []byte("secret-key"),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Endpoint != whitebit.URLPathCollateralAccountHedgeMode {
		t.Fatalf("expected endpoint %q, got %q", whitebit.URLPathCollateralAccountHedgeMode, result.Endpoint)
	}
	if !result.HedgeMode {
		t.Fatalf("expected hedge_mode=true, got false")
	}
}

func TestCredentialVerifierAdapterUnauthorizedActionDeniedMapsToInsufficientAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"message":"This API Key is not authorized to perform this action."}`))
	}))
	defer server.Close()

	client := whitebit.NewClient(server.URL, server.Client(), fixedNonceSource{value: 1})
	adapter := NewCredentialVerifierAdapter(client)

	_, err := adapter.Verify(context.Background(), domainauth.Credential{
		APIKey:    "public-key",
		APISecret: []byte("secret-key"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	var apiErr *ports.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.Code != ports.CodeForbidden {
		t.Fatalf("expected CodeForbidden, got %s", apiErr.Code)
	}
	if !strings.Contains(apiErr.Details, whitebit.URLPathCollateralAccountHedgeMode) {
		t.Fatalf("expected endpoint %q in details, got %q", whitebit.URLPathCollateralAccountHedgeMode, apiErr.Details)
	}
	if !strings.Contains(strings.ToLower(apiErr.Details), "not authorized to perform this action") {
		t.Fatalf("expected action denied detail, got %q", apiErr.Details)
	}
}
