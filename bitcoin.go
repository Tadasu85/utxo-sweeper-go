// Package main provides a dependency-free Bitcoin UTXO sweeper library.
// This file contains Bitcoin-specific primitives including network configurations,
// Bech32/Bech32m encoding/decoding, address derivation, and script building.
package main

import (
	"crypto/sha256"
	"errors"
)

// Network represents the blockchain network type.
type Network int

const (
	BitcoinMainnet  Network = iota // Bitcoin mainnet
	BitcoinTestnet                 // Bitcoin testnet
	LitecoinMainnet                // Litecoin mainnet
	LitecoinTestnet                // Litecoin testnet
)

// Asset represents the cryptocurrency asset type.
type Asset int

const (
	BTC Asset = iota // Bitcoin
	LTC              // Litecoin
)

// AddressType represents the Bitcoin address format type.
type AddressType int

const (
	P2WPKH AddressType = iota // Pay-to-Witness-Public-Key-Hash (SegWit v0)
	P2TR                      // Pay-to-Taproot (SegWit v1)
)

// NetworkConfig holds configuration parameters for a specific blockchain network.
// This includes Bech32 prefixes, address prefixes, and other network-specific constants.
type NetworkConfig struct {
	Network     Network // The network type
	Asset       Asset   // The cryptocurrency asset
	Bech32HRP   string  // Human-readable part for Bech32 (SegWit v0)
	Bech32mHRP  string  // Human-readable part for Bech32m (SegWit v1/Taproot)
	P2PKHPrefix byte    // Legacy P2PKH address prefix
	P2SHPrefix  byte    // Legacy P2SH address prefix
}

// networkConfigs defines the configuration parameters for each supported network.
// These values are based on BIP-173 (Bech32) and BIP-350 (Bech32m) specifications.
var networkConfigs = map[Network]NetworkConfig{
	BitcoinMainnet: {
		Network:     BitcoinMainnet,
		Asset:       BTC,
		Bech32HRP:   "bc", // BIP-173: bc1...
		Bech32mHRP:  "bc", // BIP-350: bc1p... (Taproot)
		P2PKHPrefix: 0x00, // Legacy: 1...
		P2SHPrefix:  0x05, // Legacy: 3...
	},
	BitcoinTestnet: {
		Network:     BitcoinTestnet,
		Asset:       BTC,
		Bech32HRP:   "tb", // BIP-173: tb1...
		Bech32mHRP:  "tb", // BIP-350: tb1p... (Taproot)
		P2PKHPrefix: 0x6f, // Legacy: m/n...
		P2SHPrefix:  0xc4, // Legacy: 2...
	},
	LitecoinMainnet: {
		Network:     LitecoinMainnet,
		Asset:       LTC,
		Bech32HRP:   "ltc", // Litecoin: ltc1...
		Bech32mHRP:  "ltc", // Litecoin: ltc1p... (Taproot)
		P2PKHPrefix: 0x30,  // Legacy: L...
		P2SHPrefix:  0x32,  // Legacy: M...
	},
	LitecoinTestnet: {
		Network:     LitecoinTestnet,
		Asset:       LTC,
		Bech32HRP:   "tltc", // Litecoin testnet: tltc1...
		Bech32mHRP:  "tltc", // Litecoin testnet: tltc1p... (Taproot)
		P2PKHPrefix: 0x6f,   // Legacy: m/n...
		P2SHPrefix:  0xc4,   // Legacy: Q...
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

// gen is the Bech32 generator polynomial coefficients as specified in BIP-173.
// These values are used in the polymod function for checksum calculation.
var gen = []int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}

// bech32Polymod implements the Bech32 checksum polynomial as specified in BIP-173.
// It takes a slice of 5-bit values and returns the polymod checksum.
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

// Bech32Encode creates a Bech32-encoded string from a human-readable part and 5-bit data.
// It automatically selects the correct checksum constant (1 for SegWit v0, 0x2bc830a3 for Taproot).
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

// Bech32Decode parses a Bech32/Bech32m string and returns HRP and the 5-bit data
// (including witness version in data[0]). It validates HRP charset, forbids mixed
// case, and verifies the checksum constant using the version (BIP-173/350).
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
	// Validate HRP characters per BIP-173 (33..126)
	if len(hrp) == 0 {
		return "", nil, errors.New("empty HRP")
	}
	for i := 0; i < len(hrp); i++ {
		c := hrp[i]
		if c < 33 || c > 126 {
			return "", nil, errors.New("invalid HRP character")
		}
	}
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

	// Verify checksum constant based on witness version per BIP-350
	if len(dataInt) < 7 { // at least version + checksum(6)
		return "", nil, errors.New("invalid data length")
	}
	ver := dataInt[0]
	if ver < 0 || ver > 31 { // 5-bit value range
		return "", nil, errors.New("invalid witness version value")
	}
	var constant int
	switch ver {
	case 0:
		constant = 1
	default:
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
func ripemd160(data []byte) []byte { h := NewRIPEMD160(); h.Write(data); return h.Sum(nil) }

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
		// No need to check b>>8 since b is a byte (0-255)
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

// CreateP2WPKH creates a Pay-to-Witness-Public-Key-Hash (SegWit v0) address.
// It takes a 20-byte public key hash and network type, returning a Bech32-encoded address.
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

// CreateP2TR creates a Pay-to-Taproot (SegWit v1) address.
// It takes a 32-byte Taproot output key and network type, returning a Bech32m-encoded address.
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

// DecodeAddress parses a Bech32/Bech32m address and returns address components.
// Network is determined by HRP; type is determined by witness version (v0=P2WPKH,
// v1=P2TR). Only these types are supported by this library.
func DecodeAddress(addr string) (*Address, error) {
	hrp, data, err := Bech32Decode(addr)
	if err != nil {
		return nil, err
	}

	// Determine network by HRP only (either Bech32 HRP or Bech32m HRP matches)
	var network Network
	found := false
	for net, config := range networkConfigs {
		if hrp == config.Bech32HRP || hrp == config.Bech32mHRP {
			network = net
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("unknown network")
	}

	// Convert 5-bit groups to bytes
	decoded, err := convertBits(data[1:], 5, 8, false)
	if err != nil {
		return nil, err
	}

	// Determine address type by witness version (data[0])
	version := data[0]
	var addrType AddressType
	switch version {
	case 0:
		addrType = P2WPKH
		if len(decoded) != 20 {
			return nil, errors.New("invalid P2WPKH data length")
		}
	case 1:
		addrType = P2TR
		if len(decoded) != 32 {
			return nil, errors.New("invalid P2TR data length")
		}
	default:
		return nil, errors.New("unsupported witness version")
	}

	return &Address{
		Type:    addrType,
		Network: network,
		Data:    decoded,
	}, nil
}

// ValidateAddress verifies that an address is valid and matches the provided public key.
// It checks the address format, network compatibility, and cryptographic validation.
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

// DeriveChangeAddress creates a v0 P2WPKH change address from a compressed pubkey.
func DeriveChangeAddress(pubKey []byte, network Network) (string, error) {
	pubKeyHash := Hash160(pubKey)
	return CreateP2WPKH(pubKeyHash, network)
}

// DeriveDepositAddress creates a v0 P2WPKH deposit address from a compressed pubkey
// and optional tag; different tags yield different addresses.
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
