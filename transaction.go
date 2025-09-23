// Package main provides a dependency-free Bitcoin UTXO sweeper library.
// This file contains Bitcoin transaction structures, serialization, and PSBT handling.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
)

// OutPoint represents a reference to a previous transaction output.
// It consists of the transaction hash and output index.
type OutPoint struct {
	Hash  [32]byte // SHA256 hash of the previous transaction
	Index uint32   // Output index in the previous transaction
}

// TxIn represents a transaction input that spends a previous output.
// It includes the previous output reference, signature script, witness data, and sequence number.
type TxIn struct {
	PreviousOutPoint OutPoint // Reference to the previous output being spent
	SignatureScript  []byte   // Legacy signature script (empty for SegWit)
	Witness          [][]byte // Witness data for SegWit transactions
	Sequence         uint32   // Sequence number for RBF and time locks
}

// TxOut represents a transaction output that creates new UTXOs.
// It specifies the value in satoshis and the output script.
type TxOut struct {
	Value    int64  // Value in satoshis
	PkScript []byte // Output script (e.g., P2WPKH, P2TR)
}

// MsgTx represents a complete Bitcoin transaction.
// It contains the version, inputs, outputs, and lock time.
type MsgTx struct {
	Version  int32   // Transaction version (typically 1 or 2)
	TxIn     []TxIn  // List of transaction inputs
	TxOut    []TxOut // List of transaction outputs
	LockTime uint32  // Block height or timestamp when transaction becomes valid
}

// NewMsgTx creates a new Bitcoin transaction with the specified version.
// The transaction is initialized with empty inputs, outputs, and zero lock time.
func NewMsgTx(version int32) *MsgTx {
	return &MsgTx{
		Version:  version,
		TxIn:     make([]TxIn, 0),
		TxOut:    make([]TxOut, 0),
		LockTime: 0,
	}
}

// AddTxIn adds a transaction input to the transaction.
// This method appends the input to the existing list of inputs.
func (tx *MsgTx) AddTxIn(txin TxIn) {
	tx.TxIn = append(tx.TxIn, txin)
}

// AddTxOut adds a transaction output to the transaction.
// This method appends the output to the existing list of outputs.
func (tx *MsgTx) AddTxOut(txout TxOut) {
	tx.TxOut = append(tx.TxOut, txout)
}

// Serialize converts the transaction to its raw byte representation.
// If includeWitness is true and any input
// has witness data, the serialization uses the SegWit marker/flag and includes
// per-input witness stacks. If includeWitness is false, the serialization is the
// legacy (non-witness) encoding regardless of witness data presence.
func (tx *MsgTx) Serialize(includeWitness bool) []byte {
	var buf bytes.Buffer

	// Version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	hasWitness := false
	if includeWitness {
		for _, in := range tx.TxIn {
			if len(in.Witness) > 0 {
				hasWitness = true
				break
			}
		}
	}

	if hasWitness {
		// SegWit marker and flag
		buf.WriteByte(0x00)
		buf.WriteByte(0x01)
	}

	// Inputs (vin)
	writeVarInt(&buf, uint64(len(tx.TxIn)))
	for _, txin := range tx.TxIn {
		// Outpoint
		buf.Write(txin.PreviousOutPoint.Hash[:])
		binary.Write(&buf, binary.LittleEndian, txin.PreviousOutPoint.Index)
		// scriptSig
		writeVarInt(&buf, uint64(len(txin.SignatureScript)))
		buf.Write(txin.SignatureScript)
		// sequence
		binary.Write(&buf, binary.LittleEndian, txin.Sequence)
	}

	// Outputs (vout)
	writeVarInt(&buf, uint64(len(tx.TxOut)))
	for _, txout := range tx.TxOut {
		binary.Write(&buf, binary.LittleEndian, txout.Value)
		writeVarInt(&buf, uint64(len(txout.PkScript)))
		buf.Write(txout.PkScript)
	}

	if hasWitness {
		// Witnesses per input
		for _, txin := range tx.TxIn {
			writeVarInt(&buf, uint64(len(txin.Witness)))
			for _, item := range txin.Witness {
				writeVarInt(&buf, uint64(len(item)))
				buf.Write(item)
			}
		}
	}

	// LockTime
	binary.Write(&buf, binary.LittleEndian, tx.LockTime)

	return buf.Bytes()
}

