# utxo-sweeper-go
Offline-first Go module that plans Bitcoin spends: coin selection, dust filtering ($0.50+), sweeping to a central wallet, split change, optional unconfirmed chaining, and PSBT output.

## Features
- **No External Dependencies**: Self-contained implementation of Bitcoin primitives
- **Instance-Based API**: Easy-to-use `Sweeper` struct with methods like `Index()`, `Spend()`, `SetFeeRate()`
- **Multi-Network Support**: Bitcoin/Litecoin mainnet/testnet with proper address derivation
- **Dust Filtering**: Configurable dust thresholds in USD and satoshis
- **Unconfirmed Chain Tracking**: Prevents spending too many unconfirmed transactions
- **PSBT Output**: Ready for external signing
 - **Consolidation**: Sweep all indexed UTXOs to a single address
 - **Distribution Strategies**: Even or weighted distribution to multiple outputs
 - **Multi-Wallet Allocation**: Persist and spend by wallet weights; weighted change allocation

## Project Structure
- `main.go` - Main program demonstrating the API
- `sweeper.go` - Core Sweeper instance and API
- `bitcoin.go` - Bitcoin primitives (Bech32, address derivation, validation)
- `transaction.go` - Transaction and PSBT serialization
- `utxos.json` - Sample UTXO data for testing

## Usage
```bash
# Build and run
go build
./utxo_sweeper

# Or run directly
go run .
```

## API Example
```go
// Create sweeper instance
sweeper := NewSweeper(pubKey, BitcoinTestnet)

// Configure
sweeper.SetFeeRate(5)
sweeper.SetDustRate(600, 0.50, 55000)
sweeper.SetUnconfirmedPolicy(true, 2, 2)
// Optional change handling
sweeper.SetChangeSplit(3, 60_000, 20_000) // parts, targetChunkSats, minChunkSats
sweeper.SetAllocationWeights([]WeightedAddr{{Address: "tb1...A", WeightBP: 6000}, {Address: "tb1...B", WeightBP: 4000}})

// Index UTXOs
sweeper.Index(utxo)

// Create spending transaction
plan, err := sweeper.Spend(outputs)

// Consolidate all to a single destination
plan, err = sweeper.ConsolidateAll("tb1...")

// Evenly distribute a total across addresses
plan, err = sweeper.SpendEven([]string{"tb1...A", "tb1...B"}, 200_000, 20_000)

// Weighted distribution by temporary weights
plan, err = sweeper.SpendWeighted([]WeightedAddr{{"tb1...A", 7000}, {"tb1...B", 3000}}, 300_000, 20_000)

// Persist multi-wallet allocation and spend by wallets
_ = sweeper.SetSpendingWallets([]WeightedAddr{{Address: "tb1...A", WeightBP: 7000}, {Address: "tb1...B", WeightBP: 3000}})
_ = sweeper.LoadSpendingWallets()
plan, err = sweeper.SpendToWallets(500_000, 20_000)
```

## Notes
- For development, `SetTestMode(true)` can be used to bypass strict address validation while wiring flows.
- Bech32/Bech32m, TX and PSBT serialization are implemented in-repo without external dependencies.
