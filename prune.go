package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// PruneRule describes the details of a rule for removing old
// items from the repository
type PruneRule struct {
	pkgPattern      *regexp.Regexp // A pattern to atch against package names
	limitVersions   bool           // should we limit the number of old version
	retainVersions  int64          // how many additional historical versions should we keep
	limitRevisions  bool           // should we limit the number of old revisions
	retainRevisions int64          // how many additional historical revisions should we keep
}

// PruneRuleSet is a group of pruning rules provided by the user
type PruneRuleSet []*PruneRule

// Pruner should return true if the given Index entry sould be removed
type Pruner func(*ReleaseIndexEntry) bool

// MakePruner creates a new pruner. The pruner is a function that takes
// a repository item, and decides if it will be included or not (true
// implies the item should be removed, false means it should be kept)
func (rules PruneRuleSet) MakePruner() Pruner {
	currPkg := ""
	currEpoch := 0
	currVersion := ""
	currVersionCnt := 0
	currRevision := ""
	currRevisionCnt := 0
	var currRule *PruneRule

	return func(entry *ReleaseIndexEntry) (prune bool) {
		item := entry.SourceItem
		if item.Name != currPkg {
			currPkg = item.Name
			currEpoch = item.Version.Epoch
			currVersion = item.Version.Version
			currVersionCnt = 1
			currRevision = item.Version.Revision
			currRevisionCnt = 1

			// Try and find a prune rule to use
			for _, r := range rules {
				if r.pkgPattern.MatchString(currPkg) {
					currRule = r
					break
				} else {
					currRule = nil
				}
			}

			return false
		}

		if item.Version.Version != currVersion || item.Version.Epoch != currEpoch {
			currVersionCnt++
			currVersion = item.Version.Version
			currEpoch = item.Version.Epoch
			currRevision = item.Version.Revision
			currRevisionCnt = 1
		} else {
			if item.Version.Revision != currRevision {
				currRevisionCnt++
				currRevision = item.Version.Revision
			} else {
				// The two versions match, shouldn't prune this
				return false
			}
		}

		if currRule.limitVersions {
			if int64(currVersionCnt) > currRule.retainVersions+1 {
				return true
			}
		}

		if currRule.limitRevisions {
			if int64(currRevisionCnt) > currRule.retainRevisions+1 {
				return true
			}
		}

		return false
	}
}

// ParsePruneRules converts a string into a set of pruning rules
func ParsePruneRules(rulesStr string) (PruneRuleSet, error) {
	ruleStrings := strings.Split(rulesStr, ",")
	var rules []*PruneRule

	for _, ruleStr := range ruleStrings {
		rule, err := ParsePruneRule(ruleStr)
		if err != nil {
			return rules, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

var ruleRegex = regexp.MustCompile(`^(.*)_(\d+|\*)-(\d+|\*)$`)

// ParsePruneRule converts a string describing a single pruning rule
// into an internal description used for pruning
func ParsePruneRule(ruleStr string) (*PruneRule, error) {
	var rule PruneRule
	var err error

	matches := ruleRegex.FindStringSubmatch(ruleStr)

	if len(matches) != 4 {
		return &rule, errors.New("invalid prune rule \"" + ruleStr + "\"")
	}

	rule.pkgPattern, err = regexp.Compile(matches[1])
	if err != nil {
		return nil, err
	}

	if matches[2] == "*" {
		rule.limitVersions = false
	} else {
		rule.limitVersions = true
		rule.retainVersions, _ = strconv.ParseInt(matches[2], 10, 16)
	}

	if matches[3] == "*" {
		rule.limitRevisions = false
	} else {
		rule.limitRevisions = true
		rule.retainRevisions, _ = strconv.ParseInt(matches[3], 10, 16)
	}
	return &rule, nil
}
