package equitrade

import (
	"fmt"
	"math/big"
	"strings"
)

const (
	// TaxPercent applies a decimal rate to compensation income.
	TaxPercent = "pct"
	// TaxFixed applies a known cash obligation in dollars.
	TaxFixed = "fixed"
)

// Calculator applies a value copy of the supplied policy and is safe for
// concurrent use. Its zero value has zero tax and zero broker fees.
type Calculator struct {
	config Config
}

// NewCalculator validates the policy. Empty modes are treated as percentage
// modes for compatibility with older configurations.
func NewCalculator(config Config) (*Calculator, error) {
	for _, rule := range taxRules(config) {
		if err := validateAmount(rule.name, rule.value, false); err != nil {
			return nil, err
		}
		switch rule.mode {
		case "", TaxPercent:
			if rule.value > 1 {
				return nil, fmt.Errorf("%s rate must be between zero and one", rule.name)
			}
		case TaxFixed:
		default:
			return nil, fmt.Errorf("%s tax mode %q is invalid", rule.name, rule.mode)
		}
	}
	fees := config.BrokerFees
	for _, field := range []struct {
		name  string
		value float64
	}{
		{"commission rate", fees.CommissionRate},
		{"minimum commission", fees.MinimumFee},
		{"flat fee", fees.FlatFee},
		{"SEC fee rate", fees.SECFeeRate},
	} {
		if err := validateAmount(field.name, field.value, false); err != nil {
			return nil, err
		}
	}
	if fees.SECFeeRate > 1 {
		return nil, fmt.Errorf("SEC fee rate must be between zero and one")
	}
	return &Calculator{config: config}, nil
}

// Config returns a copy of the calculator's policy.
func (c *Calculator) Config() Config { return c.config }

type taxRule struct {
	name  string
	value float64
	mode  string
}

func taxRules(cfg Config) [5]taxRule {
	rates, modes := cfg.TaxRates, cfg.TaxModes
	return [5]taxRule{
		{"Federal", rates.Federal, modes.Federal},
		{"Medicare", rates.Medicare, modes.Medicare},
		{"Social Security", rates.SocialSec, modes.SocialSec},
		{"State", rates.State, modes.State},
		{"Local / SDI", rates.LocalSDI, modes.LocalSDI},
	}
}

type taxSchedule [5]struct {
	rate  *big.Rat
	fixed cents
}

func newTaxSchedule(cfg Config, iso bool) (taxSchedule, string) {
	var schedule taxSchedule
	var labels []string
	for i, rule := range taxRules(cfg) {
		// US ISO disqualifying dispositions exclude federal withholding and FICA.
		if iso && i < 3 {
			continue
		}
		if rule.mode == TaxFixed {
			schedule[i].fixed = roundMoney(decimal(rule.value))
			labels = append(labels, fmt.Sprintf("%s($%.2f)", rule.name, schedule[i].fixed.dollars()))
		} else {
			schedule[i].rate = decimal(rule.value)
			labels = append(labels, fmt.Sprintf("%s(%.4g%%)", rule.name, rule.value*100))
		}
	}
	return schedule, strings.Join(labels, " + ")
}

// TaxBreakdown is the estimated withholding by component.
type TaxBreakdown struct {
	Federal   float64 `json:"federal"`
	Medicare  float64 `json:"medicare"`
	SocialSec float64 `json:"socialSec"`
	State     float64 `json:"state"`
	LocalSDI  float64 `json:"localSdi"`
	Total     float64 `json:"total"`
}

type taxAmounts [5]cents

func (schedule taxSchedule) calculate(gain cents) taxAmounts {
	var result taxAmounts
	for i, rule := range schedule {
		result[i] = rule.fixed
		if rule.rate != nil {
			result[i] = cents(quantize(multiply(rule.rate, big.NewRat(int64(gain), 1)), false))
		}
	}
	return result
}

func (amounts taxAmounts) total() cents {
	var total cents
	for _, amount := range amounts {
		total += amount
	}
	return total
}

func (amounts taxAmounts) breakdown() TaxBreakdown {
	return TaxBreakdown{
		Federal: amounts[0].dollars(), Medicare: amounts[1].dollars(),
		SocialSec: amounts[2].dollars(), State: amounts[3].dollars(),
		LocalSDI: amounts[4].dollars(), Total: amounts.total().dollars(),
	}
}

// CalculateWithholding applies the configured payroll schedule to a
// caller-supplied nonnegative compensation amount.
func (c *Calculator) CalculateWithholding(compensation float64) (TaxBreakdown, error) {
	if err := validateAmount("compensation", compensation, false); err != nil {
		return TaxBreakdown{}, err
	}
	schedule, _ := newTaxSchedule(c.config, false)
	return schedule.calculate(roundMoney(decimal(compensation))).breakdown(), nil
}

type feeSchedule struct {
	commissionRate *big.Rat
	minimumFee     cents
	flatFee        cents
	secRate        *big.Rat
}

func newFeeSchedule(cfg BrokerFees) feeSchedule {
	return feeSchedule{
		commissionRate: decimal(cfg.CommissionRate),
		minimumFee:     roundMoney(decimal(cfg.MinimumFee)),
		flatFee:        roundMoney(decimal(cfg.FlatFee)),
		secRate:        decimal(cfg.SECFeeRate),
	}
}

// FeeBreakdown is the estimated broker and regulatory cost of one sale.
type FeeBreakdown struct {
	Commission float64 `json:"commission"`
	FlatFee    float64 `json:"flatFee"`
	SECLevy    float64 `json:"secLevy"`
	Total      float64 `json:"total"`
}

type feeAmounts struct {
	commission, flatFee, secLevy cents
}

func (schedule feeSchedule) calculate(shares, rawGross *big.Rat) feeAmounts {
	if shares.Sign() == 0 {
		return feeAmounts{}
	}
	return feeAmounts{
		commission: max(roundMoney(multiply(shares, schedule.commissionRate)), schedule.minimumFee),
		flatFee:    schedule.flatFee,
		secLevy:    cents(quantize(scaled(multiply(rawGross, schedule.secRate)), true)),
	}
}

func (fees feeAmounts) total() cents { return fees.commission + fees.flatFee + fees.secLevy }

func (fees feeAmounts) breakdown() FeeBreakdown {
	return FeeBreakdown{
		Commission: fees.commission.dollars(), FlatFee: fees.flatFee.dollars(),
		SECLevy: fees.secLevy.dollars(), Total: fees.total().dollars(),
	}
}
