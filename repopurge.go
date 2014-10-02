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
	currArch := ""
	currVersion := ""
	currVersionCnt := 0
	currRevision := ""
	currRevisionCnt := 0
	var currRule *PurgeRule

	return func(item *RepoItem) (purge bool) {
		log.Printf("pkg: %v\nver:%v\nverc:%v\nrev:%v\nrevc:%v\n\n", currPkg, currVersion, currVersionCnt, currRevision, currRevisionCnt)
		if item.Name != currPkg || item.Architecture != currArch {
			currPkg = item.Name
			currArch = item.Architecture
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
		}

		if item.Version.Version != currVersion {
			currVersionCnt += 1
			currVersion = item.Version.Version
			currRevision = item.Version.Revision
			currRevisionCnt = 0
		} else {
			if item.Version.Revision != currRevision {
				currRevisionCnt += 1
				currRevision = item.Version.Revision
			} else {
				// The two versions match, shouldn't purge this
				return false
			}
		}

		if currRule.limitVersions {
			if int64(currVersionCnt) > currRule.retainVersions+1 {
				log.Printf("Limiting %v to %v historic versions", currPkg, currRule.retainVersions)
				return true
			}
		}

		if currRule.limitRevisions {
			if int64(currRevisionCnt) > currRule.retainRevisions+1 {
				log.Printf("Limiting %v to %v historic revisions", currPkg, currRule.retainRevisions)
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
