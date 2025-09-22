// Package main provides a command-line interface for the Bitcoin UTXO sweeper library.
// This demonstrates how to use the Sweeper API for UTXO management and transaction creation.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// DEFAULT_DEST_ADDR is a canonical Bitcoin testnet address used as the default destination.
// It can be overridden via the --dest flag or DEST_ADDR environment variable.
const DEFAULT_DEST_ADDR = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"

// main demonstrates the Sweeper API by loading UTXOs from a JSON file and creating a transaction.
// It shows how to configure the sweeper, index UTXOs, and generate a PSBT for signing.
func main() {
	// Parse command-line flags
	destFlag := flag.String("dest", "", "Bitcoin address to send funds to (overrides DEST_ADDR env var)")
	configFlag := flag.String("config", "config.json", "Configuration file path")
	helpFlag := flag.Bool("help", false, "Show detailed help information and usage examples")
	versionFlag := flag.Bool("version", false, "Show version information")

	// Custom usage function
	flag.Usage = func() {
		printUsage()
	}

	flag.Parse()

	// Handle help and version flags
	if *helpFlag {
		printUsage()
		os.Exit(0)
	}

	if *versionFlag {
		printVersion()
		os.Exit(0)
	}

	// Load configuration
	config, err := LoadConfig(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Determine destination address from flag, environment, or default
	destAddr := os.Getenv("DEST_ADDR")
	if *destFlag != "" {
		destAddr = *destFlag
	}
	if destAddr == "" {
		destAddr = DEFAULT_DEST_ADDR
	}

	// Load UTXOs from JSON file
	var utxos []UTXO
	if err := json.Unmarshal(mustReadFile("utxos.json"), &utxos); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse utxos.json: %v\n", err)
		fmt.Fprintf(os.Stderr, "Expected format: [{\"TxID\":\"...\",\"Vout\":0,\"ValueSats\":80000,\"Address\":\"tb1...\",\"Confirmed\":true}]\n")
		os.Exit(1)
	}

	// Create sweeper instance with test public key
	// In production, this would be a real 33-byte compressed public key
	testPubKey := []byte("test_public_key_32_bytes_long_here")
	sweeper := NewSweeper(testPubKey, config.ToNetwork())

	// Apply configuration to sweeper
	if err := config.ApplyToSweeper(sweeper); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to apply configuration: %v\n", err)
		os.Exit(1)
	}

	// Index all UTXOs from the file
	fmt.Println("Indexing UTXOs...")
	for i, utxo := range utxos {
		if err := sweeper.Index(utxo); err != nil {
			fmt.Printf("Failed to index UTXO %d (%s:%d): %v\n", i, utxo.TxID[:8]+"...", utxo.Vout, err)
			continue
		}
		fmt.Printf("Indexed UTXO %d: %s:%d (%d sats)\n", i, utxo.TxID, utxo.Vout, utxo.ValueSats)
	}

	// Create spending transaction with single output
	outputs := []TxOutput{
		{Address: destAddr, ValueSats: 150_000}, // Send 150,000 sats to destination
	}

	fmt.Println("\nCreating spending transaction...")
	plan, err := sweeper.Spend(outputs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Transaction creation failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "Check that you have sufficient UTXOs and valid addresses\n")
		os.Exit(1)
	}

	// Encode PSBT for external signing
	psbtB64, err := plan.PSBT.B64Encode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "PSBT encoding failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "This is an internal error - please report this issue\n")
		os.Exit(1)
	}

	// Display results based on output format
	if config.OutputFormat == "json" {
		outputJSON(plan, psbtB64, sweeper)
	} else {
		outputHuman(plan, psbtB64, sweeper)
	}
}

// mustReadFile reads a file and exits the program if an error occurs.
// This is a helper function for the main demonstration.
func mustReadFile(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't read %s: %v\n", path, err)
		os.Exit(1)
	}
	return b
}

