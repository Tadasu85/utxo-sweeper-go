package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

// Core types
type UTXO struct {
	TxID      string
	Vout      uint32
	ValueSats int64
	Address   string
	Confirmed bool
}

type TxOutput struct {
	Address   string
	ValueSats int64
}

type WeightedAddr struct {
	Address  string
	WeightBP int
}

type TransactionPlan struct {
	Inputs     []UTXO
	Outputs    []TxOutput
	FeeSats    int64
	RawTx      *MsgTx
	PSBT       *PSBT
	ChangeIdxs []int
}

type Opts struct {
	FeeRateSatsVB       int64
	MinDustSats         int64
	MinUSD              float64
	PriceUSDPerBTC      float64
	AllowUnconfirmed    bool
	MaxUnconfInputs     int
	ChangeSplitParts    int
	TargetChunkSats     int64
	MinChunkSats        int64
	AllocationByWeights []WeightedAddr
	MaxChainChildren    int
}

type KV interface {
	Put(key, value []byte) error
	Get(key []byte) ([]byte, error)
}

type MemKV struct{ m map[string][]byte }

func NewMemKV() *MemKV                   { return &MemKV{m: map[string][]byte{}} }
func (k *MemKV) Put(key, v []byte) error { k.m[string(key)] = v; return nil }
func (k *MemKV) Get(key []byte) ([]byte, error) {
	v, ok := k.m[string(key)]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

// Sweeper instance
type Sweeper struct {
	// Configuration
	pubKey           []byte
	network          Network
	asset            Asset
	feeRateSatsVB    int64
	minDustSats      int64
	minUSD           float64
	priceUSDPerBTC   float64
	allowUnconfirmed bool
	maxUnconfInputs  int
	maxChainDepth    int
	testMode         bool // Skip strict address validation for testing
	enforcePubKey    bool // Enforce that addresses match configured public key

	// Change/output allocation strategy
	changeSplitParts    int
	targetChunkSats     int64
	minChunkSats        int64
	allocationByWeights []WeightedAddr

	// State
	kv           KV
	indexedUTXOs []UTXO
	chainDepth   map[string]int // txid -> depth
}

// NewSweeper creates a new sweeper instance
func NewSweeper(pubKey []byte, network Network) *Sweeper {
	return &Sweeper{
		pubKey:           pubKey,
		network:          network,
		asset:            getAssetFromNetwork(network),
		feeRateSatsVB:    5, // default 5 sat/vB
		minDustSats:      600,
		minUSD:           0.50,
		priceUSDPerBTC:   55000,
		allowUnconfirmed: true,
		maxUnconfInputs:  2,
		maxChainDepth:    2,
		kv:               NewMemKV(),
		indexedUTXOs:     make([]UTXO, 0),
		chainDepth:       make(map[string]int),
		enforcePubKey:    true,
	}
}

// Get asset from network
func getAssetFromNetwork(network Network) Asset {
	switch network {
	case BitcoinMainnet, BitcoinTestnet:
		return BTC
	case LitecoinMainnet, LitecoinTestnet:
		return LTC
	default:
		return BTC
	}
}

// SetFeeRate sets the fee rate in satoshis per vbyte
func (s *Sweeper) SetFeeRate(rate int64) error {
	if rate <= 0 {
		return errors.New("fee rate must be positive")
	}
	s.feeRateSatsVB = rate
	return nil
}

// SetDustRate sets the dust threshold
func (s *Sweeper) SetDustRate(sats int64, usd float64, priceUSDPerBTC float64) {
	s.minDustSats = sats
	s.minUSD = usd
	s.priceUSDPerBTC = priceUSDPerBTC
}

// SetNetwork sets the network
func (s *Sweeper) SetNetwork(network Network) {
	s.network = network
	s.asset = getAssetFromNetwork(network)
}

// SetPubKey sets the public key
func (s *Sweeper) SetPubKey(pubKey []byte) {
	s.pubKey = pubKey
}

// SetTestMode enables test mode (skips strict address validation)
func (s *Sweeper) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// SetPubKeyCheck enables/disables enforcing that addresses match the configured public key
func (s *Sweeper) SetPubKeyCheck(enabled bool) {
	s.enforcePubKey = enabled
}

// SetUnconfirmedPolicy sets unconfirmed transaction policy
func (s *Sweeper) SetUnconfirmedPolicy(allow bool, maxInputs int, maxDepth int) {
	s.allowUnconfirmed = allow
	s.maxUnconfInputs = maxInputs
	s.maxChainDepth = maxDepth
}

// SetChangeSplit configures splitting of change outputs
func (s *Sweeper) SetChangeSplit(parts int, targetChunkSats, minChunkSats int64) {
	s.changeSplitParts = parts
	s.targetChunkSats = targetChunkSats
	s.minChunkSats = minChunkSats
}

// SetAllocationWeights sets allocation weights for distributing change across addresses
func (s *Sweeper) SetAllocationWeights(weights []WeightedAddr) {
	s.allocationByWeights = append([]WeightedAddr(nil), weights...)
}

// SetSpendingWallets persists allocation weights for multi-wallet change distribution
func (s *Sweeper) SetSpendingWallets(weights []WeightedAddr) error {
	// basic validation
	if len(weights) == 0 {
		return errors.New("weights cannot be empty")
	}
	for i := range weights {
		if weights[i].WeightBP <= 0 {
			return fmt.Errorf("weight at index %d must be > 0", i)
		}
		if !s.testMode {
			if _, err := DecodeAddress(weights[i].Address); err != nil {
				return fmt.Errorf("bad address at index %d: %w", i, err)
			}
		}
	}
	s.allocationByWeights = append([]WeightedAddr(nil), weights...)
	b, _ := json.Marshal(weights)
	return s.kv.Put([]byte("alloc:weights"), b)
}

// LoadSpendingWallets loads persisted allocation weights
func (s *Sweeper) LoadSpendingWallets() error {
	b, err := s.kv.Get([]byte("alloc:weights"))
	if err != nil {
		return err
	}
	var ws []WeightedAddr
	if e := json.Unmarshal(b, &ws); e != nil {
		return e
	}
	s.allocationByWeights = append([]WeightedAddr(nil), ws...)
	return nil
}

// SpendToWallets creates outputs to the configured wallets by weights
func (s *Sweeper) SpendToWallets(totalSats int64, minChunk int64) (*TransactionPlan, error) {
	if len(s.allocationByWeights) == 0 {
		return nil, errors.New("no wallet weights configured")
	}
	outs := buildWeightedOutputs(totalSats, s.allocationByWeights, minChunk)
	if len(outs) == 0 {
		return nil, errors.New("no outputs after weighting")
	}
	return s.Spend(outs)
}

// Index adds a UTXO to the index
func (s *Sweeper) Index(utxo UTXO) error {
	// Validate address against public key
	if err := s.validateUTXOAddress(utxo); err != nil {
		return fmt.Errorf("address validation failed: %w", err)
	}

	// Check dust threshold
	if err := s.checkDustThreshold(utxo); err != nil {
		return fmt.Errorf("dust threshold check failed: %w", err)
	}

	// Check unconfirmed policy
	if !utxo.Confirmed && !s.allowUnconfirmed {
		return errors.New("unconfirmed UTXOs not allowed")
	}

	// Check chain depth for unconfirmed UTXOs
	if !utxo.Confirmed {
		depth := s.getChainDepth(utxo.TxID)
		if depth >= s.maxChainDepth {
			return fmt.Errorf("chain depth %d exceeds maximum %d", depth, s.maxChainDepth)
		}
	}

	// Add to index
	s.indexedUTXOs = append(s.indexedUTXOs, utxo)

	// Store in KV
	key := fmt.Sprintf("utxo:%s:%d", utxo.TxID, utxo.Vout)
	data, _ := json.Marshal(utxo)
	s.kv.Put([]byte(key), data)

	return nil
}

// Validate UTXO address against public key
func (s *Sweeper) validateUTXOAddress(utxo UTXO) error {
	// Skip validation in test mode
	if s.testMode {
		return nil
	}

	// Decode address
	addr, err := DecodeAddress(utxo.Address)
	if err != nil {
		return err
	}

	// Check network match
	if addr.Network != s.network {
		return errors.New("address network mismatch")
	}

	// Validate against public key
	if s.enforcePubKey {
		return ValidateAddress(utxo.Address, s.pubKey, s.network)
	}
	return nil
}

// Check dust threshold
func (s *Sweeper) checkDustThreshold(utxo UTXO) error {
	dustUSD := dustFromUSD(s.minUSD, s.priceUSDPerBTC)
	dust := s.minDustSats
	if dustUSD > dust {
		dust = dustUSD
	}

	if utxo.ValueSats < dust {
		return fmt.Errorf("UTXO value %d below dust threshold %d", utxo.ValueSats, dust)
	}

	return nil
}

// Get chain depth for a transaction
func (s *Sweeper) getChainDepth(txid string) int {
	if depth, exists := s.chainDepth[txid]; exists {
		return depth
	}
	return 0
}

// Set chain depth for a transaction
func (s *Sweeper) setChainDepth(txid string, depth int) {
	s.chainDepth[txid] = depth
}

// Spend creates a spending transaction
func (s *Sweeper) Spend(outputs []TxOutput) (*TransactionPlan, error) {
	if len(outputs) == 0 {
		return nil, errors.New("no outputs specified")
	}

	// Validate outputs
	for i, output := range outputs {
		if !s.testMode {
			if _, err := DecodeAddress(output.Address); err != nil {
				return nil, fmt.Errorf("invalid output address at index %d: %w", i, err)
			}
		}
		if output.ValueSats <= 0 {
			return nil, fmt.Errorf("invalid output value at index %d: %d", i, output.ValueSats)
		}
	}

	// Get change address
	changeAddr, err := s.getChangeAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get change address: %w", err)
	}

	// Build transaction
	return s.buildTransaction(s.indexedUTXOs, outputs, changeAddr)
}

