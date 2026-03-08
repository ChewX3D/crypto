package whitebit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	domainauth "github.com/ChewX3D/crypto/internal/domain/auth"
)

const (
	defaultBaseURL      = "https://whitebit.com"
	defaultHTTPTimeout  = 10 * time.Second
	maxResponseBodySize = 64 * 1024
)

var (
	// ErrAPIAuth indicates authentication or authorization failure.
	ErrAPIAuth = errors.New("whitebit api auth error")
	// ErrUnauthorized indicates invalid API key/secret.
	ErrUnauthorized = errors.New("whitebit unauthorized")
	// ErrForbidden indicates valid credential without required permission.
	ErrForbidden = errors.New("whitebit forbidden")
	// ErrAPIValidation indicates request schema/validation failure.
	ErrAPIValidation = errors.New("whitebit api validation error")
	// ErrAPIBusinessRule indicates domain/business rejection by WhiteBIT.
	ErrAPIBusinessRule = errors.New("whitebit api business rule error")
	// ErrAPITransport indicates temporary transport/server/rate-limit failure.
	ErrAPITransport = errors.New("whitebit api transport error")
)

// HTTPDoer executes HTTP requests. It enables client testability.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// NonceSource generates strictly increasing nonces.
type NonceSource interface {
	Next() int64
}

// MonotonicUnixMilliNonceSource generates process-local monotonic unix-millisecond nonces.
type MonotonicUnixMilliNonceSource struct {
	mu   sync.Mutex
	last int64
}

// Next returns a strictly increasing nonce.
func (source *MonotonicUnixMilliNonceSource) Next() int64 {
	now := time.Now().UnixMilli()

	source.mu.Lock()
	defer source.mu.Unlock()

	if now <= source.last {
		now = source.last + 1
	}
	source.last = now

	return now
}

// PrivateClient defines the contract for signed private WhiteBIT API operations.
type PrivateClient interface {
	GetCollateralAccountHedgeMode(ctx context.Context, credential domainauth.Credential) (CollateralAccountHedgeModeResponse, error)
	PlaceCollateralLimitOrder(ctx context.Context, credential domainauth.Credential, request CollateralLimitOrderRequest) (json.RawMessage, error)
	PlaceCollateralBulkLimitOrder(ctx context.Context, credential domainauth.Credential, request CollateralBulkLimitOrderRequest) (json.RawMessage, error)
}

// Client executes signed private WhiteBIT HTTP API requests.
type Client struct {
	baseURL     string
	httpDoer    HTTPDoer
	nonceSource NonceSource
}

// NewDefaultClient constructs Client with production defaults.
func NewDefaultClient() *Client {
	return NewClient(defaultBaseURL, nil, nil)
}

// NewClient constructs Client with injectable dependencies for tests.
func NewClient(baseURL string, httpDoer HTTPDoer, nonceSource NonceSource) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if httpDoer == nil {
		httpDoer = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if nonceSource == nil {
		nonceSource = &MonotonicUnixMilliNonceSource{}
	}

	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpDoer:    httpDoer,
		nonceSource: nonceSource,
	}
}

type privateEnvelope struct {
	Request     string `json:"request"`
	Nonce       string `json:"nonce"`
	NonceWindow bool   `json:"nonceWindow"`
}

func (client *Client) nextPrivateEnvelope(path string) privateEnvelope {
	return privateEnvelope{
		Request:     path,
		Nonce:       strconv.FormatInt(client.nonceSource.Next(), 10),
		NonceWindow: true,
	}
}

func (client *Client) doPrivateRequest(
	ctx context.Context,
	credential domainauth.Credential,
	path string,
	requestPayload any,
	responsePayload any,
) error {
	if err := credential.Validate(); err != nil {
		return err
	}

	rawBody, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("encode request body: %w", err)
	}

	encodedPayload := base64.StdEncoding.EncodeToString(rawBody)
	signature := signPayload(encodedPayload, credential.APISecret)

	endpointURL := client.baseURL + path
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(rawBody))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-TXC-APIKEY", credential.APIKey)
	request.Header.Set("X-TXC-PAYLOAD", encodedPayload)
	request.Header.Set("X-TXC-SIGNATURE", signature)

	response, err := client.httpDoer.Do(request)
	if err != nil {
		return fmt.Errorf("%w: request failed", ErrAPITransport)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodySize))
		_ = response.Body.Close()
	}()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode > 299 {
		return mapHTTPStatusError(response.StatusCode, responseBody)
	}

	if responsePayload == nil || len(responseBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, responsePayload); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func mapHTTPStatusError(statusCode int, body []byte) error {
	responseMessage := extractErrorMessage(body)
	wrapStatus := func(base error) error {
		if responseMessage == "" {
			return fmt.Errorf("%w: status %d", base, statusCode)
		}
		return fmt.Errorf("%w: status %d: %s", base, statusCode, responseMessage)
	}

	switch {
	case statusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", wrapStatus(ErrAPIAuth), ErrUnauthorized)
	case statusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %w", wrapStatus(ErrAPIAuth), ErrForbidden)
	case statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity:
		return wrapStatus(ErrAPIValidation)
	case statusCode == http.StatusTooManyRequests || statusCode >= 500:
		return wrapStatus(ErrAPITransport)
	case statusCode >= 400 && statusCode <= 499:
		return wrapStatus(ErrAPIBusinessRule)
	default:
		return wrapStatus(ErrAPITransport)
	}
}

func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	message := firstNonEmptyString(
		payload["message"],
		payload["error"],
		payload["detail"],
		payload["description"],
	)
	validationDetails := flattenValidationErrors(payload["errors"])

	var result string
	switch {
	case message == "" && validationDetails == "":
		return ""
	case message == "":
		result = validationDetails
	case validationDetails == "":
		result = message
	default:
		result = message + ": " + validationDetails
	}

	if code, ok := payload["code"].(float64); ok && code > 0 {
		result = fmt.Sprintf("code %d: %s", int(code), result)
	}

	return result
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		asString, ok := value.(string)
		if !ok {
			continue
		}

		trimmed := strings.TrimSpace(asString)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func flattenValidationErrors(value any) string {
	if value == nil {
		return ""
	}

	byField, ok := value.(map[string]any)
	if !ok {
		return strings.Join(flattenErrorLeaf(value), ", ")
	}

	keys := make([]string, 0, len(byField))
	for key := range byField {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		messages := flattenErrorLeaf(byField[key])
		if len(messages) == 0 {
			continue
		}

		parts = append(parts, key+": "+strings.Join(messages, ", "))
	}

	return strings.Join(parts, "; ")
}

func flattenErrorLeaf(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}

		return []string{trimmed}
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, flattenErrorLeaf(item)...)
		}

		return parts
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		parts := make([]string, 0, len(typed))
		for _, key := range keys {
			nested := flattenErrorLeaf(typed[key])
			if len(nested) == 0 {
				continue
			}
			parts = append(parts, key+"="+strings.Join(nested, ", "))
		}

		return parts
	default:
		return nil
	}
}

func signPayload(encodedPayload string, secret []byte) string {
	mac := hmac.New(sha512.New, secret)
	_, _ = mac.Write([]byte(encodedPayload))
	return hex.EncodeToString(mac.Sum(nil))
}
