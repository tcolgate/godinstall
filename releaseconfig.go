package main

import "regexp"

// ReleaseConfig is used for configuration options which can be set and managed
// on a per-release basis. For instance, we may wish to vary the list of pgp
// keys used to verify uploads to a release over time.
type ReleaseConfig struct {
	VerifyChanges           bool
	VerifyChangesSufficient bool
	VerifyDebs              bool
	AcceptLoneDebs          bool

	PoolPattern poolRegexp

	AutoTrim       bool
	AutoTrimLength int

	PruneRules string

	PublicKeyIDs []StoreID
	SigningKeyID StoreID

	pruneRules *PruneRuleSet
}

type poolRegexp struct {
	*regexp.Regexp
}

func (r poolRegexp) BinaryMarshal() ([]byte, error) {
	return []byte(r.String()), nil
}

func (r *poolRegexp) BinaryUnMarshal(bs []byte) error {
	str := string(bs)
	re, err := regexp.Compile(str)
	if err != nil {
		return err
	}
	r = &poolRegexp{re}
	return nil
}

func (r *ReleaseConfig) MakeTrimmer() Trimmer {
	return nil
}

func (r *ReleaseConfig) MakePruner() Pruner {
	return r.pruneRules.MakePruner()
}
