# utxo-sweeper-go
Offline-first Go module that plans Bitcoin spends: coin selection, dust filtering ($0.50+), sweeping to a central wallet, split change, optional unconfirmed chaining, and PSBT output.

# instructions
```
go mod init utxo_sweeper
go get github.com/btcsuite/btcd@v0.24.2
go mod tidy
go run utxo_sweeper.go
```