// printUsage displays comprehensive help information and usage examples.
func printUsage() {
	fmt.Fprintf(os.Stderr, `UTXO Sweeper - Dependency-free Bitcoin UTXO management library

USAGE:
    utxo-sweeper [OPTIONS]

DESCRIPTION:
    A command-line demonstration of the UTXO Sweeper library that loads UTXOs
    from a JSON file and creates Bitcoin transactions. The library provides
    efficient UTXO indexing, dust filtering, transaction planning, and PSBT
    generation for Bitcoin and Litecoin networks.

OPTIONS:
    -dest string
        Bitcoin address to send funds to (overrides DEST_ADDR env var)
        Default: tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx (testnet)
        
    -config string
        Configuration file path (JSON format)
        Default: config.json
        
    -help
        Show this help information and usage examples
        
    -version
        Show version information

ENVIRONMENT VARIABLES:
    DEST_ADDR    Bitcoin address to send funds to (overridden by -dest flag)

EXAMPLES:
    # Basic usage with default configuration
    utxo-sweeper
    
    # Send to a specific address
    utxo-sweeper -dest bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx
    
    # Use custom configuration file
    utxo-sweeper -config my-config.json
    
    # Use environment variable
    DEST_ADDR=bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx utxo-sweeper
    
    # JSON output for scripting
    utxo-sweeper -config config.json | jq '.transaction_plan.fee_sats'
    
    # Show help
    utxo-sweeper -help
    
    # Show version
    utxo-sweeper -version

INPUT FILE:
    The program expects a utxos.json file in the current directory with the
    following format:
    
    [
      {
        "TxID": "1111111111111111111111111111111111111111111111111111111111111111",
        "Vout": 0,
        "ValueSats": 80000,
        "Address": "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
        "Confirmed": true
      }
    ]

OUTPUT:
    The program outputs:
    - Indexed UTXOs with their details
    - Transaction plan with inputs, outputs, and fees
    - Base64-encoded PSBT ready for signing
    - Unconfirmed transaction chain depth

FEATURES:
    - No external dependencies
    - Supports Bitcoin mainnet/testnet and Litecoin
    - Bech32/Bech32m address support
    - Dust filtering ($0.50 USD threshold)
    - Unconfirmed transaction chaining
    - PSBT output for external signing
    - Change splitting and weighted allocation

For more information, visit: https://github.com/Tadasu85/utxo-sweeper-go
`)
}

// printVersion displays version information.
func printVersion() {
	fmt.Printf(`UTXO Sweeper v1.0.0
A dependency-free Go library for Bitcoin UTXO management

Features:
- Bitcoin and Litecoin support
- Bech32/Bech32m address handling
- Dust filtering and transaction planning
- PSBT generation for external signing
- No external dependencies

Repository: https://github.com/Tadasu85/utxo-sweeper-go
License: MIT
`)
}

// outputHuman displays results in human-readable format.
func outputHuman(plan *TransactionPlan, psbtB64 string, sweeper *Sweeper) {
	fmt.Println("\nTransaction Plan:")
	fmt.Println("Inputs:", plan.Inputs)
	fmt.Println("Outputs:", plan.Outputs)
	fmt.Println("Fee (sats):", plan.FeeSats)
	fmt.Println("PSBT (b64):", psbtB64)
	fmt.Println("\nChain Depth:", sweeper.PendingChainDepth())
}

// outputJSON displays results in JSON format for programmatic consumption.
func outputJSON(plan *TransactionPlan, psbtB64 string, sweeper *Sweeper) {
	result := map[string]interface{}{
		"transaction_plan": map[string]interface{}{
			"inputs":  plan.Inputs,
			"outputs": plan.Outputs,
			"fee_sats": plan.FeeSats,
			"psbt_b64": psbtB64,
		},
		"chain_depth": sweeper.PendingChainDepth(),
	}
	
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON output: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println(string(jsonData))
}
