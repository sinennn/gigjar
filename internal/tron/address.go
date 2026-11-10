package tron

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
)

const (
	addressPrefixByte    = 0x41
	addressPayloadLength = 21
	checksumLength       = 4
	base58Alphabet       = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
)

var ErrInvalidAddress = errors.New("not a valid TRON address")

type Address string

func ParseAddress(value string) (Address, error) {
	text := strings.TrimSpace(value)
	if len(text) != 34 || text[0] != 'T' {
		return "", ErrInvalidAddress
	}
	decoded, ok := decodeBase58(text)
	if !ok || len(decoded) != addressPayloadLength+checksumLength {
		return "", ErrInvalidAddress
	}
	payload, checksum := decoded[:addressPayloadLength], decoded[addressPayloadLength:]
	if payload[0] != addressPrefixByte {
		return "", ErrInvalidAddress
	}
	first := sha256.Sum256(payload)
	second := sha256.Sum256(first[:])
	if !bytes.Equal(second[:checksumLength], checksum) {
		return "", ErrInvalidAddress
	}
	return Address(text), nil
}

func (a Address) String() string {
	return string(a)
}

func (a Address) Payload() ([]byte, error) {
	if _, err := ParseAddress(string(a)); err != nil {
		return nil, err
	}
	decoded, _ := decodeBase58(string(a))
	return decoded[:addressPayloadLength], nil
}

func (a Address) Hex20() (string, error) {
	payload, err := a.Payload()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(payload[1:]), nil
}

func AddressFromPayload(payload []byte) (Address, error) {
	if len(payload) != addressPayloadLength || payload[0] != addressPrefixByte {
		return "", ErrInvalidAddress
	}
	first := sha256.Sum256(payload)
	second := sha256.Sum256(first[:])
	full := append(append([]byte{}, payload...), second[:checksumLength]...)
	return Address(encodeBase58(full)), nil
}

func AddressFromHex(value string) (Address, error) {
	text := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x")
	raw, err := hex.DecodeString(text)
	if err != nil {
		return "", ErrInvalidAddress
	}
	switch len(raw) {
	case addressPayloadLength - 1:
		return AddressFromPayload(append([]byte{addressPrefixByte}, raw...))
	case addressPayloadLength:
		return AddressFromPayload(raw)
	case 32:
		if !bytes.Equal(raw[:12], make([]byte, 12)) {
			return "", ErrInvalidAddress
		}
		return AddressFromPayload(append([]byte{addressPrefixByte}, raw[12:]...))
	default:
		return "", ErrInvalidAddress
	}
}

func SimulatedJarAddress(derivationIndex int64) Address {
	digest := sha256.Sum256([]byte("gigjar-simulated-jar/" + big.NewInt(derivationIndex).String()))
	address, _ := AddressFromPayload(append([]byte{addressPrefixByte}, digest[:addressPayloadLength-1]...))
	return address
}

func encodeBase58(data []byte) string {
	value := new(big.Int).SetBytes(data)
	radix := big.NewInt(58)
	remainder := new(big.Int)
	var encoded []byte
	for value.Sign() > 0 {
		value.DivMod(value, radix, remainder)
		encoded = append(encoded, base58Alphabet[remainder.Int64()])
	}
	for _, b := range data {
		if b != 0 {
			break
		}
		encoded = append(encoded, base58Alphabet[0])
	}
	for left, right := 0, len(encoded)-1; left < right; left, right = left+1, right-1 {
		encoded[left], encoded[right] = encoded[right], encoded[left]
	}
	return string(encoded)
}

func decodeBase58(text string) ([]byte, bool) {
	value := new(big.Int)
	radix := big.NewInt(58)
	for _, character := range text {
		index := strings.IndexRune(base58Alphabet, character)
		if index < 0 {
			return nil, false
		}
		value.Mul(value, radix)
		value.Add(value, big.NewInt(int64(index)))
	}
	decoded := value.Bytes()
	leadingZeros := 0
	for leadingZeros < len(text) && text[leadingZeros] == '1' {
		leadingZeros++
	}
	return append(make([]byte, leadingZeros), decoded...), true
}
