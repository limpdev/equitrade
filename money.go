package equitrade

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
)

// This bound keeps the sum of all supported line items within cent resolution
// when converted back to float64 for the public API.
const maxAmount = 1e12

type cents int64

func (amount cents) dollars() float64 { return float64(amount) / 100 }

func decimal(value float64) *big.Rat {
	// Interpret the public float as its shortest round-trippable decimal.
	result, _ := new(big.Rat).SetString(strconv.FormatFloat(value, 'g', -1, 64))
	return result
}

func multiply(x, y *big.Rat) *big.Rat { return new(big.Rat).Mul(x, y) }

// quantize rounds nonnegative amounts. With up set, it rounds toward +infinity.
func quantize(value *big.Rat, up bool) int64 {
	var quotient, remainder big.Int
	quotient.QuoRem(value.Num(), value.Denom(), &remainder)
	if up && remainder.Sign() > 0 || !up && remainder.Lsh(&remainder, 1).Cmp(value.Denom()) >= 0 {
		return quotient.Int64() + 1
	}
	return quotient.Int64()
}

func scaled(value *big.Rat) *big.Rat  { return multiply(value, big.NewRat(100, 1)) }
func roundMoney(value *big.Rat) cents { return cents(quantize(scaled(value), false)) }

func validateAmount(name string, value float64, positive bool) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxAmount || positive && value == 0 {
		if positive {
			return fmt.Errorf("%s must be finite and greater than zero, up to %.0f", name, maxAmount)
		}
		return fmt.Errorf("%s must be finite and between zero and %.0f", name, maxAmount)
	}
	return nil
}

func validateNotional(name string, price, shares *big.Rat) error {
	if multiply(price, shares).Cmp(decimal(maxAmount)) > 0 {
		return fmt.Errorf("%s exceeds $%.0f", name, maxAmount)
	}
	return nil
}
