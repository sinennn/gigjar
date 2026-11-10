package usdt

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestParse(t *testing.T) {
	cases := map[string]Amount{
		"4":              4_000_000,
		"4.5":            4_500_000,
		"0.000001":       1,
		"11.000000":      11_000_000,
		"  12.34 ":       12_340_000,
		"-3.25":          -3_250_000,
		"9223372036854":  9_223_372_036_854_000_000,
		"0":              0,
		"007.100000":     7_100_000,
		"1234567.891234": 1_234_567_891_234,
	}
	for input, want := range cases {
		got, err := Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("Parse(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", ".", "1.", ".5", "1.0000001", "1e6", "abc", "1,000", "+1", "--1", "1.2.3", "NaN", "0x10"} {
		if _, err := Parse(input); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalidAmount", input, err)
		}
	}
	if _, err := Parse("9223372036855"); !errors.Is(err, ErrAmountTooLarge) {
		t.Errorf("expected overflow rejection, got %v", err)
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, amount := range []Amount{0, 1, 999_999, 1_000_000, 4_000_000, 12_345_678, -1, -12_345_678, math.MaxInt64} {
		text := amount.String()
		parsed, err := Parse(text)
		if err != nil || parsed != amount {
			t.Errorf("round trip %d -> %q -> %d (%v)", amount, text, parsed, err)
		}
	}
	if got := Amount(4_000_000).String(); got != "4.000000" {
		t.Errorf("String() = %q", got)
	}
	if got := Amount(-1).String(); got != "-0.000001" {
		t.Errorf("String() = %q", got)
	}
}

func TestJSONUsesDecimalStrings(t *testing.T) {
	payload, err := json.Marshal(struct {
		Amount Amount `json:"amount"`
	}{Amount: 7_250_000})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"amount":"7.250000"}` {
		t.Errorf("json = %s", payload)
	}

	var decoded struct {
		Amount Amount `json:"amount"`
	}
	if err := json.Unmarshal([]byte(`{"amount":"3.5"}`), &decoded); err != nil || decoded.Amount != 3_500_000 {
		t.Errorf("decoded = %d, err = %v", decoded.Amount, err)
	}
	if err := json.Unmarshal([]byte(`{"amount":3.5}`), &decoded); err == nil {
		t.Error("JSON numbers must be rejected")
	}
}

func TestAddDetectsOverflow(t *testing.T) {
	if _, err := Add(math.MaxInt64, 1); !errors.Is(err, ErrAmountTooLarge) {
		t.Errorf("expected overflow, got %v", err)
	}
	if sum, err := Add(4_000_000, 3_000_000); err != nil || sum != 7_000_000 {
		t.Errorf("Add = %d, %v", sum, err)
	}
}