// TxHash returns the legacy txid (double SHA256 of non-witness serialization),
// per consensus rules (witness is excluded from txid).
func (tx *MsgTx) TxHash() [32]byte {
	serialized := tx.Serialize(false)
	return sha256Double(serialized)
}

// WTxHash returns the wtxid (double SHA256 of witness-inclusive serialization).
// For transactions without witness data, wtxid equals txid.
func (tx *MsgTx) WTxHash() [32]byte {
	serialized := tx.Serialize(true)
	return sha256Double(serialized)
}

// Double SHA256
func sha256Double(data []byte) [32]byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second
}

// removed unused helper

// Write variable length integer
func writeVarInt(w *bytes.Buffer, val uint64) {
	if val < 0xfd {
		w.WriteByte(byte(val))
	} else if val <= 0xffff {
		w.WriteByte(0xfd)
		binary.Write(w, binary.LittleEndian, uint16(val))
	} else if val <= 0xffffffff {
		w.WriteByte(0xfe)
		binary.Write(w, binary.LittleEndian, uint32(val))
	} else {
		w.WriteByte(0xff)
		binary.Write(w, binary.LittleEndian, val)
	}
}

// readVarInt function removed - was unused

// PSBTInput represents a Partially Signed Bitcoin Transaction input.
// It contains all the data needed to sign a specific input.
type PSBTInput struct {
	NonWitnessUtxo     *MsgTx                      // Full previous transaction (for legacy inputs)
	WitnessUtxo        *TxOut                      // Previous output (for SegWit inputs)
	PartialSigs        map[string][]byte           // Partial signatures by public key
	SighashType        uint32                      // Signature hash type
	RedeemScript       []byte                      // P2SH redeem script
	WitnessScript      []byte                      // SegWit witness script
	Bip32Derivation    map[string]*Bip32Derivation // BIP32 derivation paths
	FinalScriptSig     []byte                      // Final signature script
	FinalScriptWitness [][]byte                    // Final witness data
}

// PSBTOutput represents a Partially Signed Bitcoin Transaction output.
// It contains metadata about how to spend the output.
type PSBTOutput struct {
	RedeemScript    []byte                      // P2SH redeem script
	WitnessScript   []byte                      // SegWit witness script
	Bip32Derivation map[string]*Bip32Derivation // BIP32 derivation paths
}

// Bip32Derivation contains BIP32 derivation path information.
// It specifies how to derive a key from a master key.
type Bip32Derivation struct {
	MasterFingerprint [4]byte  // First 4 bytes of the master key's hash160
	Path              []uint32 // Derivation path (e.g., [0, 1, 2])
}

// PSBT represents a Partially Signed Bitcoin Transaction.
// It contains an unsigned transaction and metadata for signing.
type PSBT struct {
	UnsignedTx *MsgTx       // The unsigned transaction
	Inputs     []PSBTInput  // Input metadata for signing
	Outputs    []PSBTOutput // Output metadata
}

// NewPSBTFromUnsignedTx creates a new PSBT from an unsigned transaction.
// It initializes the PSBT with empty input and output metadata.
func NewPSBTFromUnsignedTx(tx *MsgTx) *PSBT {
	psbt := &PSBT{
		UnsignedTx: tx,
		Inputs:     make([]PSBTInput, len(tx.TxIn)),
		Outputs:    make([]PSBTOutput, len(tx.TxOut)),
	}

	// Initialize inputs
	for i := range psbt.Inputs {
		psbt.Inputs[i] = PSBTInput{
			PartialSigs:     make(map[string][]byte),
			Bip32Derivation: make(map[string]*Bip32Derivation),
		}
	}

	// Initialize outputs
	for i := range psbt.Outputs {
		psbt.Outputs[i] = PSBTOutput{
			Bip32Derivation: make(map[string]*Bip32Derivation),
		}
	}

	return psbt
}

