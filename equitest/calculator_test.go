package equitest

import (
	"math"
	"testing"

	"github.com/limpdev/equitrade"
)

func newCalculator(t *testing.T, cfg equitrade.Config) *equitrade.Calculator {
	t.Helper()
	calc, err := equitrade.NewCalculator(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return calc
}

func TestNQOPlanMatchesVerification(t *testing.T) {
	calc := newCalculator(t, equitrade.Config{
		TaxRates:   equitrade.TaxRates{Federal: .22, State: 3.335},
		TaxModes:   equitrade.TaxModes{State: equitrade.TaxFixed},
		BrokerFees: equitrade.BrokerFees{CommissionRate: .04, MinimumFee: 3, FlatFee: 1.255, SECFeeRate: .0000206},
	})
	input := equitrade.OptionsInput{ExercisePrice: 10, ExercisedShares: 100, FMV: 25, SalePrice: 20}
	planned, err := calc.CalculateOptions(input)
	if err != nil {
		t.Fatal(err)
	}
	result := planned.Result
	if result.GrantType != equitrade.NQOGrantType || result.TaxableGain != 1500 ||
		result.Tax.Total != 333.34 || result.Fees.Total != 4.29 ||
		result.SharesToSell != 67 || result.Residual != 2.37 || result.NetShares != 33 {
		t.Fatalf("unexpected plan %+v", result)
	}
	verified, err := calc.VerifyOptions(input, result.SharesToSell)
	if err != nil || !verified.Sufficient || verified.TotalRequired != result.TotalCosts ||
		verified.Delta != result.Residual || verified.Fees != result.Fees || verified.Tax != result.Tax {
		t.Fatalf("verification = %+v, %v", verified, err)
	}
	fewer, err := calc.VerifyOptions(input, result.SharesToSell-1)
	if err != nil || fewer.Sufficient {
		t.Fatalf("one fewer share = %+v, %v", fewer, err)
	}
}

func TestISOCompensationAndPayroll(t *testing.T) {
	calc := newCalculator(t, equitrade.Config{
		TaxRates: equitrade.TaxRates{Federal: .22, Medicare: .0145, SocialSec: .062, State: .10},
	})
	for _, test := range []struct {
		name                                 string
		salePrice, sold, gain, tax, residual float64
	}{
		{"Above FMV", 30, 35, 350, 35, 15},
		{"Below FMV", 15, 69, 345, 34.50, .50},
		{"Below Strike", 9, 100, 0, 0, -100},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := equitrade.OptionsInput{
				GrantType: equitrade.ISOGrantType, ExercisePrice: 10, ExercisedShares: 100, FMV: 20, SalePrice: test.salePrice,
			}
			planned, err := calc.CalculateOptions(input)
			if err != nil {
				t.Fatal(err)
			}
			result := planned.Result
			if result.SharesToSell != test.sold || result.TaxableGain != test.gain ||
				result.Tax.Total != test.tax || result.Residual != test.residual {
				t.Fatalf("unexpected ISO plan %+v", result)
			}
			verified, err := calc.VerifyOptions(input, test.sold)
			if err != nil || verified.TaxableGain != test.gain || verified.Delta != test.residual {
				t.Fatalf("verification = %+v, %v", verified, err)
			}
		})
	}
}

func TestRSUFractionalLiquidation(t *testing.T) {
	calc := newCalculator(t, equitrade.Config{TaxRates: equitrade.TaxRates{Federal: .005}})
	input := equitrade.RSUInput{SharesReleased: 10.5, VestPrice: 10.05, SalePrice: .05}
	planned, err := calc.CalculateRSU(input)
	if err != nil {
		t.Fatal(err)
	}
	result := planned.Result
	if result.TaxableGain != 105.53 || result.Tax.Total != .53 || result.EstGrossProceeds != .53 ||
		result.SharesToSell != 10.5 || result.Residual != 0 || result.NetShares != 0 {
		t.Fatalf("unexpected plan %+v", result)
	}
}

