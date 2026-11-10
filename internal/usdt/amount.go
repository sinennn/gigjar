package usdt

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	Decimals      = 6
	UnitsPerToken = 1_000_000
)

type Amount int64

var (
	ErrInvalidAmount  = errors.New("invalid USDT amount")
	ErrAmountTooLarge = errors.New("USDT amount out of range")
)

func Parse(value string) (Amount, error) {
	text := strings.TrimSpace(value)
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	whole, fraction, hasPoint := strings.Cut(text, ".")
	if whole == "" || (hasPoint && fraction == "") || len(fraction) > Decimals {
		return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, value)
	}
	if !allDigits(whole) || !allDigits(fraction) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, value)
	}
	wholeUnits, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || wholeUnits > math.MaxInt64/UnitsPerToken {
		return 0, fmt.Errorf("%w: %q", ErrAmountTooLarge, value)
	}
	fractionUnits := int64(0)
	if fraction != "" {
		padded := fraction + strings.Repeat("0", Decimals-len(fraction))
		fractionUnits, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, value)
		}
	}
	units := wholeUnits*UnitsPerToken + fractionUnits
	if units < 0 {
		return 0, fmt.Errorf("%w: %q", ErrAmountTooLarge, value)
	}
	if negative {
		units = -units
	}
	return Amount(units), nil
}

func MustParse(value string) Amount {
	amount, err := Parse(value)
	if err != nil {
		panic(err)
	}
	return amount
}

func (a Amount) String() string {
	units := int64(a)
	sign := ""
	if units < 0 {
		sign = "-"
		units = -units
	}
	return fmt.Sprintf("%s%d.%06d", sign, units/UnitsPerToken, units%UnitsPerToken)
}

func (a Amount) IsPositive() bool {
	return a > 0
}

func (a Amount) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a *Amount) UnmarshalText(text []byte) error {
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

func Add(a, b Amount) (Amount, error) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, ErrAmountTooLarge
	}
	return sum, nil
}

func allDigits(text string) bool {
	for _, character := range text {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
