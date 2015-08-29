package store

import (
	"bytes"
	"encoding/hex"
)

// An interface to a content addressable file store

// StoreID is a handle for an object within the store
type ID []byte

func (s ID) String() string {
	return hex.EncodeToString(s)
}

// MarshalJSON marshals a StoreID to a json string
func (s ID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + s.String() + "\""), nil
}

// UnMarshalJSON attempts to unmarshal a json string as a storeid
func (s *ID) UnMarshalJSON(j []byte) error {
	b, err := hex.DecodeString(string(j))
	sid := ID(b)
	s = &sid
	return err
}

// ByID implements sorting for arrays of StoreID
type ByID []ID

func (a ByID) Len() int      { return len(a) }
func (a ByID) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByID) Less(i, j int) bool {
	c := bytes.Compare(a[i], a[j])
	return c < 0
}

// StoreIDFromString parses a string and returns the StoreID
// that it would represent
func IDFromString(str string) (ID, error) {
	b, err := hex.DecodeString(str)
	return ID(b), err
}
