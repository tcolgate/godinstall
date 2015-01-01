package main

import (
	"log"
	"regexp"

	"code.google.com/p/go.crypto/openpgp"
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

	PublicKeyIDs []StoreID
	SigningKeyID StoreID

	poolPattern *regexp.Regexp
	pruneRules  *PruneRuleSet
}

type httpReleaseConfig struct {
	VerifyChanges           *bool
	VerifyChangesSufficient *bool
	VerifyDebs              *bool
	AcceptLoneDebs          *bool

	PoolPattern string

	AutoTrim       *bool
	AutoTrimLength int

	PruneRules string

	PublicKeyIDs []string
	SigningKeyID string
}

func (r *ReleaseConfig) PoolPatternRegexp() *regexp.Regexp {
	if r.poolPattern == nil {
		var err error
		r.poolPattern, err = regexp.CompilePOSIX("^(" + r.PoolPattern + ")")
		if err != nil {
			log.Println("Failed to compile regexp for stored release config pool pattern, " + err.Error())
		}
	}

	return r.poolPattern
}

func (r *ReleaseConfig) SignerKey() *openpgp.Entity {
	return nil
}

func (r *ReleaseConfig) MakeTrimmer() Trimmer {
	return nil
}

func (r *ReleaseConfig) MakePruner() Pruner {
	return r.pruneRules.MakePruner()
}

func (r *ReleaseConfig) PubRing() openpgp.EntityList {
	return nil
}
