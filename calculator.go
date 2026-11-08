package equitrade

import (
	"fmt"
	"math"
	"math/big"
	"time"
)

func (c *Calculator) optionsTrade(input OptionsInput, verify bool) (trade, GrantType, float64, error) {
	grant := input.GrantType
	if grant == "" {
		grant = NQOGrantType
	}
	if grant != NQOGrantType && grant != ISOGrantType {
		return trade{}, "", 0, fmt.Errorf("unknown grant type %q", grant)
	}
	for _, field := range []struct {
		name     string
		value    float64
		positive bool
	}{
		{"exercised shares", input.ExercisedShares, true},
		{"FMV", input.FMV, true},
		{"exercise price", input.ExercisePrice, false},
		{"sale price", input.SalePrice, verify},
	} {
		if err := validateAmount(field.name, field.value, field.positive); err != nil {
			return trade{}, "", 0, err
		}
	}
	if input.ExercisePrice >= input.FMV {
		return trade{}, "", 0, fmt.Errorf("exercise price must be below FMV for a sell-to-cover exercise")
	}

	salePrice := input.SalePrice
	if salePrice == 0 {
		salePrice = input.FMV
	}
	t, err := newTrade(input.ExercisedShares, salePrice, c.config, grant == ISOGrantType)
	if err != nil {
		return trade{}, "", 0, err
	}

	shares, strike, fmv := decimal(input.ExercisedShares), decimal(input.ExercisePrice), decimal(input.FMV)
	if err := validateNotional("exercise FMV", fmv, shares); err != nil {
		return trade{}, "", 0, err
	}
	t.optionCost = roundMoney(multiply(shares, strike))
	if grant == ISOGrantType {
		t.isoSpread = new(big.Rat).Sub(decimal(min(input.FMV, salePrice)), strike)
		if t.isoSpread.Sign() < 0 {
			t.isoSpread.SetInt64(0)
		}
	} else {
		t.compensation = roundMoney(multiply(shares, new(big.Rat).Sub(fmv, strike)))
	}
	return t, grant, salePrice, nil
}

// CalculateOptions plans an NQO exercise or a same-transaction ISO exercise
// and disqualifying sale. For ISO, the estimate assumes a sale in which a loss
// could be recognized and uses the gross sale price for the income cap.
func (c *Calculator) CalculateOptions(input OptionsInput) (AuditableResult[OptionsResult], error) {
	t, grant, salePrice, err := c.optionsTrade(input, false)
	if err != nil {
		return AuditableResult[OptionsResult]{}, err
	}
	s, err := t.solve()
	if err != nil {
		return AuditableResult[OptionsResult]{}, err
	}
	result := OptionsResult{
		ExercisePrice: input.ExercisePrice, ExercisedShares: input.ExercisedShares,
		FMV: input.FMV, SalePrice: salePrice, GrantType: grant,
		OptionCost: t.optionCost.dollars(), TaxableGain: s.gain.dollars(),
		Tax: s.tax.breakdown(), Fees: s.fees.breakdown(), TotalCosts: s.required.dollars(),
		SharesToSell: s.shares, EstGrossProceeds: s.gross.dollars(),
		Residual: s.delta().dollars(), NetShares: t.netShares(s.shares),
	}
	gainFormula := fmt.Sprintf("($%g exercise FMV − $%g strike) × %g exercised", input.FMV, input.ExercisePrice, input.ExercisedShares)
	if grant == ISOGrantType {
		gainFormula = fmt.Sprintf("Max(0, Min($%g exercise FMV, $%g gross sale price) − $%g strike) × %g sold",
			input.FMV, salePrice, input.ExercisePrice, s.shares)
	}
	audit := []AuditStep{
		{"Option Cost (Strike)", fmt.Sprintf("%g exercised × $%g strike", input.ExercisedShares, input.ExercisePrice), fmt.Sprintf("$%.2f", result.OptionCost)},
		{"Compensation Income", gainFormula, fmt.Sprintf("$%.2f", result.TaxableGain)},
	}
	return AuditableResult[OptionsResult]{time.Now(), result, append(audit, t.audit(s)...)}, nil
}

