# utxo-sweeper-go
Offline-first Go module that plans Bitcoin spends: coin selection, dust filtering ($0.50+), sweeping to a central wallet, split change, optional unconfirmed chaining, P2WPKH/P2TR support, and PSBT output.

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

### CLI Flags
- `-config string`: Path to JSON config file (default: `config.json`)
- `-dest string`: Destination address override (or use `DEST_ADDR`)
- `-pubkey string`: 33-byte compressed pubkey hex for P2WPKH (or `PUBKEY_HEX`)
- `-taproot_xonly string`: 32-byte x-only key hex for P2TR change (or `TAPROOT_XONLY_HEX`)
- `-help`: Show help
- `-version`: Show version

Environment variables:
- `DEST_ADDR`, `PUBKEY_HEX`, `TAPROOT_XONLY_HEX`

Examples:
```bash
# Basic
utxo-sweeper

# Provide pubkey (compressed 33-byte hex)
utxo-sweeper -pubkey 02a163...33bytes...

# Provide Taproot x-only change key (32-byte hex)
utxo-sweeper -taproot_xonly 79be66...32bytes

# JSON output
utxo-sweeper -config config.json | jq '.transaction_plan.fee_sats'
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
  - Bech32 uses witness-version-aware checksums (BIP-173/350)
  - Tx serialization supports segwit marker/flag and witness stacks
  - PSBT encodes typed key-value maps per BIP-174
 
## Configuration
`config.json` supports:
- `network`: `bitcoin_mainnet` | `bitcoin_testnet` | `litecoin_mainnet` | `litecoin_testnet`
- `fee_rate`: sat/vB integer
- `dust_threshold_usd`, `price_usd_per_btc`
- `allow_unconfirmed`, `max_unconfirmed`, `max_chain_depth`
- `change_split_parts`, `target_chunk_sats`, `min_chunk_sats`
- `output_format`: `human` | `json`
- `test_mode`: boolean, `enforce_pubkey`: boolean

Example:
```json
{
  "network": "bitcoin_testnet",
  "fee_rate": 5,
  "dust_threshold_usd": 0.5,
  "price_usd_per_btc": 55000,
  "allow_unconfirmed": true,
  "max_unconfirmed": 2,
  "max_chain_depth": 2,
  "change_split_parts": 1,
  "target_chunk_sats": 60000,
  "min_chunk_sats": 20000,
  "output_format": "human",
  "test_mode": true,
  "enforce_pubkey": false
}
```

## Production Checklist
- Provide a real 33-byte compressed pubkey via `-pubkey` or `PUBKEY_HEX`.
- Optionally set a Taproot x-only change key with `-taproot_xonly`/`TAPROOT_XONLY_HEX`.
- Set `test_mode=false` and `enforce_pubkey=true` in config for strict validation.
- Verify fee rate policy and dust thresholds for your environment.
- Review outputs and PSBT in JSON mode before broadcasting/signed handoff.

## Limitations
- Signing is out of scope; the tool emits PSBT for external signers.
- Fee estimator is an approximation (accounts for P2WPKH vs P2TR); validate for edge cases.
- Persistence is in-memory (`MemKV`) for demo; integrate a real KV for production usage.

## File Notes
- Prefer `config.json`; any similarly named sample files are illustrative only.
