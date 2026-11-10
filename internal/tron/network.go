package tron

import "fmt"

type Network string

const (
	NetworkMainnet   Network = "tron-mainnet"
	NetworkNile      Network = "tron-nile"
	NetworkSimulated Network = "simulated"
)

const (
	mainnetUSDTContract Address = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	nileUSDTContract    Address = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"
)

func ParseNetwork(value string) (Network, error) {
	switch network := Network(value); network {
	case NetworkMainnet, NetworkNile, NetworkSimulated:
		return network, nil
	default:
		return "", fmt.Errorf("unknown network %q: expected tron-mainnet, tron-nile or simulated", value)
	}
}

func (n Network) IsSimulated() bool {
	return n == NetworkSimulated
}

func IsKnownUSDTContract(address Address) bool {
	return address == mainnetUSDTContract || address == nileUSDTContract
}

func (n Network) USDTContract() (Address, bool) {
	switch n {
	case NetworkMainnet:
		return mainnetUSDTContract, true
	case NetworkNile:
		return nileUSDTContract, true
	default:
		return "", false
	}
}
