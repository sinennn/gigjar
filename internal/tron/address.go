package tron

import (
	"bytes"
	"crypto/sha256"
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
