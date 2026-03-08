package whitebit

import (
	"context"
	"encoding/json"
	"errors"

	domainauth "github.com/ChewX3D/crypto/internal/domain/auth"
)

const (
	URLPathCollateralAccountHedgeMode = "/api/v4/collateral-account/hedge-mode"
	URLPathCollateralLimitOrder       = "/api/v4/order/collateral/limit"
	URLPathCollateralLimitOrderBulk   = "/api/v4/order/collateral/bulk"
)

var (
	// ErrMarketRequired indicates missing market in order request.
	ErrMarketRequired = errors.New("market is required")
	// ErrAmountRequired indicates missing amount in order request.
	ErrAmountRequired = errors.New("amount is required")
	// ErrPriceRequired indicates missing price in order request.
	ErrPriceRequired = errors.New("price is required")
	// ErrInvalidOrderSide indicates unknown order side enum value.
	ErrInvalidOrderSide = errors.New("invalid order side")
	// ErrInvalidPositionSide indicates unknown position side enum value.
	ErrInvalidPositionSide = errors.New("invalid position side")
	// ErrIOCConflict indicates unsupported ioc combination with postOnly or rpi.
	ErrIOCConflict = errors.New("ioc cannot be combined with postOnly or rpi")
	// ErrOrdersRequired indicates missing orders array for bulk endpoint.
	ErrOrdersRequired = errors.New("orders are required")
)

// OrderSide is a documented WhiteBIT enum for order direction.
type OrderSide string

const (
	// OrderSideBuy represents buy side value from WhiteBIT API docs.
	OrderSideBuy OrderSide = "buy"
	// OrderSideSell represents sell side value from WhiteBIT API docs.
	OrderSideSell OrderSide = "sell"
)

// IsValid returns true when side matches documented WhiteBIT enum values.
func (side OrderSide) IsValid() bool {
	return side == OrderSideBuy || side == OrderSideSell
}

// PositionSide is a documented WhiteBIT enum for collateral position side.
type PositionSide string

const (
	// PositionSideLong represents long position side enum value.
	PositionSideLong PositionSide = "long"
	// PositionSideShort represents short position side enum value.
	PositionSideShort PositionSide = "short"
)

// IsValid returns true when position side matches documented enum values.
func (side PositionSide) IsValid() bool {
	return side == PositionSideLong || side == PositionSideShort
}

// CollateralAccountHedgeModeResponse models hedge mode response payload.
type CollateralAccountHedgeModeResponse struct {
	HedgeMode bool `json:"hedgeMode"`
}

// CollateralLimitOrderRequest is request payload for limit order endpoint.
type CollateralLimitOrderRequest struct {
	Market        string       `json:"market"`
	Side          OrderSide    `json:"side"`
	Amount        string       `json:"amount"`
	Price         string       `json:"price"`
	PositionSide  PositionSide `json:"positionSide,omitempty"`
	ClientOrderID string       `json:"clientOrderId,omitempty"`
	PostOnly      *bool        `json:"postOnly,omitempty"`
	IOC           *bool        `json:"ioc,omitempty"`
	RPI           *bool        `json:"rpi,omitempty"`
	StopLoss      string       `json:"stopLoss,omitempty"`
	TakeProfit    string       `json:"takeProfit,omitempty"`
}

// CollateralBulkLimitOrderRequest is request payload for bulk limit order endpoint.
type CollateralBulkLimitOrderRequest struct {
	Orders     []CollateralLimitOrderRequest `json:"orders"`
	StopOnFail *bool                         `json:"stopOnFail,omitempty"`
}

type collateralHedgeModePayload struct {
	privateEnvelope
}

type collateralLimitOrderPayload struct {
	privateEnvelope
	CollateralLimitOrderRequest
}

type collateralBulkLimitOrderPayload struct {
	privateEnvelope
	Orders     []CollateralLimitOrderRequest `json:"orders"`
	StopOnFail *bool                         `json:"stopOnFail,omitempty"`
}

func (request CollateralLimitOrderRequest) validate() error {
	if request.Market == "" {
		return ErrMarketRequired
	}
	if request.Amount == "" {
		return ErrAmountRequired
	}
	if request.Price == "" {
		return ErrPriceRequired
	}
	if !request.Side.IsValid() {
		return ErrInvalidOrderSide
	}
	if request.PositionSide != "" && !request.PositionSide.IsValid() {
		return ErrInvalidPositionSide
	}
	if request.IOC != nil && *request.IOC {
		if request.PostOnly != nil && *request.PostOnly {
			return ErrIOCConflict
		}
		if request.RPI != nil && *request.RPI {
			return ErrIOCConflict
		}
	}

	return nil
}

func (request CollateralBulkLimitOrderRequest) validate() error {
	if len(request.Orders) == 0 {
		return ErrOrdersRequired
	}

	for index := range request.Orders {
		if err := request.Orders[index].validate(); err != nil {
			return err
		}
	}

	return nil
}

// GetCollateralAccountHedgeMode calls WhiteBIT collateral hedge-mode endpoint.
func (client *Client) GetCollateralAccountHedgeMode(
	ctx context.Context,
	credential domainauth.Credential,
) (CollateralAccountHedgeModeResponse, error) {
	payload := collateralHedgeModePayload{
		privateEnvelope: client.nextPrivateEnvelope(URLPathCollateralAccountHedgeMode),
	}

	var response CollateralAccountHedgeModeResponse
	if err := client.doPrivateRequest(ctx, credential, URLPathCollateralAccountHedgeMode, payload, &response); err != nil {
		return CollateralAccountHedgeModeResponse{}, err
	}

	return response, nil
}

// PlaceCollateralLimitOrder calls WhiteBIT collateral limit order endpoint.
func (client *Client) PlaceCollateralLimitOrder(
	ctx context.Context,
	credential domainauth.Credential,
	request CollateralLimitOrderRequest,
) (json.RawMessage, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}

	payload := collateralLimitOrderPayload{
		privateEnvelope:             client.nextPrivateEnvelope(URLPathCollateralLimitOrder),
		CollateralLimitOrderRequest: request,
	}

	var response json.RawMessage
	if err := client.doPrivateRequest(ctx, credential, URLPathCollateralLimitOrder, payload, &response); err != nil {
		return nil, err
	}

	return response, nil
}

// PlaceCollateralBulkLimitOrder calls WhiteBIT collateral bulk limit order endpoint.
func (client *Client) PlaceCollateralBulkLimitOrder(
	ctx context.Context,
	credential domainauth.Credential,
	request CollateralBulkLimitOrderRequest,
) (json.RawMessage, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}

	payload := collateralBulkLimitOrderPayload{
		privateEnvelope: client.nextPrivateEnvelope(URLPathCollateralLimitOrderBulk),
		Orders:          request.Orders,
		StopOnFail:      request.StopOnFail,
	}

	var response json.RawMessage
	if err := client.doPrivateRequest(ctx, credential, URLPathCollateralLimitOrderBulk, payload, &response); err != nil {
		return nil, err
	}

	return response, nil
}
