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
	destFlag := flag.String("dest", "", "destination address for spend (overrides DEST_ADDR env)")
	testMode := flag.Bool("testmode", true, "enable test mode (skip strict address validation)")
	noPubKeyCheck := flag.Bool("no-pubkey-check", true, "disable enforcing that inputs match configured pubkey")
	flag.Parse()

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
		fmt.Fprintf(os.Stderr, "bad utxos.json: %v\n", err)
		os.Exit(1)
	}

	// Create sweeper instance with test public key
	// In production, this would be a real 33-byte compressed public key
	testPubKey := []byte("test_public_key_32_bytes_long_here")
	sweeper := NewSweeper(testPubKey, BitcoinTestnet)

	// Configure sweeper with appropriate settings
	sweeper.SetFeeRate(5)                    // 5 sat/vB fee rate
	sweeper.SetDustRate(600, 0.50, 55000)    // $0.50 dust threshold at $55k BTC
	sweeper.SetUnconfirmedPolicy(true, 2, 2) // Allow 2 unconfirmed inputs, max depth 2
	sweeper.SetTestMode(*testMode)           // Enable/disable test mode
	sweeper.SetPubKeyCheck(!*noPubKeyCheck)  // Enable/disable public key validation

	// Index all UTXOs from the file
	fmt.Println("Indexing UTXOs...")
	for i, utxo := range utxos {
		if err := sweeper.Index(utxo); err != nil {
			fmt.Printf("Failed to index UTXO %d: %v\n", i, err)
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
		fmt.Fprintf(os.Stderr, "spending failed: %v\n", err)
		os.Exit(1)
	}

	// Encode PSBT for external signing
	psbtB64, err := plan.PSBT.B64Encode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "psbt encode failed: %v\n", err)
		os.Exit(1)
	}

	// Display transaction details
	fmt.Println("\nTransaction Plan:")
	fmt.Println("Inputs:", plan.Inputs)
	fmt.Println("Outputs:", plan.Outputs)
	fmt.Println("Fee (sats):", plan.FeeSats)
	fmt.Println("PSBT (b64):", psbtB64)

	// Show unconfirmed transaction chain depth
	fmt.Println("\nChain Depth:", sweeper.PendingChainDepth())
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