// Get change address
func (s *Sweeper) getChangeAddress() (string, error) {
	if s.testMode {
		return "tb1test_change_address", nil
	}
	return DeriveChangeAddress(s.pubKey, s.network)
}

// Build transaction (refactored from original)
func (s *Sweeper) buildTransaction(utxos []UTXO, outputs []TxOutput, changeAddr string) (*TransactionPlan, error) {
	// Calculate dust threshold
	dustUSD := dustFromUSD(s.minUSD, s.priceUSDPerBTC)
	dust := s.minDustSats
	if dustUSD > dust {
		dust = dustUSD
	}
	if dust <= 0 {
		dust = 600
	}

	// Calculate total output value
	totalOut := int64(0)
	for _, o := range outputs {
		totalOut += o.ValueSats
	}
	if totalOut <= 0 {
		return nil, errors.New("outputs total must be > 0")
	}

	// Select UTXOs
	selected, totalIn, estFee, err := s.selectUTXOsFor(totalOut, utxos, dust, len(outputs))
	if err != nil {
		return nil, err
	}

	// Calculate change
	change := totalIn - totalOut - estFee

	// Build final outputs
	finalOutputs := make([]TxOutput, 0, len(outputs)+8)
	finalOutputs = append(finalOutputs, outputs...)

	changeIdxs := []int{}
	if change > dust {
		// Weighted allocation of change across specified addresses
		if len(s.allocationByWeights) > 0 {
			ws := buildWeightedOutputs(change, s.allocationByWeights, max64(1, dust))
			for _, w := range ws {
				finalOutputs = append(finalOutputs, w)
				changeIdxs = append(changeIdxs, len(finalOutputs)-1)
			}
		} else if s.changeSplitParts > 1 && s.minChunkSats > 0 {
			parts := s.changeSplitParts
			if s.targetChunkSats > 0 {
				guess := int(change / s.targetChunkSats)
				if guess >= 2 {
					parts = guess
				}
			}
			chunks := splitEven(change, parts, max64(s.minChunkSats, dust))
			for _, c := range chunks {
				if c >= dust {
					finalOutputs = append(finalOutputs, TxOutput{Address: changeAddr, ValueSats: c})
					changeIdxs = append(changeIdxs, len(finalOutputs)-1)
				}
			}
			if len(changeIdxs) == 0 {
				finalOutputs = append(finalOutputs, TxOutput{Address: changeAddr, ValueSats: change})
				changeIdxs = append(changeIdxs, len(finalOutputs)-1)
			}
		} else {
			// Single change output
			finalOutputs = append(finalOutputs, TxOutput{Address: changeAddr, ValueSats: change})
			changeIdxs = append(changeIdxs, len(finalOutputs)-1)
		}
	}

	// Recalculate fee with final outputs
	nIn := len(selected)
	nOut := len(finalOutputs)
	vbytes := estimateTxVBytes(nIn, nOut)
	finalFee := vbytes * s.feeRateSatsVB

	// Adjust change for final fee
	changeDelta := (totalIn - totalOut) - finalFee
	if changeDelta < 0 {
		return nil, errors.New("final fee overshoots; add UTXOs or reduce outputs")
	}

	if len(changeIdxs) == 1 {
		finalOutputs[changeIdxs[0]].ValueSats += changeDelta
	} else if len(changeIdxs) == 0 {
		finalFee = totalIn - totalOut
	}

	// Build transaction
	tx := NewMsgTx(2) // version 2

	// Add inputs
	for _, in := range selected {
		outpoint, err := NewOutPointFromStr(in.TxID, in.Vout)
		if err != nil {
			return nil, fmt.Errorf("invalid txid: %s (%w)", in.TxID, err)
		}
		txin := TxIn{
			PreviousOutPoint: outpoint,
			SignatureScript:  nil,
			Witness:          nil,
			Sequence:         0xffffffff,
		}
		tx.AddTxIn(txin)
	}

	// Add outputs
	for _, out := range finalOutputs {
		script, err := s.buildOutputScript(out.Address)
		if err != nil {
			return nil, fmt.Errorf("bad output script %s (%w)", out.Address, err)
		}
		txout := TxOut{
			Value:    out.ValueSats,
			PkScript: script,
		}
		tx.AddTxOut(txout)
	}

	// Create PSBT
	psbt := NewPSBTFromUnsignedTx(tx)

	// Set witness UTXOs
	for i, in := range selected {
		script, err := s.buildOutputScript(in.Address)
		if err != nil {
			return nil, err
		}
		psbt.Inputs[i].WitnessUtxo = &TxOut{
			Value:    in.ValueSats,
			PkScript: script,
		}
	}

	// Update chain depth for unconfirmed inputs
	for _, in := range selected {
		if !in.Confirmed {
			s.setChainDepth(in.TxID, s.getChainDepth(in.TxID)+1)
		}
	}

	return &TransactionPlan{
		Inputs:     selected,
		Outputs:    finalOutputs,
		FeeSats:    finalFee,
		RawTx:      tx,
		PSBT:       psbt,
		ChangeIdxs: changeIdxs,
	}, nil
}

