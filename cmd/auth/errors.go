package authcmd

import (
	"errors"

	"github.com/ChewX3D/wbcli/internal/app/ports"
	clitools "github.com/ChewX3D/wbcli/internal/cli"
	domainauth "github.com/ChewX3D/wbcli/internal/domain/auth"
)

type staticAuthErrorRule struct {
	match   error
	message string
}

var staticAuthErrorRules = []staticAuthErrorRule{
	{match: domainauth.ErrAPIKeyRequired, message: "api key is required in stdin payload"},
	{match: domainauth.ErrAPISecretRequired, message: "api secret is required in stdin payload"},
	{match: clitools.ErrCredentialInputMissing, message: "stdin credentials are required: first line API key, second line API secret"},
	{match: clitools.ErrCredentialInputTooLarge, message: "stdin credential payload exceeds maximum allowed size"},
	{match: clitools.ErrCredentialInputFormat, message: "stdin credential payload must contain exactly two non-empty lines: api_key then api_secret"},
	{match: ports.ErrCredentialNotFound, message: "not logged in; run wbcli auth login first"},
	{match: ports.ErrSecretStoreUnavailable, message: "os-keychain backend is unavailable on this system; install/unlock keychain backend and retry"},
	{match: ports.ErrSecretStorePermissionDenied, message: "os-keychain access denied; keychain is locked or access is restricted"},
}

func mapError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *ports.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}

	for _, rule := range staticAuthErrorRules {
		if errors.Is(err, rule.match) {
			return errors.New(rule.message)
		}
	}

	return err
}
