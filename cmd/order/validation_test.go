package ordercmd

import "testing"

func TestValidatePositiveFloatFlag(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		value     float64
		wantError string
	}{
		{name: "valid", flag: "--amount", value: 1},
		{name: "zero", flag: "--amount", value: 0, wantError: "--amount must be greater than 0"},
		{name: "negative", flag: "--price", value: -1, wantError: "--price must be greater than 0"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := validatePositiveFloatFlag(testCase.flag, testCase.value)
			if testCase.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %q, got nil", testCase.wantError)
			}
			if err.Error() != testCase.wantError {
				t.Fatalf("unexpected error. got %q want %q", err.Error(), testCase.wantError)
			}
		})
	}
}

func TestValidateNonNegativeInt64Flag(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		value     int64
		wantError string
	}{
		{name: "valid", flag: "--expiration", value: 0},
		{name: "positive", flag: "--expiration", value: 10},
		{name: "negative", flag: "--expiration", value: -1, wantError: "--expiration must be greater than or equal to 0"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateNonNegativeInt64Flag(testCase.flag, testCase.value)
			if testCase.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %q, got nil", testCase.wantError)
			}
			if err.Error() != testCase.wantError {
				t.Fatalf("unexpected error. got %q want %q", err.Error(), testCase.wantError)
			}
		})
	}
}

func TestValidateBase(t *testing.T) {
	tests := []struct {
		name      string
		options   baseOptions
		wantError string
	}{
		{
			name: "valid",
			options: baseOptions{
				Market: "BTC_PERP",
				Side:   "buy",
			},
		},
		{
			name: "valid unknown side passes base validation",
			options: baseOptions{
				Market: "BTC_PERP",
				Side:   "HOLD",
			},
		},
		{
			name: "missing market",
			options: baseOptions{
				Side: "buy",
			},
			wantError: "--market is required",
		},
		{
			name: "missing side",
			options: baseOptions{
				Market: "BTC_PERP",
			},
			wantError: "--side is required",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateBase(testCase.options)
			if testCase.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %q, got nil", testCase.wantError)
			}
			if err.Error() != testCase.wantError {
				t.Fatalf("unexpected error. got %q want %q", err.Error(), testCase.wantError)
			}
		})
	}
}

func TestNormalizeSideAlias(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSide string
		wantOK   bool
	}{
		{name: "buy", input: "buy", wantSide: "buy", wantOK: true},
		{name: "long alias", input: "long", wantSide: "long", wantOK: true},
		{name: "sell", input: "sell", wantSide: "sell", wantOK: true},
		{name: "short alias", input: "short", wantSide: "short", wantOK: true},
		{name: "case insensitive", input: "LoNg", wantSide: "long", wantOK: true},
		{name: "unsupported", input: "hold", wantSide: "", wantOK: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			gotSide, gotOK := normalizeSideAlias(testCase.input)
			if gotSide != testCase.wantSide || gotOK != testCase.wantOK {
				t.Fatalf("unexpected normalize result: got=(%q,%t), want=(%q,%t)", gotSide, gotOK, testCase.wantSide, testCase.wantOK)
			}
		})
	}
}

func TestValidateRequiredStringFlag(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		value     string
		wantError string
	}{
		{name: "valid", flag: "--amount", value: "0.1"},
		{name: "empty", flag: "--price", value: "", wantError: "--price is required"},
		{name: "spaces", flag: "--market", value: "   ", wantError: "--market is required"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateRequiredStringFlag(testCase.flag, testCase.value)
			if testCase.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil || err.Error() != testCase.wantError {
				t.Fatalf("unexpected error. got %v want %q", err, testCase.wantError)
			}
		})
	}
}

func TestNormalizeOutputMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMode string
		wantOK   bool
	}{
		{name: "default empty", input: "", wantMode: "table", wantOK: true},
		{name: "table", input: "table", wantMode: "table", wantOK: true},
		{name: "json", input: "json", wantMode: "json", wantOK: true},
		{name: "case insensitive", input: "JSON", wantMode: "json", wantOK: true},
		{name: "invalid", input: "yaml", wantMode: "", wantOK: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			gotMode, gotOK := normalizeOutputMode(testCase.input)
			if gotMode != testCase.wantMode || gotOK != testCase.wantOK {
				t.Fatalf("unexpected normalize result: got=(%q,%t), want=(%q,%t)", gotMode, gotOK, testCase.wantMode, testCase.wantOK)
			}
		})
	}
}

func TestValidateAmountMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantError string
	}{
		{name: "constant", mode: "constant"},
		{name: "arithmetic", mode: "arithmetic"},
		{name: "geometric", mode: "geometric"},
		{name: "capped geometric", mode: "capped-geometric"},
		{name: "fibonacci", mode: "fibonacci"},
		{name: "custom list", mode: "custom-list"},
		{name: "unsupported", mode: "random", wantError: amountModeErrorMessage},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateAmountMode(testCase.mode)
			if testCase.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %q, got nil", testCase.wantError)
			}
			if err.Error() != testCase.wantError {
				t.Fatalf("unexpected error. got %q want %q", err.Error(), testCase.wantError)
			}
		})
	}
}