// Build output script for address
func (s *Sweeper) buildOutputScript(addr string) ([]byte, error) {
	// In test mode, return a simple script
	if s.testMode {
		// Return a simple P2WPKH script for testing
		return []byte{0x00, 0x14, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13}, nil
	}

	decoded, err := DecodeAddress(addr)
	if err != nil {
		return nil, err
	}

	switch decoded.Type {
	case P2WPKH:
		return BuildP2WPKHScript(decoded.Data), nil
	case P2TR:
		return BuildP2TRScript(decoded.Data), nil
	default:
		return nil, errors.New("unsupported address type")
	}
}

// Select UTXOs for spending
func (s *Sweeper) selectUTXOsFor(targetOutSats int64, utxos []UTXO, dust int64, nFixedOutputs int) ([]UTXO, int64, int64, error) {
	// Filter UTXOs
	cands := s.filterUTXOs(utxos, dust)
	if len(cands) == 0 {
		return nil, 0, 0, errors.New("no spendable UTXOs after filters")
	}

	// Greedy selection
	var selected []UTXO
	totalIn := int64(0)

	for i := 0; i < len(cands); i++ {
		selected = append(selected, cands[i])
		totalIn += cands[i].ValueSats
		nIn := len(selected)
		nOut := nFixedOutputs + 1
		estVBytes := estimateTxVBytes(nIn, nOut)
		fee := estVBytes * s.feeRateSatsVB

		if totalIn >= targetOutSats+fee {
			return selected, totalIn, fee, nil
		}
	}

	return nil, 0, 0, errors.New("balance is not enough for outputs + fee")
}

