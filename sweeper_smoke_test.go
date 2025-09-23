package main

import (
	"bytes"
	"testing"
)

func TestBech32DecodeValidInvalid(t *testing.T) {
	// Build a valid testnet P2WPKH from a fake pubkey hash
	addrOK, err := CreateP2WPKH(Hash160([]byte("pubkey")), BitcoinTestnet)
	if err != nil {
		t.Fatalf("CreateP2WPKH: %v", err)
	}
	if _, _, err := Bech32Decode(addrOK); err != nil {
		t.Fatalf("Bech32Decode valid failed: %v", err)
	}
	// Invalid: mixed case
	if _, _, err := Bech32Decode("Tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"); err == nil {
		t.Fatalf("expected mixed-case error")
	}
}

func TestTxSerializationHashes(t *testing.T) {
	tx := NewMsgTx(2)
	// 1 dummy input
	tx.AddTxIn(TxIn{PreviousOutPoint: OutPoint{}, Sequence: 0xffffffff})
	// 1 dummy output
	tx.AddTxOut(TxOut{Value: 1000, PkScript: []byte{0x00, 0x14, 0xaa}})

	h1 := tx.TxHash()
	// Add witness stack to create wtxid difference
	tx.TxIn[0].Witness = [][]byte{{0x01, 0x02}}
	hw := tx.WTxHash()
	if h1 == hw {
		t.Fatalf("expected txid != wtxid when witness present")
	}
}

func TestPSBTSerializeMagic(t *testing.T) {
	tx := NewMsgTx(2)
	ps := NewPSBTFromUnsignedTx(tx)
	b := ps.Serialize()
	if !bytes.HasPrefix(b, []byte("psbt\xff")) {
		t.Fatalf("psbt missing magic prefix")
	}
}

func TestCoinSelectionAndFees(t *testing.T) {
	s := NewSweeper([]byte("test_pubkey__________33bytes________")[:33], BitcoinTestnet)
	s.SetTestMode(true)
	// Index three UTXOs
	_ = s.Index(UTXO{TxID: stringsRepeat("a", 64), Vout: 0, ValueSats: 80_000, Address: "tb1in1", Confirmed: true})
	_ = s.Index(UTXO{TxID: stringsRepeat("b", 64), Vout: 0, ValueSats: 90_000, Address: "tb1in2", Confirmed: true})
	_ = s.Index(UTXO{TxID: stringsRepeat("c", 64), Vout: 0, ValueSats: 120_000, Address: "tb1in3", Confirmed: true})

	outs := []TxOutput{{Address: "tb1dest", ValueSats: 150_000}}
	plan, err := s.Spend(outs)
	if err != nil {
		t.Fatalf("Spend failed: %v", err)
	}
	if plan.FeeSats <= 0 {
		t.Fatalf("expected positive fee")
	}
	var inSum, outSum int64
	for _, u := range plan.Inputs {
		inSum += u.ValueSats
	}
	for _, o := range plan.Outputs {
		outSum += o.ValueSats
	}
	if inSum < outSum+plan.FeeSats {
		t.Fatalf("inputs do not cover outputs+fee")
	}
}

func TestDustFiltering(t *testing.T) {
	s := NewSweeper([]byte("test_pubkey__________33bytes________")[:33], BitcoinTestnet)
	s.SetTestMode(true)
	s.SetDustRate(600, 0.50, 55_000)
	if err := s.Index(UTXO{TxID: stringsRepeat("d", 64), Vout: 0, ValueSats: 100, Address: "tb1in", Confirmed: true}); err == nil {
		t.Fatalf("expected dust rejection")
	}
}

func TestWeightedAllocationSplit(t *testing.T) {
	outs := buildWeightedOutputs(100_000, []WeightedAddr{{Address: "tb1A", WeightBP: 7000}, {Address: "tb1B", WeightBP: 3000}}, 10)
	var sum int64
	for _, o := range outs {
		sum += o.ValueSats
	}
	if sum != 100_000 {
		t.Fatalf("weighted sum mismatch: %d", sum)
	}
}

func TestFeeEstimatorTypes(t *testing.T) {
	// Construct valid addresses for estimator
	pk := make([]byte, 33)
	for i := range pk {
		pk[i] = byte(i)
	}
	p2w, err := CreateP2WPKH(Hash160(pk), BitcoinTestnet)
	if err != nil {
		t.Fatalf("p2w: %v", err)
	}
	xonly := make([]byte, 32)
	for i := range xonly {
		xonly[i] = byte(i)
	}
	p2tr, err := CreateP2TR(xonly, BitcoinTestnet)
	if err != nil {
		t.Fatalf("p2tr: %v", err)
	}

	s := NewSweeper(pk, BitcoinTestnet)
	s.SetTestMode(false)
	// Use two inputs to amplify per-input differences
	v1 := estimateTxVBytesDetailed(s, []UTXO{{Address: p2w, ValueSats: 10_000}, {Address: p2w, ValueSats: 10_000}}, []TxOutput{{Address: p2w, ValueSats: 1000}})
	v2 := estimateTxVBytesDetailed(s, []UTXO{{Address: p2tr, ValueSats: 10_000}, {Address: p2tr, ValueSats: 10_000}}, []TxOutput{{Address: p2tr, ValueSats: 1000}})
	if v2 >= v1 {
		t.Fatalf("expected P2TR vbytes < P2WPKH (got %d vs %d)", v2, v1)
	}
}

// helper: build a dummy 64-char hex string
func stringsRepeat(c string, n int) string {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		b.WriteString(c)
	}
	return b.String()
}
