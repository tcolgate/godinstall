package main

import (
	"errors"
	"log"
	"regexp"
	"strconv"
	"strings"
)

type PurgeRule struct {
	pkgPattern      *regexp.Regexp
	limitVersions   bool
	retainVersions  int64
	limitRevisions  bool
	retainRevisions int64
}

type PurgeRuleSet []*PurgeRule

func (rules PurgeRuleSet) MakePurger() func(*RepoItem) bool {
	currPkg := ""
	currVersion := ""
	currVersionCnt := 0
	currRevision := ""
	currRevisionCnt := 0
	var currRule *PurgeRule

	return func(item *RepoItem) (purge bool) {
		if item.Name != currPkg {
			currPkg = item.Name
			currVersion = item.Version.Version
			currVersionCnt = 1
			currRevision = item.Version.Revision
			currRevisionCnt = 1

			// Try and find a purge rule to use
			for _, r := range rules {
				log.Println(r.pkgPattern)
				if r.pkgPattern.MatchString(currPkg) {
					currRule = r
					break
				} else {
					currRule = nil
				}
			}

			log.Println(currRule)
			return false
		} else {
			if item.Version.Version != currVersion {
				currVersionCnt += 1
				currVersion = item.Version.Version
				currRevision = item.Version.Revision
				currRevisionCnt = 1
			} else {
				if item.Version.Revision != currRevision {
					currRevisionCnt += 1
					currRevision = item.Version.Revision
				} else {
					// pkg, version and revision all match, we already
					// have this package!
					return true
				}
			}
		}

		if currRule.limitVersions {
			if int64(currVersionCnt) > currRule.retainVersions {
				log.Printf("Limiting %v to %v verssions", currPkg, currRule.retainVersionsa)
				return true
			}
		}

		if currRule.limitRevisions {
			if int64(currRevisionCnt) > currRule.retainRevisions {
				log.Printf("Limiting %v to %v revisions", currPkg, currRule.retainRevisions)
				return true
			}
		}

		return false
	}
}

func ParsePurgeRules(rulesStr string) (PurgeRuleSet, error) {
	ruleStrings := strings.Split(rulesStr, ",")
	rules := make([]*PurgeRule, 0)

	for _, ruleStr := range ruleStrings {
		rule, err := ParsePurgeRule(ruleStr)
		if err != nil {
			return rules, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

var ruleRegex = regexp.MustCompile(`^(.*)_(\d+|\*)-(\d+|\*)$`)

func ParsePurgeRule(ruleStr string) (*PurgeRule, error) {
	var rule PurgeRule
	var err error

	matches := ruleRegex.FindStringSubmatch(ruleStr)

	if len(matches) != 4 {
		return &rule, errors.New("invalid purge rule \"" + ruleStr + "\"")
	}

	log.Println(matches)
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
