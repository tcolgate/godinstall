package main

import (
	"errors"
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

func ParsePurgeRules(rulesStr string) ([]*PurgeRule, error) {
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

	if len(matches) < 3 {
		return &rule, errors.New("invalid purge rule \"" + ruleStr + "\"")
	}

	rule.pkgPattern, err = regexp.Compile(matches[0])
	if err != nil {
		return nil, err
	}

	if matches[1] == "*" {
		rule.limitVersions = false
	} else {
		rule.limitVersions = true
		rule.retainVersions, _ = strconv.ParseInt(matches[1], 10, 16)
	}

	if matches[2] == "*" {
		rule.limitRevisions = false
	} else {
		rule.limitRevisions = true
		rule.retainRevisions, _ = strconv.ParseInt(matches[2], 10, 16)
	}
	return &rule, nil
}
