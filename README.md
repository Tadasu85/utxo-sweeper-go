# utxo-sweeper-go
Offline-first Go module that plans Bitcoin spends: coin selection, dust filtering ($0.50+), sweeping to a central wallet, split change, optional unconfirmed chaining, and PSBT output.

## Features
- **No External Dependencies**: Self-contained implementation of Bitcoin primitives
- **Instance-Based API**: Easy-to-use `Sweeper` struct with methods like `Index()`, `Spend()`, `SetFeeRate()`
- **Multi-Network Support**: Bitcoin/Litecoin mainnet/testnet with proper address derivation
- **Dust Filtering**: Configurable dust thresholds in USD and satoshis
- **Unconfirmed Chain Tracking**: Prevents spending too many unconfirmed transactions
- **PSBT Output**: Ready for external signing

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

// Index UTXOs
sweeper.Index(utxo)

// Create spending transaction
plan, err := sweeper.Spend(outputs)
```
