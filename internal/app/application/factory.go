package application

import (
	"context"
	"fmt"

	"github.com/ChewX3D/crypto/internal/adapters/clock"
	"github.com/ChewX3D/crypto/internal/adapters/configstore"
	"github.com/ChewX3D/crypto/internal/adapters/secretstore"
	"github.com/ChewX3D/crypto/internal/adapters/whitebit/adapters/collaterlal"
	whitebit_credentials_adapters "github.com/ChewX3D/crypto/internal/adapters/whitebit/adapters/credentials"
	authservice "github.com/ChewX3D/crypto/internal/app/services/auth"
	collateralservice "github.com/ChewX3D/crypto/internal/app/services/collateral"
)

// AuthUseCases defines auth operations exposed to command adapters.
type AuthUseCases interface {
	Login(ctx context.Context, request authservice.LoginRequest) (authservice.LoginResult, error)
	Logout(ctx context.Context) (authservice.LogoutResult, error)
	Status(ctx context.Context) (authservice.StatusResult, error)
}

// CollateralUseCases defines collateral operations exposed to command adapters.
type CollateralUseCases interface {
	PlaceOrder(ctx context.Context, request collateralservice.PlaceOrderRequest) (collateralservice.PlaceOrderResult, error)
}

// Application holds use-case interfaces used by CLI command adapters.
type Application struct {
	Auth       AuthUseCases
	Collateral CollateralUseCases
}

type authUseCases struct {
	login  *authservice.LoginService
	logout *authservice.LogoutService
	status *authservice.StatusService
}

type collateralUseCases struct {
	placeOrder *collateralservice.PlaceOrderService
}

// New constructs application container from prepared use-case interfaces.
func New(auth AuthUseCases) *Application {
	return &Application{Auth: auth}
}

// NewWithUseCases constructs application container from prepared use-case interfaces.
func NewWithUseCases(auth AuthUseCases, collateral CollateralUseCases) *Application {
	return &Application{
		Auth:       auth,
		Collateral: collateral,
	}
}

// NewWithAuthServices constructs application container from concrete auth services.
func NewWithAuthServices(
	login *authservice.LoginService,
	logout *authservice.LogoutService,
	status *authservice.StatusService,
) *Application {
	return NewWithUseCases(&authUseCases{
		login:  login,
		logout: logout,
		status: status,
	}, nil)
}

// NewWithServices constructs application container from concrete auth and collateral services.
func NewWithServices(
	login *authservice.LoginService,
	logout *authservice.LogoutService,
	status *authservice.StatusService,
	placeOrder *collateralservice.PlaceOrderService,
) *Application {
	return NewWithUseCases(&authUseCases{
		login:  login,
		logout: logout,
		status: status,
	}, &collateralUseCases{
		placeOrder: placeOrder,
	})
}

// NewDefault wires adapters and services for production runtime.
func NewDefault() (*Application, error) {
	sessionStore, err := configstore.NewDefaultSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init session store: %w", err)
	}

	credentialStore := secretstore.NewOSKeychainStore()
	credentialVerifier := whitebit_credentials_adapters.NewDefaultCredentialVerifierAdapter()
	collateralOrderExecutor := whitebit_collateral_adapters.NewDefaultCollateralOrderExecutorAdapter()
	realClock := clock.Real{}

	return NewWithServices(
		authservice.NewLoginService(credentialStore, sessionStore, realClock, credentialVerifier),
		authservice.NewLogoutService(credentialStore, sessionStore),
		authservice.NewStatusService(sessionStore),
		collateralservice.NewPlaceOrderService(credentialStore, sessionStore, collateralOrderExecutor, realClock),
	), nil
}

func (useCases *authUseCases) Login(ctx context.Context, request authservice.LoginRequest) (authservice.LoginResult, error) {
	return useCases.login.Execute(ctx, request)
}

func (useCases *authUseCases) Logout(ctx context.Context) (authservice.LogoutResult, error) {
	return useCases.logout.Execute(ctx)
}

func (useCases *authUseCases) Status(ctx context.Context) (authservice.StatusResult, error) {
	return useCases.status.Execute(ctx)
}

func (useCases *collateralUseCases) PlaceOrder(
	ctx context.Context,
	request collateralservice.PlaceOrderRequest,
) (collateralservice.PlaceOrderResult, error) {
	return useCases.placeOrder.Execute(ctx, request)
}
