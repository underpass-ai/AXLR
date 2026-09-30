package domain

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

// ModelRate is a decimal USD price per token. Its zero value means unknown.
type ModelRate struct{ decimal string }

var modelRatePattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

func NewModelRate(perToken string) (ModelRate, error) {
	if !modelRatePattern.MatchString(perToken) {
		return ModelRate{}, errors.New("model rate must be a nonnegative decimal USD per token")
	}
	return ModelRate{decimal: perToken}, nil
}

// Display returns the price in USD per million tokens, or empty when unknown.
func (r ModelRate) Display() string {
	if r.decimal == "" {
		return ""
	}
	price, _ := new(big.Rat).SetString(r.decimal)
	price.Mul(price, big.NewRat(1_000_000, 1))
	precision := 0
	if dot := strings.IndexByte(r.decimal, '.'); dot >= 0 {
		precision = len(r.decimal) - dot - 1 - 6
		if precision < 0 {
			precision = 0
		}
	}
	value := price.FloatString(precision)
	if precision > 0 {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	if dot := strings.IndexByte(value, '.'); dot >= 0 && len(value)-dot-1 < 2 {
		value += strings.Repeat("0", 2-(len(value)-dot-1))
	}
	return "$" + value + " / 1M tokens"
}
