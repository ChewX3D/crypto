package ports

import (
	"context"
	"encoding/json"

	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

// CollateralLimitOrderRequest defines collateral limit order fields needed by app use-cases.
type CollateralLimitOrderRequest struct {
	Market        string
	Side          string
	PositionSide  string
	Amount        string
	Price         string
	ClientOrderID string
	PostOnly      bool
}

// CollateralOrderExecutor submits collateral orders to external exchange APIs.
type CollateralOrderExecutor interface {
	GetCollateralAccountHedgeMode(
		ctx context.Context,
		credential domainauth.Credential,
	) (bool, error)
	PlaceCollateralLimitOrder(
		ctx context.Context,
		credential domainauth.Credential,
		request CollateralLimitOrderRequest,
	) (json.RawMessage, error)
}
