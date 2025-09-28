// Package main provides a dependency-free Bitcoin UTXO sweeper library.
// This file contains configuration structures and loading functionality.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the configuration file structure.
// It allows users to specify settings without hardcoding them in the program.
type Config struct {
	// Network settings
	Network string `json:"network"` // "bitcoin_mainnet", "bitcoin_testnet", "litecoin_mainnet", "litecoin_testnet"

	// Fee settings
	FeeRate int64 `json:"fee_rate"` // Fee rate in satoshis per virtual byte

	// Dust filtering
	DustThresholdUSD float64 `json:"dust_threshold_usd"` // Dust threshold in USD
	PriceUSDPerBTC   float64 `json:"price_usd_per_btc"`  // BTC price for dust calculation

	// Unconfirmed transaction handling
	AllowUnconfirmed bool `json:"allow_unconfirmed"` // Whether to allow unconfirmed UTXOs
	MaxUnconfirmed   int  `json:"max_unconfirmed"`   // Maximum unconfirmed inputs per transaction
	MaxChainDepth    int  `json:"max_chain_depth"`   // Maximum unconfirmed transaction chain depth

	// Change handling
	ChangeSplitParts int   `json:"change_split_parts"` // Number of parts to split change into
	TargetChunkSats  int64 `json:"target_chunk_sats"`  // Target size for change chunks
	MinChunkSats     int64 `json:"min_chunk_sats"`     // Minimum size for change chunks

	// Output settings
	OutputFormat string `json:"output_format"` // "human", "json"

	// Validation settings
	TestMode      bool `json:"test_mode"`      // Skip strict address validation
	EnforcePubKey bool `json:"enforce_pubkey"` // Enforce public key validation
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() *Config {
	return &Config{
		Network:          "bitcoin_testnet",
		FeeRate:          5,
		DustThresholdUSD: 0.50,
		PriceUSDPerBTC:   55000.0,
		AllowUnconfirmed: true,
		MaxUnconfirmed:   2,
		MaxChainDepth:    2,
		ChangeSplitParts: 1,
		TargetChunkSats:  60000,
		MinChunkSats:     20000,
		OutputFormat:     "human",
		TestMode:         true,
		EnforcePubKey:    false,
	}
}

// LoadConfig loads configuration from a JSON file.
// If the file doesn't exist, it returns the default configuration.
func LoadConfig(filename string) (*Config, error) {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read and parse config file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", filename, err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file '%s': %w", filename, err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// SaveConfig saves the configuration to a JSON file.
func (c *Config) SaveConfig(filename string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file '%s': %w", filename, err)
	}

	return nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	// Validate network
	validNetworks := map[string]bool{
		"bitcoin_mainnet":  true,
		"bitcoin_testnet":  true,
		"litecoin_mainnet": true,
		"litecoin_testnet": true,
	}
	if !validNetworks[c.Network] {
		return fmt.Errorf("invalid network '%s' - must be one of: bitcoin_mainnet, bitcoin_testnet, litecoin_mainnet, litecoin_testnet", c.Network)
	}

	// Validate fee rate
	if c.FeeRate <= 0 {
		return fmt.Errorf("fee_rate must be positive (got %d)", c.FeeRate)
	}

	// Validate dust threshold
	if c.DustThresholdUSD < 0 {
		return fmt.Errorf("dust_threshold_usd must be non-negative (got %f)", c.DustThresholdUSD)
	}

	// Validate BTC price
	if c.PriceUSDPerBTC <= 0 {
		return fmt.Errorf("price_usd_per_btc must be positive (got %f)", c.PriceUSDPerBTC)
	}

	// Validate unconfirmed settings
	if c.MaxUnconfirmed < 0 {
		return fmt.Errorf("max_unconfirmed must be non-negative (got %d)", c.MaxUnconfirmed)
	}
	if c.MaxChainDepth < 0 {
		return fmt.Errorf("max_chain_depth must be non-negative (got %d)", c.MaxChainDepth)
	}

	// Validate change settings
	if c.ChangeSplitParts < 1 {
		return fmt.Errorf("change_split_parts must be at least 1 (got %d)", c.ChangeSplitParts)
	}
	if c.TargetChunkSats < 0 {
		return fmt.Errorf("target_chunk_sats must be non-negative (got %d)", c.TargetChunkSats)
	}
	if c.MinChunkSats < 0 {
		return fmt.Errorf("min_chunk_sats must be non-negative (got %d)", c.MinChunkSats)
	}

	// Validate output format
	validFormats := map[string]bool{
		"human": true,
		"json":  true,
	}
	if !validFormats[c.OutputFormat] {
		return fmt.Errorf("invalid output_format '%s' - must be 'human' or 'json'", c.OutputFormat)
	}

	return nil
}

// ToNetwork converts the string network to the Network enum.
func (c *Config) ToNetwork() Network {
	switch c.Network {
	case "bitcoin_mainnet":
		return BitcoinMainnet
	case "bitcoin_testnet":
		return BitcoinTestnet
	case "litecoin_mainnet":
		return LitecoinMainnet
	case "litecoin_testnet":
		return LitecoinTestnet
	default:
		return BitcoinTestnet // fallback
	}
}

// ApplyToSweeper applies the configuration to a Sweeper instance.
func (c *Config) ApplyToSweeper(s *Sweeper) error {
	// Set network
	s.SetNetwork(c.ToNetwork())

	// Set fee rate
	if err := s.SetFeeRate(c.FeeRate); err != nil {
		return fmt.Errorf("failed to set fee rate: %w", err)
	}

	// Set dust rate
	s.SetDustRate(int64(c.DustThresholdUSD*100), c.DustThresholdUSD, c.PriceUSDPerBTC)

	// Set unconfirmed policy
	s.SetUnconfirmedPolicy(c.AllowUnconfirmed, c.MaxUnconfirmed, c.MaxChainDepth)

	// Set test mode and pubkey check
	s.SetTestMode(c.TestMode)
	s.SetPubKeyCheck(c.EnforcePubKey)

	// Set change split
	s.SetChangeSplit(c.ChangeSplitParts, c.TargetChunkSats, c.MinChunkSats)

	return nil
}
