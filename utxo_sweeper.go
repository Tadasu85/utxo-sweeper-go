package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

var netParams = &chaincfg.TestNet3Params

const DEST_ADDR = "tb1pgzu7n6lvqqzrl4un25tqpgzd4e7hqcas822kgsurldk5lzfh7d4qptrhts"
const CHANGE_ADDR = "tb1pgzu7n6lvqqzrl4un25tqpgzd4e7hqcas822kgsurldk5lzfh7d4qptrhts"

const (
	baseOverheadVBytes = 10
	inVBytesTaproot    = 58
	outVBytes          = 31
)

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
	RawTx      *wire.MsgTx
	PSBT       *psbt.Packet
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

func NewMemKV() *MemKV                                  { return &MemKV{m: map[string][]byte{}} }
func (k *MemKV) Put(key, v []byte) error                { k.m[string(key)] = v; return nil }
func (k *MemKV) Get(key []byte) ([]byte, error)         { v, ok := k.m[string(key)]; if !ok { return nil, errors.New("not found") }; return v, nil }
func estimateTxVBytes(nIn, nOut int) int64              { return int64(baseOverheadVBytes + nIn*inVBytesTaproot + nOut*outVBytes) }
func payToAddrScript(addrStr string) ([]byte, error)    { a, e := btcutil.DecodeAddress(addrStr, netParams); if e != nil { return nil, e }; return txscript.PayToAddrScript(a) }
func dustFromUSD(minUSD, price float64) int64           { if minUSD <= 0 || price <= 0 { return 0 }; sats := (minUSD / price) * 1e8; return int64(math.Ceil(sats)) }
func splitEven(value int64, parts int, minChunk int64) []int64 {
	if parts <= 1 || value <= 0 { return []int64{value} }
	chunk := value / int64(parts)
	if chunk < minChunk {
		parts = int(value / minChunk)
		if parts < 1 { parts = 1 }
		chunk = value / int64(parts)
	}
	out := make([]int64, parts)
	rem := value
	for i := 0; i < parts; i++ { out[i] = chunk; rem -= chunk }
	for i := 0; i < len(out) && rem > 0; i++ { out[i]++; rem-- }
	res := out[:0]
	for _, v := range out { if v > 0 { res = append(res, v) } }
	return res
}
func buildWeightedOutputs(total int64, ws []WeightedAddr, minChunk int64) []TxOutput {
	if len(ws) == 0 || total <= 0 { return nil }
	sum := 0
	for _, w := range ws { sum += w.WeightBP }
	if sum <= 0 { return nil }
	var outs []TxOutput
	acc := int64(0)
	for i, w := range ws {
		share := (total * int64(w.WeightBP)) / int64(sum)
		if i == len(ws)-1 { share = total - acc }
		if share >= minChunk {
			outs = append(outs, TxOutput{Address: w.Address, ValueSats: share})
			acc += share
		}
	}
	return outs
}
func validateAllAddresses(utxos []UTXO, outs []TxOutput, changeAddr string, weights []WeightedAddr) error {
	for i, u := range utxos {
		if _, err := btcutil.DecodeAddress(u.Address, netParams); err != nil {
			return fmt.Errorf("bad address in UTXO[%d]: %s (%w)", i, u.Address, err)
		}
	}
	for i, o := range outs {
		if _, err := btcutil.DecodeAddress(o.Address, netParams); err != nil {
			return fmt.Errorf("bad address in Output[%d]: %s (%w)", i, o.Address, err)
		}
	}
	if changeAddr != "" {
		if _, err := btcutil.DecodeAddress(changeAddr, netParams); err != nil {
			return fmt.Errorf("bad change address: %s (%w)", changeAddr, err)
		}
	}
	for i, w := range weights {
		if _, err := btcutil.DecodeAddress(w.Address, netParams); err != nil {
			return fmt.Errorf("bad address in Allocation[%d]: %s (%w)", i, w.Address, err)
		}
	}
	return nil
}
func filterUTXOs(utxos []UTXO, minValue int64, allowUnconf bool, maxUnconf int) []UTXO {
	var res []UTXO
	unconf := 0
	cpy := append([]UTXO(nil), utxos...)
	sort.Slice(cpy, func(i, j int) bool { return cpy[i].ValueSats < cpy[j].ValueSats })
	for _, u := range cpy {
		if u.ValueSats < minValue { continue }
		if !allowUnconf && !u.Confirmed { continue }
		if allowUnconf && !u.Confirmed {
			if unconf >= maxUnconf { continue }
			unconf++
		}
		res = append(res, u)
	}
	return res
}
func selectUTXOsFor(targetOutSats int64, feeRateSatsVB int64, utxos []UTXO, dust int64, allowUnconf bool, maxUnconf int, nFixedOutputs int) ([]UTXO, int64, int64, error) {
	cands := filterUTXOs(utxos, dust, allowUnconf, maxUnconf)
	if len(cands) == 0 { return nil, 0, 0, errors.New("no spendable UTXOs after filters") }
	var selected []UTXO
	totalIn := int64(0)
	for i := 0; i < len(cands); i++ {
		selected = append(selected, cands[i])
		totalIn += cands[i].ValueSats
		nIn := len(selected)
		nOut := nFixedOutputs + 1
		estVBytes := estimateTxVBytes(nIn, nOut)
		fee := estVBytes * feeRateSatsVB
		if totalIn >= targetOutSats+fee { return selected, totalIn, fee, nil }
	}
	return nil, 0, 0, errors.New("balance is not enough for outputs + fee")
}
func BuildTransaction(utxos []UTXO, outputs []TxOutput, opts Opts, changeAddr string) (TransactionPlan, error) {
	if opts.FeeRateSatsVB <= 0 { return TransactionPlan{}, errors.New("fee rate must be > 0") }
	dustUSD := dustFromUSD(opts.MinUSD, opts.PriceUSDPerBTC)
	dust := opts.MinDustSats
	if dustUSD > dust { dust = dustUSD }
	if dust <= 0 { dust = 600 }
	if err := validateAllAddresses(utxos, outputs, changeAddr, opts.AllocationByWeights); err != nil { return TransactionPlan{}, err }
	totalOut := int64(0)
	for _, o := range outputs { totalOut += o.ValueSats }
	if totalOut <= 0 { return TransactionPlan{}, errors.New("outputs total must be > 0") }
	selected, totalIn, estFee, err := selectUTXOsFor(totalOut, opts.FeeRateSatsVB, utxos, dust, opts.AllowUnconfirmed, opts.MaxUnconfInputs, len(outputs))
	if err != nil { return TransactionPlan{}, err }
	change := totalIn - totalOut - estFee
	finalOutputs := make([]TxOutput, 0, len(outputs)+8)
	finalOutputs = append(finalOutputs, outputs...)
	changeIdxs := []int{}
	if change > dust {
		if len(opts.AllocationByWeights) > 0 {
			ws := buildWeightedOutputs(change, opts.AllocationByWeights, max(1, dust))
			for _, w := range ws {
				finalOutputs = append(finalOutputs, w)
				changeIdxs = append(changeIdxs, len(finalOutputs)-1)
			}
		} else if opts.ChangeSplitParts > 1 && opts.MinChunkSats > 0 {
			parts := opts.ChangeSplitParts
			if opts.TargetChunkSats > 0 {
				guess := int(change / opts.TargetChunkSats)
				if guess >= 2 { parts = guess }
			}
			chunks := splitEven(change, parts, max(opts.MinChunkSats, dust))
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
			finalOutputs = append(finalOutputs, TxOutput{Address: changeAddr, ValueSats: change})
			changeIdxs = append(changeIdxs, len(finalOutputs)-1)
		}
	}
	nIn := len(selected)
	nOut := len(finalOutputs)
	vbytes := estimateTxVBytes(nIn, nOut)
	finalFee := vbytes * opts.FeeRateSatsVB
	changeDelta := (totalIn - totalOut) - finalFee
	if changeDelta < 0 { return TransactionPlan{}, errors.New("final fee overshoots; add UTXOs or reduce outputs") }
	if len(changeIdxs) == 1 {
		finalOutputs[changeIdxs[0]].ValueSats += changeDelta
	} else if len(changeIdxs) == 0 {
		finalFee = totalIn - totalOut
	} else {
		per := changeDelta / int64(len(changeIdxs))
		rem := changeDelta - per*int64(len(changeIdxs))
		for i, idx := range changeIdxs {
			add := per
			if int64(i) < rem { add++ }
			finalOutputs[idx].ValueSats += add
		}
	}
	tx := wire.NewMsgTx(wire.TxVersion)
	for _, in := range selected {
		h, e := chainhash.NewHashFromStr(in.TxID)
		if e != nil { return TransactionPlan{}, fmt.Errorf("invalid txid: %s (%w)", in.TxID, e) }
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(h, in.Vout), nil, nil))
	}
	for _, out := range finalOutputs {
		pk, e := payToAddrScript(out.Address)
		if e != nil { return TransactionPlan{}, fmt.Errorf("bad output script %s (%w)", out.Address, e) }
		tx.AddTxOut(wire.NewTxOut(out.ValueSats, pk))
	}
	pkt, e := psbt.NewFromUnsignedTx(tx)
	if e != nil { return TransactionPlan{}, e }
	if len(pkt.Inputs) != len(selected) { return TransactionPlan{}, errors.New("psbt inputs mismatch") }
	for i, in := range selected {
		pk, e := payToAddrScript(in.Address)
		if e != nil { return TransactionPlan{}, e }
		pkt.Inputs[i].WitnessUtxo = &wire.TxOut{Value: in.ValueSats, PkScript: pk}
	}
	return TransactionPlan{Inputs: selected, Outputs: finalOutputs, FeeSats: finalFee, RawTx: tx, PSBT: pkt, ChangeIdxs: changeIdxs}, nil
}
func BuildChildFromParentChange(parent TransactionPlan, opts Opts, dests []TxOutput, changeAddr string) (TransactionPlan, error) {
	if len(parent.ChangeIdxs) == 0 { return TransactionPlan{}, errors.New("no change outputs in parent") }
	var childUTXOs []UTXO
	parentTxid := parent.RawTx.TxHash().String()
	for _, idx := range parent.ChangeIdxs {
		o := parent.Outputs[idx]
		childUTXOs = append(childUTXOs, UTXO{
			TxID:      parentTxid,
			Vout:      uint32(idx),
			ValueSats: o.ValueSats,
			Address:   o.Address,
			Confirmed: false,
		})
	}
	return BuildTransaction(childUTXOs, dests, opts, changeAddr)
}
func max(a, b int64) int64 { if a > b { return a }; return b }
func mustReadFile(path string) []byte { b, err := os.ReadFile(path); if err != nil { fmt.Fprintf(os.Stderr, "can't read %s: %v\n", path, err); os.Exit(1) }; return b }

