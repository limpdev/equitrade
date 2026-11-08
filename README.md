# Equity Plan Calculations

Import `planq/equity` from this module. The package uses the Go standard library and takes all prices, tax inputs, and broker fees from the caller. A calculator snapshots its configuration at construction and can be shared across goroutines.

```go
import "planq/equity"

func planExercise() (equity.OptionsResult, error) {
	calc, err := equity.NewCalculator(equity.Config{
		TaxRates: equity.TaxRates{Federal: 0.22, State: 12.50},
		TaxModes: equity.TaxModes{State: equity.TaxFixed},
		BrokerFees: equity.BrokerFees{
			CommissionRate: 0.01,
			MinimumFee:     1.00,
		},
	})
	if err != nil {
		return equity.OptionsResult{}, err
	}
	plan, err := calc.CalculateOptions(equity.OptionsInput{
		GrantType:       equity.NQOGrantType,
		ExercisePrice:   10,
		ExercisedShares: 100,
		FMV:             25,
		SalePrice:       24.80,
	})
	return plan.Result, err
}
```

The configuration above is illustrative. Use decimal rates in `TaxPercent` mode and dollar amounts in `TaxFixed` mode. An omitted mode means `TaxPercent`. Fixed amounts apply as cash obligations even if the percentage-based compensation amount is zero.

| Operation | Calculation |
| --- | --- |
| `CalculateOptions` with NQO | Exercise cost for all exercised shares plus withholding on the exercise FMV spread for all exercised shares. |
| `CalculateOptions` with ISO | Exercise cost for all exercised shares plus configured state and local amounts. The same-transaction disqualifying-sale estimate uses the positive spread between the lesser of exercise FMV and gross sale price and the strike, multiplied by the shares sold. |
| `CalculateRSU` | Withholding on shares released multiplied by vest FMV. The sale price drives cash proceeds. |
| `VerifyOptions`, `VerifyRSU` | Reconcile a known share count at an explicit fill price with the same tax and fee calculation used to plan the trade. Use zero sold shares to see an unfunded obligation and the full available quantity to model a sell-all. |
| `CalculateWithholding` | Apply the configured payroll schedule to a known compensation amount. |
| `CalculateSale` | Calculate gross and net proceeds, fees, and signed capital gain or loss for a sale with a caller-supplied total adjusted basis. |
| `CalculateOptionsBatch` | Keep results and errors in input order. Summary totals and average FMV use successful rows. |

### Pricing and Rounding

Options planning uses exercise FMV when the sale price is omitted. Verification requires a fill price. For RSUs, both planning and verification require a sale price. A fill below strike can produce a cash shortfall.

The solver finds the first whole-share count that covers the obligation using the actual cent-rounded ledger. If whole shares cannot cover it, it evaluates full liquidation at the available share count, including any fractional remainder. A negative `Residual` or `Delta` is a cash shortfall. A zero-share sale has zero broker fees. An unusually expensive exact search returns `ErrSearchLimit`.

The API takes and returns `float64` to match PlanQ’s existing JSON payloads. Inputs are interpreted as their shortest round-trippable decimals. Internally, compensation, exercise cost, gross proceeds, and individual withholding and fee lines round to the nearest cent, with half-cent ties rounded up. The SEC levy rounds up from unrounded gross proceeds. Transaction inputs and per-leg notionals are bounded at one trillion to preserve cent resolution.

### US Tax Scope

ISO sell-to-cover assumes vested shares and a disqualifying sale in which a loss would be recognizable. The income cap uses gross sale price and estimates fees separately. Federal income withholding, Medicare, and Social Security are excluded on the ISO path. [IRS Publication 15-B](https://www.irs.gov/publications/p15b), [IRS Publication 525](https://www.irs.gov/publications/p525), [26 CFR § 1.422-1](https://www.govinfo.gov/content/pkg/CFR-2024-title26-vol7/pdf/CFR-2024-title26-vol7-sec1-422-1.pdf).

Payroll thresholds, wage bases, AMT, qualifying dispositions, lot selection, and filing rules sit outside the package. Supply the applicable rates, broker fee schedule, payroll-calculated fixed amounts, and adjusted sale basis for your workflow.

