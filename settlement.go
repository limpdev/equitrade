package equitrade

import (
	"errors"
	"fmt"
	"math"
	"math/big"
)

// ErrSearchLimit indicates that a fee or tax policy made the exact share
// search too expensive. An actual fill can still be verified.
var ErrSearchLimit = errors.New("sell-to-cover search limit reached")

// VerifyResult reconciles a given fill against exercise cost, withholding, and
// broker fees. A negative Delta is a cash shortfall.
type VerifyResult struct {
	Sufficient    bool         `json:"sufficient"`
	SharesSold    float64      `json:"sharesSold"`
	NetShares     float64      `json:"netShares"`
	Proceeds      float64      `json:"proceeds"`
	OptionCost    float64      `json:"optionCost"`
	TaxableGain   float64      `json:"taxableGain"`
	Tax           TaxBreakdown `json:"tax"`
	BaseLiability float64      `json:"baseLiability"`
	Fees          FeeBreakdown `json:"fees"`
	TotalRequired float64      `json:"totalRequired"`
	Delta         float64      `json:"delta"`
}

type trade struct {
	available    float64
	price        *big.Rat
	optionCost   cents
	compensation cents
	isoSpread    *big.Rat
	taxes        taxSchedule
	taxFormula   string
	fees         feeSchedule
}

func newTrade(available, price float64, cfg Config, iso bool) (trade, error) {
	t := trade{available: available, price: decimal(price), fees: newFeeSchedule(cfg.BrokerFees)}
	shares := decimal(available)
	if err := validateNotional("gross sale proceeds", t.price, shares); err != nil {
		return trade{}, err
	}
	if err := validateNotional("commission", t.fees.commissionRate, shares); err != nil {
		return trade{}, err
	}
	t.taxes, t.taxFormula = newTaxSchedule(cfg, iso)
	return t, nil
}

type settlement struct {
	shares   float64
	gain     cents
	tax      taxAmounts
	fees     feeAmounts
	gross    cents
	required cents
}

func (s settlement) delta() cents { return s.gross - s.required }

func (t trade) settle(shares float64) settlement {
	quantity := decimal(shares)
	rawGross := multiply(quantity, t.price)
	s := settlement{
		shares: shares, gain: t.compensation,
		gross: roundMoney(rawGross), fees: t.fees.calculate(quantity, rawGross),
	}
	if t.isoSpread != nil {
		s.gain = roundMoney(multiply(t.isoSpread, quantity))
	}
	s.tax = t.taxes.calculate(s.gain)
	s.required = t.optionCost + s.tax.total() + s.fees.total()
	return s
}

func (t trade) verify(shares float64) (VerifyResult, error) {
	if err := validateAmount("sold shares", shares, false); err != nil {
		return VerifyResult{}, err
	}
	if shares > t.available {
		return VerifyResult{}, fmt.Errorf("sold shares exceed available shares")
	}
	s := t.settle(shares)
	return VerifyResult{
		Sufficient: s.delta() >= 0,
		SharesSold: shares, NetShares: t.netShares(shares), Proceeds: s.gross.dollars(),
		OptionCost: t.optionCost.dollars(), TaxableGain: s.gain.dollars(), Tax: s.tax.breakdown(),
		BaseLiability: (t.optionCost + s.tax.total()).dollars(),
		Fees:          s.fees.breakdown(), TotalRequired: s.required.dollars(), Delta: s.delta().dollars(),
	}, nil
}

func (t trade) netShares(sold float64) float64 {
	net, _ := new(big.Rat).Sub(decimal(t.available), decimal(sold)).Float64()
	return net
}

// positiveMarginIsMonotone bounds how much the rounded line items can
// increase in one share. It applies between positive share counts, where the
// flat fee and the commission minimum cannot introduce an initial jump.
func (t trade) positiveMarginIsMonotone() bool {
	costStep := quantize(scaled(t.fees.commissionRate), true) +
		quantize(scaled(multiply(t.price, t.fees.secRate)), true)
	if t.isoSpread != nil {
		gainStep := big.NewRat(quantize(scaled(t.isoSpread), true), 1)
		for _, rule := range t.taxes {
			if rule.rate != nil {
				costStep += quantize(multiply(gainStep, rule.rate), true)
			}
		}
	}
	grossStep := scaled(t.price)
	return new(big.Rat).SetInt64(costStep).Cmp(grossStep) <= 0
}

// solve uses the same rounded settlement ledger as verify. Prefer the first
// covering whole share count. If none covers, evaluate liquidation at the
// exact available quantity, which can include a fractional share.
func (t trade) solve() (settlement, error) {
	zero := t.settle(0)
	if zero.delta() >= 0 {
		return zero, nil
	}

	limit := int64(math.Floor(t.available))
	if limit == 0 {
		return t.settle(t.available), nil
	}
	if t.positiveMarginIsMonotone() {
		low, high := int64(1), limit+1
		for low < high {
			mid := low + (high-low)/2
			if t.settle(float64(mid)).delta() >= 0 {
				high = mid
			} else {
				low = mid + 1
			}
		}
		if low <= limit {
			return t.settle(float64(low)), nil
		}
		return t.settle(t.available), nil
	}

	// All required costs are nondecreasing, even if the net margin is not.
	// Skip every share count whose proceeds cannot meet the current cost.
	// Half a cent below required is the first gross that rounds up to it.
	current := zero
	const maxEvaluations = 10000
	for range maxEvaluations {
		threshold := big.NewRat(2*int64(current.required)-1, 200)
		next := new(big.Rat).Quo(threshold, t.price)
		if next.Cmp(big.NewRat(limit, 1)) > 0 {
			return t.settle(t.available), nil
		}
		current = t.settle(float64(quantize(next, true)))
		if current.delta() >= 0 {
			return current, nil
		}
	}
	return settlement{}, ErrSearchLimit
}