// Filter UTXOs based on dust and unconfirmed policy
func (s *Sweeper) filterUTXOs(utxos []UTXO, minValue int64) []UTXO {
	var res []UTXO
	unconf := 0

	// Sort by value (ascending)
	cpy := make([]UTXO, len(utxos))
	copy(cpy, utxos)
	sort.Slice(cpy, func(i, j int) bool {
		return cpy[i].ValueSats < cpy[j].ValueSats
	})

	for _, u := range cpy {
		if u.ValueSats < minValue {
			continue
		}
		if !s.allowUnconfirmed && !u.Confirmed {
			continue
		}
		if s.allowUnconfirmed && !u.Confirmed {
			if unconf >= s.maxUnconfInputs {
				continue
			}
			unconf++
		}
		res = append(res, u)
	}

	return res
}

// ConsolidateAll sweeps all indexed UTXOs into a single destination address (no change)
func (s *Sweeper) ConsolidateAll(destAddr string) (*TransactionPlan, error) {
	if !s.testMode {
		if _, err := DecodeAddress(destAddr); err != nil {
			return nil, fmt.Errorf("invalid destination address: %w", err)
		}
	}
	// Dust threshold
	dustUSD := dustFromUSD(s.minUSD, s.priceUSDPerBTC)
	dust := s.minDustSats
	if dustUSD > dust {
		dust = dustUSD
	}
	cands := s.filterUTXOs(s.indexedUTXOs, dust)
	if len(cands) == 0 {
		return nil, errors.New("no spendable UTXOs to consolidate")
	}
	// Sum inputs
	totalIn := int64(0)
	for _, u := range cands {
		totalIn += u.ValueSats
	}
	// Estimate fee for nIn inputs and 1 output
	vbytes := estimateTxVBytes(len(cands), 1)
	fee := vbytes * s.feeRateSatsVB
	if totalIn <= fee || (totalIn-fee) < dust {
		return nil, errors.New("balance too low after fees for consolidation")
	}
	// Build single-output plan
	outputs := []TxOutput{{Address: destAddr, ValueSats: totalIn - fee}}
	// Build raw tx and psbt
	tx := NewMsgTx(2)
	for _, in := range cands {
		op, err := NewOutPointFromStr(in.TxID, in.Vout)
		if err != nil {
			return nil, fmt.Errorf("invalid txid: %w", err)
		}
		tx.AddTxIn(TxIn{PreviousOutPoint: op, Sequence: 0xffffffff})
	}
	script, err := s.buildOutputScript(destAddr)
	if err != nil {
		return nil, err
	}
	tx.AddTxOut(TxOut{Value: outputs[0].ValueSats, PkScript: script})
	psbt := NewPSBTFromUnsignedTx(tx)
	for i, in := range cands {
		sc, err := s.buildOutputScript(in.Address)
		if err != nil {
			return nil, err
		}
		psbt.Inputs[i].WitnessUtxo = &TxOut{Value: in.ValueSats, PkScript: sc}
	}
	for _, in := range cands {
		if !in.Confirmed {
			s.setChainDepth(in.TxID, s.getChainDepth(in.TxID)+1)
		}
	}
	return &TransactionPlan{Inputs: cands, Outputs: outputs, FeeSats: fee, RawTx: tx, PSBT: psbt, ChangeIdxs: nil}, nil
}

