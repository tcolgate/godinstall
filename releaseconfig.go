package main

import (
	"log"
	"regexp"
)

// ReleaseConfig is used for configuration options which can be set and managed
// on a per-release basis. For instance, we may wish to vary the list of pgp
// keys used to verify uploads to a release over time.
type ReleaseConfig struct {
	VerifyChanges           bool
	VerifyChangesSufficient bool
	VerifyDebs              bool
	AcceptLoneDebs          bool

	PoolPattern string

	AutoTrim       bool
	AutoTrimLength int

	PruneRules string

	PublicKeyIDs []StoreID `json:",omitempty"`
	SigningKeyID StoreID   `json:",omitempty"`

	pruneRules *PruneRuleSet
	poolRegex  *regexp.Regexp
}

func (r *ReleaseConfig) MakeTrimmer() Trimmer {
	return MakeLengthTrimmer(r.AutoTrimLength)
}

func (r *ReleaseConfig) MakePruner() Pruner {
	if r.pruneRules == nil {
		rules, err := ParsePruneRules(r.PruneRules)
		if err != nil {
			log.Println("Error parsing stored prune rules", err)
		}
		r.pruneRules = &rules
	}
	return r.pruneRules.MakePruner()
}

func (r *ReleaseConfig) PoolRegexp() *regexp.Regexp {
	if r.poolRegex == nil {
		var err error
		r.poolRegex, err = regexp.Compile(r.PoolPattern)
		if err != nil {
			log.Println("Error parsing stored pool pattern", err)
		}
	}

	return r.poolRegex
}
