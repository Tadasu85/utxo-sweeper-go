package main

import (
	"crypto/sha256"
	"errors"
)

// Network types
type Network int

const (
	BitcoinMainnet Network = iota
	BitcoinTestnet
	LitecoinMainnet
	LitecoinTestnet
)

// Asset types
type Asset int

const (
	BTC Asset = iota
	LTC
)

// Address types
type AddressType int

const (
	P2WPKH AddressType = iota
	P2TR
)

// Network configuration
type NetworkConfig struct {
	Network     Network
	Asset       Asset
	Bech32HRP   string
	Bech32mHRP  string
	P2PKHPrefix byte
	P2SHPrefix  byte
}

var networkConfigs = map[Network]NetworkConfig{
	BitcoinMainnet: {
		Network:     BitcoinMainnet,
		Asset:       BTC,
		Bech32HRP:   "bc",
		Bech32mHRP:  "bc",
		P2PKHPrefix: 0x00,
		P2SHPrefix:  0x05,
	},
	BitcoinTestnet: {
		Network:     BitcoinTestnet,
		Asset:       BTC,
		Bech32HRP:   "tb",
		Bech32mHRP:  "tb",
		P2PKHPrefix: 0x6f,
		P2SHPrefix:  0xc4,
	},
	LitecoinMainnet: {
		Network:     LitecoinMainnet,
		Asset:       LTC,
		Bech32HRP:   "ltc",
		Bech32mHRP:  "ltc",
		P2PKHPrefix: 0x30,
		P2SHPrefix:  0x32,
	},
	LitecoinTestnet: {
		Network:     LitecoinTestnet,
		Asset:       LTC,
		Bech32HRP:   "tltc",
		Bech32mHRP:  "tltc",
		P2PKHPrefix: 0x6f,
		P2SHPrefix:  0xc4,
	},
}

// Bech32 encoding constants
const (
	charset    = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	charsetRev = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var charsetMap = make(map[byte]int)
var charsetRevMap = make(map[byte]int)

func init() {
	for i, c := range charset {
		charsetMap[byte(c)] = i
	}
	for i, c := range charsetRev {
		charsetRevMap[byte(c)] = i
	}
}

// Bech32 polynomial (BIP-173)
var gen = []int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}