// SpendEven builds evenly distributed outputs across provided addresses and spends
func (s *Sweeper) SpendEven(destAddrs []string, totalSats int64, minChunk int64) (*TransactionPlan, error) {
	if len(destAddrs) == 0 {
		return nil, errors.New("no destination addresses")
	}
	chunks := splitEven(totalSats, len(destAddrs), minChunk)
	if len(chunks) == 0 {
		return nil, errors.New("unable to build even chunks")
	}
	// Map chunks to addresses (truncate or stop at min(len))
	outs := make([]TxOutput, 0, len(chunks))
	limit := len(chunks)
	if limit > len(destAddrs) {
		limit = len(destAddrs)
	}
	for i := 0; i < limit; i++ {
		outs = append(outs, TxOutput{Address: destAddrs[i], ValueSats: chunks[i]})
	}
	return s.Spend(outs)
}

// SpendWeighted distributes a total across weighted addresses and spends
func (s *Sweeper) SpendWeighted(weights []WeightedAddr, totalSats int64, minChunk int64) (*TransactionPlan, error) {
	outs := buildWeightedOutputs(totalSats, weights, minChunk)
	if len(outs) == 0 {
		return nil, errors.New("no outputs after weighting")
	}
	return s.Spend(outs)
}

// Get indexed UTXOs
func (s *Sweeper) GetIndexedUTXOs() []UTXO {
	return s.indexedUTXOs
}

