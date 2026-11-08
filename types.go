// Package equity calculates equity plan transactions using caller-supplied tax and broker policies.
package equitrade

import "time"

// GrantType distinguishes option grant treatment for tax/solver purposes.
type GrantType string

const (
	NQOGrantType GrantType = "NQO"
	ISOGrantType GrantType = "ISO"
)

// Config contains the caller's payroll and broker policy.
type Config struct {
	TaxRates   TaxRates   `json:"taxRates"`
	TaxModes   TaxModes   `json:"taxModes"`
	BrokerFees BrokerFees `json:"brokerFees"`
}

// TaxRates holds decimal rates or dollar amounts as selected by TaxModes.
type TaxRates struct {
	Federal   float64 `json:"federal"`
	Medicare  float64 `json:"medicare"`
	SocialSec float64 `json:"socialSec"`
	State     float64 `json:"state"`
	LocalSDI  float64 `json:"localSdi"`
}

// TaxModes selects TaxPercent or TaxFixed independently for each component.
type TaxModes struct {
	Federal   string `json:"federal"`
	Medicare  string `json:"medicare"`
	SocialSec string `json:"socialSec"`
	State     string `json:"state"`
	LocalSDI  string `json:"localSdi"`
}

// BrokerFees applies to one sale. CommissionRate is dollars per share and
// SECFeeRate is a decimal rate on gross proceeds.
type BrokerFees struct {
	CommissionRate float64 `json:"commissionRate"`
	MinimumFee     float64 `json:"minimumFee"`
	FlatFee        float64 `json:"flatFee"`
	SECFeeRate     float64 `json:"secFee"`
}

// OptionsInput describes a sell-to-cover option exercise. A zero SalePrice uses FMV for planning.
// The zero GrantType is NQO.
type OptionsInput struct {
	ExercisePrice   float64   `json:"exercisePrice"`
	ExercisedShares float64   `json:"exercisedShares"`
	FMV             float64   `json:"fmv"`
	SalePrice       float64   `json:"salePrice,omitempty"`
	GrantType       GrantType `json:"grantType,omitempty"`
}

// RSUInput describes a sell-to-cover release. VestPrice drives compensation
// income and SalePrice drives proceeds.
type RSUInput struct {
	SharesReleased float64 `json:"sharesReleased"`
	VestPrice      float64 `json:"vestPrice"`
	SalePrice      float64 `json:"salePrice"`
}

// OptionsResult reports an option exercise and its sell-to-cover sale.
type OptionsResult struct {
	ExercisePrice   float64   `json:"exercisePrice"`
	ExercisedShares float64   `json:"exercisedShares"`
	FMV             float64   `json:"fmv"`
	SalePrice       float64   `json:"salePrice"`
	GrantType       GrantType `json:"grantType"`

	OptionCost  float64      `json:"optionCost"`
	TaxableGain float64      `json:"taxableGain"`
	Tax         TaxBreakdown `json:"tax"`
	Fees        FeeBreakdown `json:"fees"`

	TotalCosts       float64 `json:"totalCosts"`
	SharesToSell     float64 `json:"sharesToSell"`
	EstGrossProceeds float64 `json:"estGrossProceeds"`
	Residual         float64 `json:"residual"`
	NetShares        float64 `json:"netShares"`
}

// RSUResult is the full output of an RSU STC calculation.
type RSUResult struct {
	SharesReleased float64 `json:"sharesReleased"`
	VestPrice      float64 `json:"vestPrice"`
	SalePrice      float64 `json:"salePrice"`

	TaxableGain float64      `json:"taxableGain"`
	Tax         TaxBreakdown `json:"tax"`
	Fees        FeeBreakdown `json:"fees"`

	TotalCosts       float64 `json:"totalCosts"`
	SharesToSell     float64 `json:"sharesToSell"`
	EstGrossProceeds float64 `json:"estGrossProceeds"`
	Residual         float64 `json:"residual"`
	NetShares        float64 `json:"netShares"`
}

// AuditStep records a calculation rule and its evaluated value.
type AuditStep struct {
	Description string `json:"description"`
	Formula     string `json:"formula"`
	Value       string `json:"value"`
}

// AuditableResult pairs the calculated result with a timestamp and audit steps.
type AuditableResult[T any] struct {
	Timestamp  time.Time   `json:"timestamp"`
	Result     T           `json:"result"`
	AuditTrail []AuditStep `json:"auditTrail"`
}