func TestSolverChecksEarlierCoverageWhenMarginIsNonmonotone(t *testing.T) {
	calc := newCalculator(t, equitrade.Config{
		TaxRates:   equitrade.TaxRates{Federal: .01},
		TaxModes:   equitrade.TaxModes{Federal: equitrade.TaxFixed},
		BrokerFees: equitrade.BrokerFees{CommissionRate: .004},
	})
	input := equitrade.RSUInput{SharesReleased: 2, VestPrice: 1, SalePrice: .005}
	planned, err := calc.CalculateRSU(input)
	if err != nil || planned.Result.SharesToSell != 1 || planned.Result.Residual != 0 {
		t.Fatalf("plan = %+v, %v", planned.Result, err)
	}
	full, err := calc.VerifyRSU(input, 2)
	if err != nil || full.Sufficient || full.Delta != -.01 {
		t.Fatalf("full sale = %+v, %v", full, err)
	}
}

func TestZeroObligationDoesNotTriggerFees(t *testing.T) {
	calc := newCalculator(t, equitrade.Config{BrokerFees: equitrade.BrokerFees{FlatFee: 10, MinimumFee: 5}})
	input := equitrade.RSUInput{SharesReleased: 25, VestPrice: 10, SalePrice: 8}
	result, err := calc.CalculateRSU(input)
	if err != nil || result.Result.SharesToSell != 0 || result.Result.Fees.Total != 0 {
		t.Fatalf("plan = %+v, %v", result.Result, err)
	}
	verified, err := calc.VerifyRSU(input, 0)
	if err != nil || !verified.Sufficient || verified.NetShares != 25 {
		t.Fatalf("verification = %+v, %v", verified, err)
	}
}

func TestValidationAndBatchSummary(t *testing.T) {
	for _, cfg := range []equitrade.Config{
		{TaxRates: equitrade.TaxRates{Federal: math.NaN()}},
		{TaxModes: equitrade.TaxModes{Federal: "percentage"}},
		{BrokerFees: equitrade.BrokerFees{SECFeeRate: -1}},
	} {
		if _, err := equitrade.NewCalculator(cfg); err == nil {
			t.Fatalf("accepted invalid config %+v", cfg)
		}
	}
	var calc equitrade.Calculator
	input := equitrade.OptionsInput{ExercisePrice: 10, ExercisedShares: 100, FMV: 25}
	if _, err := calc.VerifyOptions(input, 40); err == nil {
		t.Fatal("accepted a missing fill price")
	}
	bad := input
	bad.GrantType = "unknown"
	notFinite := input
	notFinite.ExercisedShares = math.Inf(1)
	batch := calc.CalculateOptionsBatch([]equitrade.OptionsInput{bad, input, notFinite})
	if batch[0].Err == nil || batch[1].Err != nil || batch[2].Err == nil {
		t.Fatalf("unexpected batch %+v", batch)
	}
	summary := batch.Summarize()
	if summary.Count != 1 || summary.FailedCount != 2 || summary.AverageFMV != 25 ||
		summary.TotalCosts != 1000 || summary.TotalSharesToSell != 40 || summary.TotalNetShares != 60 {
		t.Fatalf("unexpected summary %+v", summary)
	}
	input.SalePrice = 25
	if _, err := calc.VerifyOptions(input, 101); err == nil {
		t.Fatal("accepted a sale of more shares than available")
	}
}

func TestStandaloneSaleAndWithholding(t *testing.T) {
	calc := newCalculator(t, equitrade.Config{
		BrokerFees: equitrade.BrokerFees{CommissionRate: .004, MinimumFee: .50, SECFeeRate: .0001},
		TaxRates:   equitrade.TaxRates{Federal: .50, State: .005},
		TaxModes:   equitrade.TaxModes{State: equitrade.TaxFixed},
	})
	sale, err := calc.CalculateSale(equitrade.SaleInput{SharesSold: 10, SalePrice: 10, AdjustedBasis: 102.005})
	if err != nil || sale.GrossProceeds != 100 || sale.Fees.SECLevy != .01 ||
		sale.NetProceeds != 99.49 || sale.AdjustedBasis != 102.01 || sale.CapitalGainLoss != -2.52 {
		t.Fatalf("sale = %+v, %v", sale, err)
	}
	tax, err := calc.CalculateWithholding(.29)
	if err != nil || tax.Federal != .15 || tax.State != .01 || tax.Total != .16 {
		t.Fatalf("tax = %+v, %v", tax, err)
	}
}