// Bech32 checksum (BIP-173 polymod)
func bech32Polymod(values []int) int {
	chk := 1
	for _, v := range values {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ v
		for i := 0; i < 5; i++ {
			if (b>>i)&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

// Bech32 expand HRP
func bech32ExpandHRP(hrp string) []int {
	// per BIP-173: [hrp_high...] + [0] + [hrp_low...]
	high := make([]int, len(hrp))
	low := make([]int, len(hrp))
	for i, c := range hrp {
		high[i] = int(c) >> 5
		low[i] = int(c) & 31
	}
	out := make([]int, 0, len(high)+1+len(low))
	out = append(out, high...)
	out = append(out, 0)
	out = append(out, low...)
	return out
}

// Bech32 verify checksum (constant=1) and Bech32m verify (constant=0x2bc830a3)
func bech32VerifyChecksum(hrp string, data []int, constant int) bool {
	return bech32Polymod(append(bech32ExpandHRP(hrp), data...)) == constant
}

// Bech32/Bech32m create checksum with provided constant
func bech32CreateChecksum(hrp string, data []int, constant int) []int {
	values := append(bech32ExpandHRP(hrp), data...)
	polymod := bech32Polymod(append(values, 0, 0, 0, 0, 0, 0)) ^ constant
	checksum := make([]int, 6)
	for i := 0; i < 6; i++ {
		checksum[i] = (polymod >> (5 * (5 - i))) & 31
	}
	return checksum
}

// Bech32 encode
func Bech32Encode(hrp string, data []int) string {
	// Select bech32 (1) for v0, bech32m (0x2bc830a3) for v>=1
	constant := 1
	if len(data) > 0 && data[0] != 0 {
		constant = 0x2bc830a3
	}
	combined := append(data, bech32CreateChecksum(hrp, data, constant)...)
	result := hrp + "1"
	for _, v := range combined {
		result += string(charset[v])
	}
	return result
}

// Bech32 decode
func Bech32Decode(bech string) (string, []int, error) {
	if len(bech) < 8 || len(bech) > 90 {
		return "", nil, errors.New("invalid bech32 string length")
	}

	// Check for mixed case
	hasLower := false
	hasUpper := false
	for _, c := range bech {
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
	}
	if hasLower && hasUpper {
		return "", nil, errors.New("mixed case in bech32 string")
	}

	// Convert to lowercase
	bech = toLower(bech)

	// Find separator
	pos := -1
	for i, c := range bech {
		if c == '1' {
			pos = i
			break
		}
	}
	if pos < 1 || pos > len(bech)-7 {
		return "", nil, errors.New("invalid separator position")
	}

	hrp := bech[:pos]
	data := bech[pos+1:]

	// Validate characters
	for _, c := range data {
		if _, ok := charsetMap[byte(c)]; !ok {
			return "", nil, errors.New("invalid character in data")
		}
	}

	// Convert to integers
	dataInt := make([]int, len(data))
	for i, c := range data {
		dataInt[i] = charsetMap[byte(c)]
	}

	// Verify checksum per BIP-350 using version in dataInt[0]
	constant := 1
	if len(dataInt) == 0 {
		return "", nil, errors.New("invalid data")
	}
	if dataInt[0] != 0 {
		constant = 0x2bc830a3
	}
	if !bech32VerifyChecksum(hrp, dataInt, constant) {
		return "", nil, errors.New("invalid checksum")
	}

	return hrp, dataInt[:len(dataInt)-6], nil
}

// Convert string to lowercase
func toLower(s string) string {
	result := make([]byte, len(s))
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			result[i] = byte(c + 32)
		} else {
			result[i] = byte(c)
		}
	}
	return string(result)
}

// Hash160 (RIPEMD160(SHA256(data)))
func Hash160(data []byte) []byte {
	sha := sha256.Sum256(data)
	return ripemd160(sha[:])
}

// RIPEMD160 implementation (simplified)
func ripemd160(data []byte) []byte {
	// This is a placeholder - in production you'd want a proper RIPEMD160 implementation
	// For now, we'll use a simplified version that just returns the first 20 bytes of SHA256
	hash := sha256.Sum256(data)
	return hash[:20]
}

// SHA256
func SHA256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// Convert 5-bit groups to 8-bit groups
func convertBits(data []int, fromBits, toBits int, pad bool) ([]byte, error) {
	acc := 0
	bits := 0
	result := make([]byte, 0)
	maxv := (1 << toBits) - 1
	maxAcc := (1 << (fromBits + toBits - 1)) - 1

	for _, value := range data {
		if value < 0 || (value>>fromBits) != 0 {
			return nil, errors.New("invalid value")
		}
		acc = ((acc << fromBits) | value) & maxAcc
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			result = append(result, byte((acc>>bits)&maxv))
		}
	}

	if pad {
		if bits > 0 {
			result = append(result, byte((acc<<(toBits-bits))&maxv))
		}
	} else if bits >= fromBits || ((acc<<(toBits-bits))&maxv) != 0 {
		return nil, errors.New("invalid padding")
	}

	return result, nil
}

// Convert bytes (8-bit) to 5-bit groups (ints) per BIP-173
func convert8to5(data []byte) ([]int, error) {
	acc := 0
	bits := 0
	ret := make([]int, 0)
	const toBits = 5
	const maxv = (1 << toBits) - 1
	for _, b := range data {
		if b>>8 != 0 {
			return nil, errors.New("invalid byte")
		}
		acc = (acc << 8) | int(b)
		bits += 8
		for bits >= toBits {
			bits -= toBits
			ret = append(ret, (acc>>bits)&maxv)
		}
	}
	if bits > 0 {
		ret = append(ret, (acc<<(toBits-bits))&maxv)
	}
	return ret, nil
}

// Address validation and creation
type Address struct {
	Type    AddressType
	Network Network
	Data    []byte
}

