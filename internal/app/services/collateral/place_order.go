package collateral

import (
	"context"
	"fmt"
	"strings"

	"github.com/ChewX3D/wbcli/internal/app/ports"
	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

const (
	singleOrderMode       = "single"
	collateralOrderPrefix = "order"
)

// PlaceOrderRequest is input for collateral single order placement use-case.
type PlaceOrderRequest struct {
	Market        string
	Side          string
	Amount        string
	Price         string
	ClientOrderID string
}

// PlaceOrderResult is normalized output for collateral single order placement use-case.
type PlaceOrderResult struct {
	RequestID       string   `json:"request_id"`
	Mode            string   `json:"mode"`
	OrdersPlanned   int      `json:"orders_planned"`
	OrdersSubmitted int      `json:"orders_submitted"`
	OrdersFailed    int      `json:"orders_failed"`
	Errors          []string `json:"errors"`
}

// PlaceOrderService orchestrates collateral single order placement.
type PlaceOrderService struct {
	credentialStore ports.CredentialStore
	sessionStore    ports.SessionStore
	orderExecutor   ports.CollateralOrderExecutor
	clock           ports.Clock
}

// NewPlaceOrderService constructs PlaceOrderService.
func NewPlaceOrderService(
	credentialStore ports.CredentialStore,
	sessionStore ports.SessionStore,
	orderExecutor ports.CollateralOrderExecutor,
	clock ports.Clock,
) *PlaceOrderService {
	return &PlaceOrderService{
		credentialStore: credentialStore,
		sessionStore:    sessionStore,
		orderExecutor:   orderExecutor,
		clock:           clock,
	}
}

// Execute places one collateral post-only limit order.
func (service *PlaceOrderService) Execute(ctx context.Context, request PlaceOrderRequest) (PlaceOrderResult, error) {
	credential, err := service.credentialStore.Load(ctx)
	if err != nil {
		return PlaceOrderResult{}, fmt.Errorf("load credential: %w", err)
	}

	hedgeMode, err := service.resolveHedgeMode(ctx, credential)
	if err != nil {
		return PlaceOrderResult{}, fmt.Errorf("resolve hedge mode: %w", err)
	}

	requestPayload := buildOrderRequest(request, hedgeMode)
	_, err = service.orderExecutor.PlaceCollateralLimitOrder(ctx, credential, requestPayload)
	if err != nil && isHedgeModeMismatchError(err) {
		refreshedHedgeMode, refreshErr := service.refreshHedgeMode(ctx, credential)
		if refreshErr != nil {
			return PlaceOrderResult{}, fmt.Errorf(
				"place collateral limit order: %w; refresh hedge mode: %v",
				err,
				refreshErr,
			)
		}

		requestPayload = buildOrderRequest(request, refreshedHedgeMode)
		_, err = service.orderExecutor.PlaceCollateralLimitOrder(ctx, credential, requestPayload)
	}
	if err != nil {
		return PlaceOrderResult{}, fmt.Errorf("place collateral limit order: %w", err)
	}

	return PlaceOrderResult{
		RequestID:       fmt.Sprintf("%s-%d", collateralOrderPrefix, service.clock.Now().UTC().UnixNano()),
		Mode:            singleOrderMode,
		OrdersPlanned:   1,
		OrdersSubmitted: 1,
		OrdersFailed:    0,
		Errors:          []string{},
	}, nil
}

func (service *PlaceOrderService) resolveHedgeMode(ctx context.Context, credential domainauth.Credential) (bool, error) {
	session, found, err := service.sessionStore.GetSession(ctx)
	if err != nil {
		return false, fmt.Errorf("read session metadata: %w", err)
	}
	if found && session.HedgeMode != nil {
		return *session.HedgeMode, nil
	}

	return service.refreshHedgeMode(ctx, credential)
}

func (service *PlaceOrderService) refreshHedgeMode(ctx context.Context, credential domainauth.Credential) (bool, error) {
	value, err := service.orderExecutor.GetCollateralAccountHedgeMode(ctx, credential)
	if err != nil {
		return false, err
	}
	if err := service.persistHedgeMode(ctx, value); err != nil {
		return false, err
	}

	return value, nil
}

func (service *PlaceOrderService) persistHedgeMode(ctx context.Context, hedgeMode bool) error {
	session, found, err := service.sessionStore.GetSession(ctx)
	if err != nil {
		return fmt.Errorf("read session metadata: %w", err)
	}

	now := service.clock.Now().UTC()
	if !found {
		session = ports.SessionMetadata{
			Backend:   service.credentialStore.BackendName(),
			CreatedAt: now,
		}
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}

	session.UpdatedAt = now
	session.HedgeMode = boolRef(hedgeMode)
	if err := service.sessionStore.SaveSession(ctx, session); err != nil {
		return fmt.Errorf("save session metadata: %w", err)
	}

	return nil
}

func buildOrderRequest(request PlaceOrderRequest, hedgeMode bool) ports.CollateralLimitOrderRequest {
	orderSide, positionSide := resolveOrderSides(strings.TrimSpace(request.Side), hedgeMode)

	return ports.CollateralLimitOrderRequest{
		Market:        strings.TrimSpace(request.Market),
		Side:          orderSide,
		PositionSide:  positionSide,
		Amount:        strings.TrimSpace(request.Amount),
		Price:         strings.TrimSpace(request.Price),
		ClientOrderID: request.ClientOrderID,
		PostOnly:      true,
	}
}

func resolveOrderSides(side string, hedgeMode bool) (string, string) {
	normalized := strings.ToLower(strings.TrimSpace(side))
	if hedgeMode {
		switch normalized {
		case "long", "buy":
			return "buy", "long"
		case "short", "sell":
			return "sell", "short"
		default:
			return normalized, ""
		}
	}

	switch normalized {
	case "long", "buy":
		return "buy", ""
	case "short", "sell":
		return "sell", ""
	default:
		return normalized, ""
	}
}

func isHedgeModeMismatchError(err error) bool {
	detail := strings.ToLower(err.Error())
	if detail == "" {
		return false
	}

	return strings.Contains(detail, "hedgemode") &&
		strings.Contains(detail, "position side does not match user's setting")
}

func boolRef(value bool) *bool {
	allocated := value
	return &allocated
}
