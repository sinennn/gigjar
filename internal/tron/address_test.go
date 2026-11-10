package tron

import (
	"errors"
	"testing"
)

func TestParseAddressAcceptsRealAddresses(t *testing.T) {
	for _, address := range []string{
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		" TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t ",
	} {
		if _, err := ParseAddress(address); err != nil {
			t.Errorf("ParseAddress(%q) = %v", address, err)
		}
	}
}

func TestParseAddressRejectsInvalidAddresses(t *testing.T) {
	for _, address := range []string{
		"",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6u",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6",
		"0xdAC17F958D2ee523a2206206994597C13D831ec7",
		"1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj0t",
	} {
		if _, err := ParseAddress(address); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("ParseAddress(%q) = %v, want ErrInvalidAddress", address, err)
		}
	}
}

func TestNetworkContractsAreValidAddresses(t *testing.T) {
	for _, network := range []Network{NetworkMainnet, NetworkNile} {
		contract, ok := network.USDTContract()
		if !ok {
			t.Fatalf("%s has no USDT contract", network)
		}
		if _, err := ParseAddress(string(contract)); err != nil {
			t.Errorf("%s contract %s is invalid: %v", network, contract, err)
		}
	}
	if _, ok := NetworkSimulated.USDTContract(); ok {
		t.Error("simulated network must not claim a real USDT contract")
	}
	if _, err := ParseNetwork("ethereum"); err == nil {
		t.Error("unknown network accepted")
	}
}
