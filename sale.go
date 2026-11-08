package equitrade

// SaleInput describes one sale of previously acquired shares. AdjustedBasis
// is the total basis allocated to the shares sold, in dollars.
type SaleInput struct {
	SharesSold    float64 `json:"sharesSold"`
	SalePrice     float64 `json:"salePrice"`
	AdjustedBasis float64 `json:"adjustedBasis"`
}

// SaleResult reports the cash proceeds and signed capital gain or loss.
type SaleResult struct {
	GrossProceeds   float64      `json:"grossProceeds"`
	Fees            FeeBreakdown `json:"fees"`
	NetProceeds     float64      `json:"netProceeds"`
	AdjustedBasis   float64      `json:"adjustedBasis"`
	CapitalGainLoss float64      `json:"capitalGainLoss"`
}

// CalculateSale deducts configured selling fees and caller-supplied adjusted
// basis from a sale. It does not infer lots, holding periods, or capital gains
// taxes.
func (c *Calculator) CalculateSale(input SaleInput) (SaleResult, error) {
	for _, field := range []struct {
		name     string
		value    float64
		positive bool
	}{
		{"sold shares", input.SharesSold, true},
		{"sale price", input.SalePrice, true},
		{"adjusted basis", input.AdjustedBasis, false},
	} {
		if err := validateAmount(field.name, field.value, field.positive); err != nil {
			return SaleResult{}, err
		}
	}
	shares, price := decimal(input.SharesSold), decimal(input.SalePrice)
	schedule := newFeeSchedule(c.config.BrokerFees)
	if err := validateNotional("gross sale proceeds", price, shares); err != nil {
		return SaleResult{}, err
	}
	if err := validateNotional("commission", schedule.commissionRate, shares); err != nil {
		return SaleResult{}, err
	}
	rawGross := multiply(shares, price)
	gross := roundMoney(rawGross)
	fees := schedule.calculate(shares, rawGross)
	net := gross - fees.total()
	basis := roundMoney(decimal(input.AdjustedBasis))
	return SaleResult{
		GrossProceeds: gross.dollars(), Fees: fees.breakdown(), NetProceeds: net.dollars(),
		AdjustedBasis: basis.dollars(), CapitalGainLoss: (net - basis).dollars(),
	}, nil
}