// VerifyOptions reconciles actual sold shares and an explicit fill price.
// An adverse fill can produce a shortfall even at full liquidation.
func (c *Calculator) VerifyOptions(input OptionsInput, soldShares float64) (VerifyResult, error) {
	t, _, _, err := c.optionsTrade(input, true)
	if err != nil {
		return VerifyResult{}, err
	}
	return t.verify(soldShares)
}

func (c *Calculator) rsuTrade(input RSUInput) (trade, error) {
	for _, field := range []struct {
		name  string
		value float64
	}{
		{"shares released", input.SharesReleased},
		{"vest price", input.VestPrice},
		{"sale price", input.SalePrice},
	} {
		if err := validateAmount(field.name, field.value, true); err != nil {
			return trade{}, err
		}
	}
	t, err := newTrade(input.SharesReleased, input.SalePrice, c.config, false)
	if err != nil {
		return trade{}, err
	}
	vestPrice, shares := decimal(input.VestPrice), decimal(input.SharesReleased)
	if err := validateNotional("vest FMV", vestPrice, shares); err != nil {
		return trade{}, err
	}
	t.compensation = roundMoney(multiply(vestPrice, shares))
	return t, nil
}

// CalculateRSU plans a sale to fund withholding on an RSU release.
func (c *Calculator) CalculateRSU(input RSUInput) (AuditableResult[RSUResult], error) {
	t, err := c.rsuTrade(input)
	if err != nil {
		return AuditableResult[RSUResult]{}, err
	}
	s, err := t.solve()
	if err != nil {
		return AuditableResult[RSUResult]{}, err
	}
	result := RSUResult{
		SharesReleased: input.SharesReleased, VestPrice: input.VestPrice, SalePrice: input.SalePrice,
		TaxableGain: s.gain.dollars(), Tax: s.tax.breakdown(), Fees: s.fees.breakdown(),
		TotalCosts: s.required.dollars(), SharesToSell: s.shares, EstGrossProceeds: s.gross.dollars(),
		Residual: s.delta().dollars(), NetShares: t.netShares(s.shares),
	}
	audit := []AuditStep{
		{"Compensation Income", fmt.Sprintf("%g released × $%g vest FMV", input.SharesReleased, input.VestPrice), fmt.Sprintf("$%.2f", result.TaxableGain)},
	}
	return AuditableResult[RSUResult]{time.Now(), result, append(audit, t.audit(s)...)}, nil
}

// VerifyRSU reconciles actual sold shares and an explicit fill price.
func (c *Calculator) VerifyRSU(input RSUInput, soldShares float64) (VerifyResult, error) {
	t, err := c.rsuTrade(input)
	if err != nil {
		return VerifyResult{}, err
	}
	return t.verify(soldShares)
}

func (t trade) audit(s settlement) []AuditStep {
	shareRule := "First whole share count whose proceeds cover the obligation"
	switch {
	case s.delta() < 0:
		shareRule = "Available shares are insufficient to cover the obligation"
	case s.shares != math.Trunc(s.shares):
		shareRule = "Full liquidation including the fractional share"
	}
	return []AuditStep{
		{"Estimated Withholding", t.taxFormula, fmt.Sprintf("$%.2f", s.tax.total().dollars())},
		{"Broker Fees", "Commission (subject to minimum) + flat fee + SEC levy", fmt.Sprintf("$%.2f", s.fees.total().dollars())},
		{"Shares to Sell", shareRule, fmt.Sprintf("%g shares", s.shares)},
		{"Residual Cash", "Gross proceeds − exercise cost − withholding − broker fees", fmt.Sprintf("$%.2f", s.delta().dollars())},
		{"Net Shares", fmt.Sprintf("%g available − %g sold", t.available, s.shares), fmt.Sprintf("%g shares", t.netShares(s.shares))},
	}
}
