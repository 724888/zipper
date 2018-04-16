// Copyright (C) 2017, Zipper Team.  All rights reserved.
//
// This file is part of zipper
//
// The zipper is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The zipper is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
package utxo

import (
	"github.com/zipper-project/zipper/proto"
)

type Hash [32]byte

// utxoOutput houses details about an individual unspent transaction output such
// as whether or not it is spent, its address, and how much it pays.
type utxoOutput struct {
	asset   int32  // The asset of the output
	amount  int64  // The amount of the output.
	address []byte // The address of the output
	spent   bool   // Output is spent.
}

// UtxoEntry contains contextual information about an unspent transaction such
// as which block it was found in, and the spent status of its outputs.
type UtxoEntry struct {
	modified      bool                  // Entry changed since load.
	version       int32                 // The version of this tx.
	blockHeight   int32                 // Height of block containing tx.
	sparseOutputs map[int32]*utxoOutput // Sparse map of unspent outputs.
}

// Version returns the version of the transaction the utxo represents.
func (entry *UtxoEntry) Version() int32 {
	return entry.version
}

// BlockHeight returns the height of the block containing the transaction the
// utxo entry represents.
func (entry *UtxoEntry) BlockHeight() int32 {
	return entry.blockHeight
}

// IsOutputSpent returns whether or not the provided output index has been
// spent based upon the current state of the unspent transaction output view
// the entry was obtained from.
//
// Returns true if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) IsOutputSpent(outputIndex int32) bool {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return true
	}

	return output.spent
}

// UnspendOutput marks the output at the provided index as unspent.  Specifying an
// output index that does not exist will not have any effect.
func (entry *UtxoEntry) UnspendOutput(outputIndex int32) {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return
	}

	// Nothing to do if the output is already unspent.
	if !output.spent {
		return
	}

	entry.modified = true
	output.spent = false
}

// SpendOutput marks the output at the provided index as spent.  Specifying an
// output index that does not exist will not have any effect.
func (entry *UtxoEntry) SpendOutput(outputIndex int32) {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return
	}

	// Nothing to do if the output is already spent.
	if output.spent {
		return
	}

	entry.modified = true
	output.spent = true
}

// IsFullySpent returns whether or not the transaction the utxo entry represents
// is fully spent.
func (entry *UtxoEntry) IsFullySpent() bool {
	// The entry is not fully spent if any of the outputs are unspent.
	for _, output := range entry.sparseOutputs {
		if !output.spent {
			return false
		}
	}

	return true
}

// AssetByIndex returns the amount of the provided output index.
//
// Returns 0 if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) AssetByIndex(outputIndex int32) int32 {
	return 0
}

// AmountByIndex returns the amount of the provided output index.
//
// Returns 0 if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) AmountByIndex(outputIndex int32) int64 {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return 0
	}

	return output.amount
}

// AddressByIndex returns the public key script for the provided output index.
//
// Returns nil if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) AddressByIndex(outputIndex int32) []byte {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return nil
	}

	return output.address
}

// newUtxoEntry returns a new unspent transaction output entry with the provided
// coinbase flag and block height ready to have unspent outputs added.
func newUtxoEntry(version int32, blockHeight int32) *UtxoEntry {
	return &UtxoEntry{
		version:       version,
		blockHeight:   blockHeight,
		sparseOutputs: make(map[int32]*utxoOutput),
	}
}

// UtxoViewpoint represents a view into the set of unspent transaction outputs
// from a specific point of view in the chain.  For example, it could be for
// the end of the main chain, some point in the history of the main chain, or
// down a side chain.
//
// The unspent outputs are needed by other transactions for things such as
// script validation and double spend prevention.
type UtxoViewpoint struct {
	entries map[Hash]*UtxoEntry
}

// LookupEntry returns information about a given transaction according to the
// current state of the view.  It will return nil if the passed transaction
// hash does not exist in the view or is otherwise not available such as when
// it has been disconnected during a reorg.
func (view *UtxoViewpoint) LookupEntry(txHash *Hash) *UtxoEntry {
	entry, ok := view.entries[*txHash]
	if !ok {
		return nil
	}

	return entry
}

// Entries returns the underlying map that stores of all the utxo entries.
func (view *UtxoViewpoint) Entries() map[Hash]*UtxoEntry {
	return view.entries
}

// commit prunes all entries marked modified that are now fully spent and marks
// all entries as unmodified.
func (view *UtxoViewpoint) commit() {
	for txHash, entry := range view.entries {
		if entry == nil || (entry.modified && entry.IsFullySpent()) {
			delete(view.entries, txHash)
			continue
		}

		entry.modified = false
	}
}

// NewUtxoViewpoint returns a new empty unspent transaction output view.
func NewUtxoViewpoint() *UtxoViewpoint {
	return &UtxoViewpoint{
		entries: make(map[Hash]*UtxoEntry),
	}
}

// AddTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the view.  When the view already has entries for any of the
// outputs, they are simply marked unspent.  All fields will be updated for
// existing entries since it's possible it has changed during a reorg.
func (view *UtxoViewpoint) AddTxOuts(tx *proto.Transaction, blockHeight int32) {
	// When there are not already any utxos associated with the transaction,
	// add a new entry for it to the view.
	//entry := view.LookupEntry(tx.Hash())
	// if entry == nil {
	// 	entry = newUtxoEntry(tx.Header.Version, blockHeight)
	// 	view.entries[*tx.Hash()] = entry
	// } else {
	// 	entry.blockHeight = blockHeight
	// }
	// entry.modified = true

	// // Loop all of the transaction outputs and add those which are not
	// // provably unspendable.
	// for txOutIdx, txOut := range tx.Outputs {

	// 	// Update existing entries.  All fields are updated because it's
	// 	// possible (although extremely unlikely) that the existing
	// 	// entry is being replaced by a different transaction with the
	// 	// same hash.  This is allowed so long as the previous
	// 	// transaction is fully spent.
	// 	if output, ok := entry.sparseOutputs[int32(txOutIdx)]; ok {
	// 		output.spent = false
	// 		output.asset = txOut.Asset
	// 		output.amount = txOut.Value
	// 		output.address = txOut.Address
	// 		continue
	// 	}

	// 	// Add the unspent transaction output.
	// 	entry.sparseOutputs[int32(txOutIdx)] = &utxoOutput{
	// 		spent:   false,
	// 		asset:   txOut.Asset,
	// 		amount:  txOut.Value,
	// 		address: txOut.Address,
	// 	}
	// }
	return
}

// connectTransaction updates the view by adding all new utxos created by the
// passed transaction and marking all utxos that the transactions spend as
// spent.  In addition, when the 'stxos' argument is not nil, it will be updated
// to append an entry for each spent txout.  An error will be returned if the
// view does not contain the required utxos.
func (view *UtxoViewpoint) connectTransaction(tx *proto.Transaction, blockHeight int32) error {
	// Spend the referenced utxos by marking them spent in the view and,
	// if a slice was provided for the spent txout details, append an entry
	// to it.
	// for _, txIn := range tx.Inputs {
	// 	originIndex := txIn.PreviousOutPoint.Index
	// 	entry := view.entries[txIn.PreviousOutPoint.Hash]

	// 	// Ensure the referenced utxo exists in the view.  This should
	// 	// never happen unless there is a bug is introduced in the code.
	// 	if entry == nil {
	// 		//TODO
	// 		return fmt.Errorf("view missing input %v", txIn.PreviousOutPoint)
	// 	}
	// 	entry.SpendOutput(originIndex)
	// }

	// // Add the transaction's outputs as available utxos.
	// view.AddTxOuts(tx, blockHeight)
	return nil
}
