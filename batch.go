package equitrade

import "math/big"

// OptionsRow is one batch outcome. Err is set when that row fails.
type OptionsRow struct {
	Result OptionsResult
	Err    error
}

// OptionsBatch preserves input order, including failed rows.
type OptionsBatch []OptionsRow

// CalculateOptionsBatch evaluates independent exercises under the same policy.
func (c *Calculator) CalculateOptionsBatch(inputs []OptionsInput) OptionsBatch {
	batch := make(OptionsBatch, len(inputs))
	for i, input := range inputs {
		calculated, err := c.CalculateOptions(input)
		batch[i] = OptionsRow{Result: calculated.Result, Err: err}
	}
	return batch
}

// OptionsSummary aggregates successful rows. Failed rows contribute only to
// FailedCount. AverageFMV is the unweighted average across successful rows.
type OptionsSummary struct {
	Count                int
	FailedCount          int
	TotalExercisedShares float64
	TotalSharesToSell    float64
	TotalNetShares       float64
	TotalCosts           float64
	TotalTaxes           float64
	TotalFees            float64
	AverageFMV           float64
}

// Summarize aggregates the successful rows of an options batch.
func (batch OptionsBatch) Summarize() OptionsSummary {
	var summary OptionsSummary
	var exercised, sold, retained, fmv, costs, taxes, fees big.Rat
	for _, row := range batch {
		if row.Err != nil {
			summary.FailedCount++
			continue
		}
		r := row.Result
		summary.Count++
		exercised.Add(&exercised, decimal(r.ExercisedShares))
		sold.Add(&sold, decimal(r.SharesToSell))
		retained.Add(&retained, decimal(r.NetShares))
		fmv.Add(&fmv, decimal(r.FMV))
		costs.Add(&costs, decimal(r.TotalCosts))
		taxes.Add(&taxes, decimal(r.Tax.Total))
		fees.Add(&fees, decimal(r.Fees.Total))
	}
	if summary.Count > 0 {
		fmv.Quo(&fmv, big.NewRat(int64(summary.Count), 1))
	}
	summary.AverageFMV, _ = fmv.Float64()
	summary.TotalExercisedShares, _ = exercised.Float64()
	summary.TotalSharesToSell, _ = sold.Float64()
	summary.TotalNetShares, _ = retained.Float64()
	summary.TotalCosts, _ = costs.Float64()
	summary.TotalTaxes, _ = taxes.Float64()
	summary.TotalFees, _ = fees.Float64()
	return summary
}
