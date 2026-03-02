package ordercmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	appcontainer "github.com/ChewX3D/wbcli/internal/app/application"
	collateralservice "github.com/ChewX3D/wbcli/internal/app/services/collateral"
	"github.com/spf13/cobra"
)

type placeOptions struct {
	baseOptions
	Amount        string
	Price         string
	ClientOrderID string
	Output        string
}

func newPlaceCmd(getApplication func() (*appcontainer.Application, error)) *cobra.Command {
	options := &placeOptions{}

	command := &cobra.Command{
		Use:   "place",
		Short: "Place a single collateral limit order",
		Long: "Place one collateral limit order through WhiteBIT signed API using current single-session credentials.\n" +
			"Supported side values are `buy`, `sell`, `long`, `short`.\n" +
			"Order submission always enforces `postOnly=true`.",
		Example: `  # canonical side value
  wbcli collateral order place --market BTC_PERP --side buy --amount 0.01 --price 50000

  # alias side value
  wbcli collateral order place --market BTC_PERP --side short --amount 0.02 --price 51000

  # with client order id pass-through
  wbcli collateral order place --market BTC_PERP --side long --amount 0.01 --price 49950 --client-order-id bot-001

  # machine-readable output
  wbcli collateral order place --market BTC_PERP --side sell --amount 0.03 --price 52000 --output json`,
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateBase(options.baseOptions); err != nil {
				return err
			}
			if err := validateRequiredStringFlag("--amount", options.Amount); err != nil {
				return err
			}
			if err := validateRequiredStringFlag("--price", options.Price); err != nil {
				return err
			}

			side, ok := normalizeSideAlias(options.Side)
			if !ok {
				return errors.New("--side must be one of: buy, long, sell, short")
			}

			outputMode, ok := normalizeOutputMode(options.Output)
			if !ok {
				return errors.New("--output must be one of: table, json")
			}

			return runWithApplication(command, getApplication, func(application *appcontainer.Application) error {
				if application.Collateral == nil {
					return errors.New("collateral order service is not configured")
				}

				result, err := application.Collateral.PlaceOrder(command.Context(), collateralservice.PlaceOrderRequest{
					Market:        options.Market,
					Side:          side,
					Amount:        options.Amount,
					Price:         options.Price,
					ClientOrderID: options.ClientOrderID,
				})
				if err != nil {
					return err
				}

				return renderPlaceOutput(command.OutOrStdout(), outputMode, result)
			})
		},
	}

	addBaseFlags(command, &options.baseOptions)
	command.Flags().StringVar(&options.Amount, "amount", "", "order amount as string accepted by WhiteBIT")
	command.Flags().StringVar(&options.Price, "price", "", "limit price as string accepted by WhiteBIT")
	command.Flags().StringVar(&options.ClientOrderID, "client-order-id", "", "client order id (pass-through)")
	command.Flags().StringVar(&options.Output, "output", "table", "output format: table|json")

	return command
}

func renderPlaceOutput(writer io.Writer, outputMode string, result collateralservice.PlaceOrderResult) error {
	if outputMode == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)

		return encoder.Encode(result)
	}

	_, err := fmt.Fprintf(
		writer,
		"request_id=%s mode=%s orders_planned=%d orders_submitted=%d orders_failed=%d errors=%s\n",
		result.RequestID,
		result.Mode,
		result.OrdersPlanned,
		result.OrdersSubmitted,
		result.OrdersFailed,
		renderErrors(result.Errors),
	)
	return err
}

func renderErrors(values []string) string {
	if len(values) == 0 {
		return "[]"
	}

	return "[" + strings.Join(values, ",") + "]"
}
