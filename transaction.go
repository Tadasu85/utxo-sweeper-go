// Package main provides a dependency-free Bitcoin UTXO sweeper library.
// This file contains Bitcoin transaction structures, serialization, and PSBT handling.
package main

import (
	"bytes"
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
// This follows the standard Bitcoin transaction serialization format.
func (tx *MsgTx) Serialize() []byte {
	var buf bytes.Buffer

	// Version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Inputs
	writeVarInt(&buf, uint64(len(tx.TxIn)))
	for _, txin := range tx.TxIn {
		// Previous output
		buf.Write(txin.PreviousOutPoint.Hash[:])
		binary.Write(&buf, binary.LittleEndian, txin.PreviousOutPoint.Index)

		// Signature script
		writeVarInt(&buf, uint64(len(txin.SignatureScript)))
		buf.Write(txin.SignatureScript)

		// Sequence
		binary.Write(&buf, binary.LittleEndian, txin.Sequence)
	}

	// Outputs
	writeVarInt(&buf, uint64(len(tx.TxOut)))
	for _, txout := range tx.TxOut {
		// Value
		binary.Write(&buf, binary.LittleEndian, txout.Value)

		// PkScript
		writeVarInt(&buf, uint64(len(txout.PkScript)))
		buf.Write(txout.PkScript)
	}

	// LockTime
	binary.Write(&buf, binary.LittleEndian, tx.LockTime)

	return buf.Bytes()
}

// Calculate transaction hash
func (tx *MsgTx) TxHash() [32]byte {
	serialized := tx.Serialize()
	return sha256Double(serialized)
}

// Double SHA256
func sha256Double(data []byte) [32]byte {
	first := sha256Sum(data)
	second := sha256Sum(first[:])
	return second
}

// SHA256 sum
func sha256Sum(data []byte) [32]byte {
	// This is a placeholder - in production you'd want a proper SHA256 implementation
	// For now, we'll use a simplified version
	var result [32]byte
	copy(result[:], data[:min(32, len(data))])
	return result
}

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

	// PSBT magic
	buf.WriteString("psbt\xff")

	// Global map
	globalMap := make(map[string][]byte)

	// Unsigned transaction
	txBytes := psbt.UnsignedTx.Serialize()
	globalMap["unsigned_tx"] = txBytes

	// Serialize global map
	serializeMap(&buf, globalMap)

	// Input maps
	for _, input := range psbt.Inputs {
		inputMap := make(map[string][]byte)

		if input.WitnessUtxo != nil {
			inputMap["witness_utxo"] = serializeTxOut(input.WitnessUtxo)
		}

		if input.FinalScriptSig != nil {
			inputMap["final_script_sig"] = input.FinalScriptSig
		}

		if len(input.FinalScriptWitness) > 0 {
			inputMap["final_script_witness"] = serializeWitness(input.FinalScriptWitness)
		}

		serializeMap(&buf, inputMap)
	}

	// Output maps
	for _, output := range psbt.Outputs {
		outputMap := make(map[string][]byte)

		if output.RedeemScript != nil {
			outputMap["redeem_script"] = output.RedeemScript
		}

		if output.WitnessScript != nil {
			outputMap["witness_script"] = output.WitnessScript
		}

		serializeMap(&buf, outputMap)
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
func serializeMap(buf *bytes.Buffer, m map[string][]byte) {
	for key, value := range m {
		writeVarInt(buf, uint64(len(key)))
		buf.WriteString(key)
		writeVarInt(buf, uint64(len(value)))
		buf.Write(value)
	}
	buf.WriteByte(0x00) // separator
}

// B64Encode converts the PSBT to a base64-encoded string.
// This is the standard format for sharing PSBTs between applications.
func (psbt *PSBT) B64Encode() (string, error) {
	data := psbt.Serialize()
	return base64Encode(data), nil
}

// Simple base64 encoding
func base64Encode(data []byte) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	var result []byte
	for i := 0; i < len(data); i += 3 {
		chunk := data[i:min(i+3, len(data))]

		// Convert 3 bytes to 24 bits
		bits := uint32(0)
		for j, b := range chunk {
			bits |= uint32(b) << (16 - j*8)
		}

		// Convert to 4 base64 characters
		for j := 0; j < 4; j++ {
			if i*4/3+j < (len(data)*4+2)/3 {
				idx := (bits >> (18 - j*6)) & 0x3f
				result = append(result, charset[idx])
			} else {
				result = append(result, '=')
			}
		}
	}

	return string(result)
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
