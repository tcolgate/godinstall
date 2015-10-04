// Package store is a content addressable file store
// TODO(tcm): Abstract out file operations to allow alternate backing stores
package store

import "hash"

type Hasher interface {
	NewHash() hash.Hash
	EmptyFileID() ID       // Return the StoreID for an 0 byte object
	IsEmptyFileID(ID) bool // Compares the ID to the stores empty ID
}

type hasher struct {
	newHash func() hash.Hash
	emptyID ID
}

func emptyID(hf func() hash.Hash) ID {
	hasher := hf()
	id := hasher.Sum(nil)
	return id
}

func (h *hasher) NewHash() hash.Hash {
	return h.newHash()
}

func (h *hasher) EmptyFileID() ID {
	if h.emptyID == nil {
		h.emptyID = emptyID(h.newHash)
	}
	return h.emptyID
}

func (h *hasher) IsEmptyFileID(id ID) bool {
	return CompareID(id, h.EmptyFileID())
}

// New creates a blob store that uses  hex encoded hash strings of
// ingested blobs for IDs, using the provided hash function
func NewHasher(
	hf func() hash.Hash,
) Hasher {
	return &hasher{
		newHash: hf,
	}
}
