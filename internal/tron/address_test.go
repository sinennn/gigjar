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

func TestHexConversionMatchesTronGrid(t *testing.T) {
	cases := []struct {
		hex  string
		want Address
	}{
		{"41b5131044ccc9d9600aa4384157017dd83496511e", "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA"},
		{"eca9bc828a3005b9a3b909f2cc5c2a54794de05f", "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"},
		{"0x000000000000000000000000b5131044ccc9d9600aa4384157017dd83496511e", "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA"},
	}
	for _, c := range cases {
		got, err := AddressFromHex(c.hex)
		if err != nil || got != c.want {
			t.Errorf("AddressFromHex(%q) = %q, %v; want %q", c.hex, got, err, c.want)
		}
	}
	hex20, err := Address("TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf").Hex20()
	if err != nil || hex20 != "eca9bc828a3005b9a3b909f2cc5c2a54794de05f" {
		t.Errorf("Hex20 = %q, %v", hex20, err)
	}
}

func TestAddressFromHexRejectsGarbage(t *testing.T) {
	for _, value := range []string{"", "zz", "42b5131044ccc9d9600aa4384157017dd83496511e", "0x010000000000000000000000b5131044ccc9d9600aa4384157017dd83496511e"} {
		if _, err := AddressFromHex(value); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("AddressFromHex(%q) = %v, want ErrInvalidAddress", value, err)
		}
	}
}

func TestSimulatedJarAddressesAreValidAndDistinct(t *testing.T) {
	seen := map[Address]bool{}
	for index := int64(1); index <= 200; index++ {
		address := SimulatedJarAddress(index)
		if _, err := ParseAddress(string(address)); err != nil {
			t.Fatalf("simulated address %q is invalid: %v", address, err)
		}
		if seen[address] {
			t.Fatalf("duplicate simulated address %q", address)
		}
		seen[address] = true
	}
	if SimulatedJarAddress(7) != SimulatedJarAddress(7) {
		t.Fatal("simulated addresses must be deterministic")
	}
}
