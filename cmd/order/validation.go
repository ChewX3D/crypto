package ordercmd

import (
	"errors"
	"fmt"
	"strings"
)

const amountModeErrorMessage = "--amount-mode must be one of: constant, arithmetic, geometric, capped-geometric, fibonacci, custom-list"

var supportedAmountModes = map[string]struct{}{
	"constant":         {},
	"arithmetic":       {},
	"geometric":        {},
	"capped-geometric": {},
	"fibonacci":        {},
	"custom-list":      {},
}

func validatePositiveFloatFlag(flagName string, value float64) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", flagName)
	}

	return nil
}

func validateNonNegativeInt64Flag(flagName string, value int64) error {
	if value < 0 {
		return fmt.Errorf("%s must be greater than or equal to 0", flagName)
	}

	return nil
}

func validateBase(options baseOptions) error {
	if strings.TrimSpace(options.Market) == "" {
		return errors.New("--market is required")
	}

	side := strings.TrimSpace(options.Side)
	if side == "" {
		return errors.New("--side is required")
	}

	return nil
}

func normalizeSideAlias(side string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		return "buy", true
	case "long":
		return "long", true
	case "sell":
		return "sell", true
	case "short":
		return "short", true
	default:
		return "", false
	}
}

func validateRequiredStringFlag(flagName string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", flagName)
	}

	return nil
}

func normalizeOutputMode(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "table":
		return "table", true
	case "json":
		return "json", true
	default:
		return "", false
	}
}

func validateAmountMode(mode string) error {
	if _, ok := supportedAmountModes[mode]; ok {
		return nil
	}

	return errors.New(amountModeErrorMessage)
}