// Get pending chain depth
func (s *Sweeper) PendingChainDepth() map[string]int {
	return s.chainDepth
}

// Clear index
func (s *Sweeper) ClearIndex() {
	s.indexedUTXOs = make([]UTXO, 0)
	s.chainDepth = make(map[string]int)
}

// Helper functions (from original)
func dustFromUSD(minUSD, price float64) int64 {
	if minUSD <= 0 || price <= 0 {
		return 0
	}
	sats := (minUSD / price) * 1e8
	return int64(math.Ceil(sats))
}

func estimateTxVBytes(nIn, nOut int) int64 {
	const (
		baseOverheadVBytes = 10
		inVBytesTaproot    = 58
		outVBytes          = 31
	)
	return int64(baseOverheadVBytes + nIn*inVBytesTaproot + nOut*outVBytes)
}

// Utilities
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func splitEven(value int64, parts int, minChunk int64) []int64 {
	if parts <= 1 || value <= 0 {
		return []int64{value}
	}
	chunk := value / int64(parts)
	if chunk < minChunk {
		parts = int(value / minChunk)
		if parts < 1 {
			parts = 1
		}
		chunk = value / int64(parts)
	}
	out := make([]int64, parts)
	rem := value
	for i := 0; i < parts; i++ {
		out[i] = chunk
		rem -= chunk
	}
	for i := 0; i < len(out) && rem > 0; i++ {
		out[i]++
		rem--
	}
	res := out[:0]
	for _, v := range out {
		if v > 0 {
			res = append(res, v)
		}
	}
	return res
}

func buildWeightedOutputs(total int64, ws []WeightedAddr, minChunk int64) []TxOutput {
	if len(ws) == 0 || total <= 0 {
		return nil
	}
	sum := 0
	for _, w := range ws {
		sum += w.WeightBP
	}
	if sum <= 0 {
		return nil
	}
	var outs []TxOutput
	acc := int64(0)
	for i, w := range ws {
		share := (total * int64(w.WeightBP)) / int64(sum)
		if i == len(ws)-1 {
			share = total - acc
		}
		if share >= minChunk {
			outs = append(outs, TxOutput{Address: w.Address, ValueSats: share})
			acc += share
		}
	}
	return outs
}
