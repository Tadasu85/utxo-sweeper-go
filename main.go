package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// Default testnet destination address (P2WPKH example). Override via --dest or DEST_ADDR env.
const DEFAULT_DEST_ADDR = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"

// Main function demonstrating the new Sweeper API
func main() {
	// Flags/environment
	destFlag := flag.String("dest", "", "destination address for spend (overrides DEST_ADDR env)")
	testMode := flag.Bool("testmode", true, "enable test mode (skip strict address validation)")
	noPubKeyCheck := flag.Bool("no-pubkey-check", true, "disable enforcing that inputs match configured pubkey")
	flag.Parse()

	destAddr := os.Getenv("DEST_ADDR")
	if *destFlag != "" {
		destAddr = *destFlag
	}
	if destAddr == "" {
		destAddr = DEFAULT_DEST_ADDR
	}
	// Read UTXOs from file
	var utxos []UTXO
	if err := json.Unmarshal(mustReadFile("utxos.json"), &utxos); err != nil {
		fmt.Fprintf(os.Stderr, "bad utxos.json: %v\n", err)
		os.Exit(1)
	}

	// Create sweeper instance
	// In real usage, this would be a real public key
	testPubKey := []byte("test_public_key_32_bytes_long_here")
	sweeper := NewSweeper(testPubKey, BitcoinTestnet)

	// Configure sweeper
	sweeper.SetFeeRate(5)
	sweeper.SetDustRate(600, 0.50, 55000)
	sweeper.SetUnconfirmedPolicy(true, 2, 2)
	sweeper.SetTestMode(*testMode)
	sweeper.SetPubKeyCheck(!*noPubKeyCheck)

	// Index UTXOs
	fmt.Println("Indexing UTXOs...")
	for i, utxo := range utxos {
		if err := sweeper.Index(utxo); err != nil {
			fmt.Printf("Failed to index UTXO %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Indexed UTXO %d: %s:%d (%d sats)\n", i, utxo.TxID, utxo.Vout, utxo.ValueSats)
	}

	// Create spending transaction
	outputs := []TxOutput{
		{Address: destAddr, ValueSats: 150_000},
	}

	fmt.Println("\nCreating spending transaction...")
	plan, err := sweeper.Spend(outputs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spending failed: %v\n", err)
		os.Exit(1)
	}

	// Encode PSBT
	psbtB64, err := plan.PSBT.B64Encode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "psbt encode failed: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Println("\nTransaction Plan:")
	fmt.Println("Inputs:", plan.Inputs)
	fmt.Println("Outputs:", plan.Outputs)
	fmt.Println("Fee (sats):", plan.FeeSats)
	fmt.Println("PSBT (b64):", psbtB64)

	// Show chain depth
	fmt.Println("\nChain Depth:", sweeper.PendingChainDepth())
}

// Helper function to read file
func mustReadFile(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't read %s: %v\n", path, err)
		os.Exit(1)
	}
	return b
}