func main() {
	var utxos []UTXO
	if err := json.Unmarshal(mustReadFile("utxos.json"), &utxos); err != nil {
		fmt.Fprintf(os.Stderr, "bad utxos.json: %v\n", err)
		os.Exit(1)
	}
	outputs := []TxOutput{
		{Address: DEST_ADDR, ValueSats: 150_000},
	}
	opts := Opts{
		FeeRateSatsVB:    5,
		MinDustSats:      600,
		MinUSD:           0.50,
		PriceUSDPerBTC:   55000,
		AllowUnconfirmed: true,
		MaxUnconfInputs:  2,
		ChangeSplitParts: 3,
		TargetChunkSats:  60_000,
		MinChunkSats:     20_000,
		MaxChainChildren: 1,
		AllocationByWeights: nil,
	}
	plan, err := BuildTransaction(utxos, outputs, opts, CHANGE_ADDR)
	if err != nil { fmt.Fprintf(os.Stderr, "parent plan failed: %v\n", err); os.Exit(1) }
	psbtB64, err := plan.PSBT.B64Encode()
	if err != nil { fmt.Fprintf(os.Stderr, "parent psbt encode failed: %v\n", err); os.Exit(1) }
	fmt.Println("parent inputs:", plan.Inputs)
	fmt.Println("parent outputs:", plan.Outputs)
	fmt.Println("parent fee (sats):", plan.FeeSats)
	fmt.Println("parent psbt (b64):", psbtB64)

	if opts.MaxChainChildren > 0 && len(plan.ChangeIdxs) > 0 {
		childOutputs := []TxOutput{{Address: DEST_ADDR, ValueSats: 0}}
		var changeSum int64 = 0
		for _, idx := range plan.ChangeIdxs { changeSum += plan.Outputs[idx].ValueSats }
		chunks := splitEven(changeSum, opts.ChangeSplitParts, max(opts.MinChunkSats, opts.MinDustSats))
		childOutputs = childOutputs[:0]
		for _, c := range chunks { childOutputs = append(childOutputs, TxOutput{Address: CHANGE_ADDR, ValueSats: c}) }
		childPlan, err := BuildChildFromParentChange(plan, opts, childOutputs, CHANGE_ADDR)
		if err == nil {
			psbtChild, err2 := childPlan.PSBT.B64Encode()
			if err2 == nil {
				fmt.Println("child inputs:", childPlan.Inputs)
				fmt.Println("child outputs:", childPlan.Outputs)
				fmt.Println("child fee (sats):", childPlan.FeeSats)
				fmt.Println("child psbt (b64):", psbtChild)
			}
		}
	}
}