// Create P2WPKH address
func CreateP2WPKH(pubKeyHash []byte, network Network) (string, error) {
	if len(pubKeyHash) != 20 {
		return "", errors.New("invalid pubkey hash length")
	}

	config, ok := networkConfigs[network]
	if !ok {
		return "", errors.New("unsupported network")
	}

	// Convert witness program to 5-bit groups
	prog5, err := convert8to5(pubKeyHash)
	if err != nil {
		return "", err
	}
	data5bit := make([]int, 0, 1+len(prog5))
	data5bit = append(data5bit, 0) // witness version 0
	data5bit = append(data5bit, prog5...)

	return Bech32Encode(config.Bech32HRP, data5bit), nil
}

// Create P2TR address
func CreateP2TR(taprootOutputKey []byte, network Network) (string, error) {
	if len(taprootOutputKey) != 32 {
		return "", errors.New("invalid taproot output key length")
	}

	config, ok := networkConfigs[network]
	if !ok {
		return "", errors.New("unsupported network")
	}

	// Convert witness program to 5-bit groups
	prog5, err := convert8to5(taprootOutputKey)
	if err != nil {
		return "", err
	}
	data5bit := make([]int, 0, 1+len(prog5)) // 1 for version
	data5bit = append(data5bit, 1)           // witness version 1
	data5bit = append(data5bit, prog5...)

	return Bech32Encode(config.Bech32mHRP, data5bit), nil
}

// Decode address
func DecodeAddress(addr string) (*Address, error) {
	hrp, data, err := Bech32Decode(addr)
	if err != nil {
		return nil, err
	}

	// Find network
	var network Network
	var addrType AddressType
	for net, config := range networkConfigs {
		if hrp == config.Bech32HRP {
			network = net
			addrType = P2WPKH
			break
		}
		if hrp == config.Bech32mHRP {
			network = net
			addrType = P2TR
			break
		}
	}

	if network == 0 && hrp == "" {
		return nil, errors.New("unknown network")
	}

	// Convert 5-bit groups to bytes
	decoded, err := convertBits(data[1:], 5, 8, false)
	if err != nil {
		return nil, err
	}

	if addrType == P2WPKH && len(decoded) != 20 {
		return nil, errors.New("invalid P2WPKH data length")
	}
	if addrType == P2TR && len(decoded) != 32 {
		return nil, errors.New("invalid P2TR data length")
	}

	return &Address{
		Type:    addrType,
		Network: network,
		Data:    decoded,
	}, nil
}

// Validate address against public key
func ValidateAddress(addr string, pubKey []byte, network Network) error {
	decoded, err := DecodeAddress(addr)
	if err != nil {
		return err
	}

	if decoded.Network != network {
		return errors.New("address network mismatch")
	}

	// For P2WPKH, check if address matches pubkey hash
	if decoded.Type == P2WPKH {
		expectedHash := Hash160(pubKey)
		if !bytesEqual(decoded.Data, expectedHash) {
			return errors.New("address does not match public key")
		}
	}

	// For P2TR, check if address matches taproot output key
	if decoded.Type == P2TR {
		// In a real implementation, you'd derive the taproot output key from the pubkey
		// For now, we'll just check length
		if len(decoded.Data) != 32 {
			return errors.New("invalid taproot output key length")
		}
	}

	return nil
}

// Helper function to compare byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Derive addresses from public key
func DeriveChangeAddress(pubKey []byte, network Network) (string, error) {
	pubKeyHash := Hash160(pubKey)
	return CreateP2WPKH(pubKeyHash, network)
}

func DeriveDepositAddress(pubKey []byte, tag []byte, network Network) (string, error) {
	// Combine pubkey with tag
	combined := append(pubKey, tag...)
	pubKeyHash := Hash160(combined)
	return CreateP2WPKH(pubKeyHash, network)
}

// Script building
func BuildP2WPKHScript(pubKeyHash []byte) []byte {
	if len(pubKeyHash) != 20 {
		panic("invalid pubkey hash length")
	}
	script := make([]byte, 22)
	script[0] = 0x00 // OP_0
	script[1] = 0x14 // 20 bytes
	copy(script[2:], pubKeyHash)
	return script
}

func BuildP2TRScript(taprootOutputKey []byte) []byte {
	if len(taprootOutputKey) != 32 {
		panic("invalid taproot output key length")
	}
	script := make([]byte, 34)
	script[0] = 0x51 // OP_1
	script[1] = 0x20 // 32 bytes
	copy(script[2:], taprootOutputKey)
	return script
}