// Serialize converts the PSBT to its binary representation.
// This follows the BIP-174 PSBT serialization format.
func (psbt *PSBT) Serialize() []byte {
	var buf bytes.Buffer

	// PSBT magic: 0x70736274 0xff ("psbt\xff")
	buf.WriteString("psbt\xff")

	// ---- Global map ----
	// key: 0x00 (unsigned tx), value: non-witness serialized tx
	{
		key := []byte{0x00}
		val := psbt.UnsignedTx.Serialize(false)
		writeVarInt(&buf, uint64(len(key)))
		buf.Write(key)
		writeVarInt(&buf, uint64(len(val)))
		buf.Write(val)
		// Separator
		buf.WriteByte(0x00)
	}

	// ---- Input maps ----
	for _, input := range psbt.Inputs {
		// witness_utxo (type 0x01)
		if input.WitnessUtxo != nil {
			key := []byte{0x01}
			val := serializeTxOut(input.WitnessUtxo)
			writeVarInt(&buf, uint64(len(key)))
			buf.Write(key)
			writeVarInt(&buf, uint64(len(val)))
			buf.Write(val)
		}

		// final_script_sig (type 0x07)
		if input.FinalScriptSig != nil {
			key := []byte{0x07}
			val := input.FinalScriptSig
			writeVarInt(&buf, uint64(len(key)))
			buf.Write(key)
			writeVarInt(&buf, uint64(len(val)))
			buf.Write(val)
		}

		// final_script_witness (type 0x08), value is stack serialization
		if len(input.FinalScriptWitness) > 0 {
			key := []byte{0x08}
			val := serializeWitness(input.FinalScriptWitness)
			writeVarInt(&buf, uint64(len(key)))
			buf.Write(key)
			writeVarInt(&buf, uint64(len(val)))
			buf.Write(val)
		}

		// Separator for input map
		buf.WriteByte(0x00)
	}

	// ---- Output maps ----
	for _, output := range psbt.Outputs {
		// redeem_script (type 0x00)
		if output.RedeemScript != nil {
			key := []byte{0x00}
			val := output.RedeemScript
			writeVarInt(&buf, uint64(len(key)))
			buf.Write(key)
			writeVarInt(&buf, uint64(len(val)))
			buf.Write(val)
		}

		// witness_script (type 0x01)
		if output.WitnessScript != nil {
			key := []byte{0x01}
			val := output.WitnessScript
			writeVarInt(&buf, uint64(len(key)))
			buf.Write(key)
			writeVarInt(&buf, uint64(len(val)))
			buf.Write(val)
		}

		// Separator for output map
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

// Serialize transaction output
func serializeTxOut(txout *TxOut) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, txout.Value)
	writeVarInt(&buf, uint64(len(txout.PkScript)))
	buf.Write(txout.PkScript)
	return buf.Bytes()
}

// Serialize witness
func serializeWitness(witness [][]byte) []byte {
	var buf bytes.Buffer
	writeVarInt(&buf, uint64(len(witness)))
	for _, item := range witness {
		writeVarInt(&buf, uint64(len(item)))
		buf.Write(item)
	}
	return buf.Bytes()
}

// Serialize map
// serializeMap removed in favor of explicit BIP-174 key-value serialization

// B64Encode converts the PSBT to a base64-encoded string.
// This is the standard format for sharing PSBTs between applications.
func (psbt *PSBT) B64Encode() (string, error) {
	data := psbt.Serialize()
	return base64Encode(data), nil
}

// Simple base64 encoding
func base64Encode(data []byte) string { return base64.StdEncoding.EncodeToString(data) }

// removed unused helper

// Create outpoint from string hash and index
func NewOutPointFromStr(hashStr string, index uint32) (OutPoint, error) {
	var hash [32]byte
	if len(hashStr) != 64 {
		return OutPoint{}, errors.New("invalid hash length")
	}

	// Convert hex string to bytes
	for i := 0; i < 32; i++ {
		val, err := hexToByte(hashStr[i*2 : i*2+2])
		if err != nil {
			return OutPoint{}, err
		}
		hash[i] = val
	}

	return OutPoint{Hash: hash, Index: index}, nil
}

// Convert hex string to byte
func hexToByte(hex string) (byte, error) {
	if len(hex) != 2 {
		return 0, errors.New("invalid hex length")
	}

	var result byte
	for i, c := range hex {
		var val byte
		if c >= '0' && c <= '9' {
			val = byte(c - '0')
		} else if c >= 'a' && c <= 'f' {
			val = byte(c - 'a' + 10)
		} else if c >= 'A' && c <= 'F' {
			val = byte(c - 'A' + 10)
		} else {
			return 0, errors.New("invalid hex character")
		}
		result |= val << (4 * (1 - i))
	}

	return result, nil
}
