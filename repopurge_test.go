package main

import "testing"

var testRepoPurgeInput = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},
	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "2", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", ""}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

// .*_*-*
var testPurgeOutput1 = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},
	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "2", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", ""}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

// .*_*-0
var testPurgeOutput2 = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},

	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},

	&RepoItem{Name: "pkge", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "2", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

// .*_*-2
var testPurgeOutput3 = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},

	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},

	&RepoItem{Name: "pkge", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "2", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", ""}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

// .*_0-*
var testPurgeOutput4 = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},

	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},

	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

// .*_2-*
var testPurgeOutput5 = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},

	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},

	&RepoItem{Name: "pkge", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", ""}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

// .*_0-0
var testPurgeOutput6 = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},

	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},

	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

// .*_2-2
var testPurgeOutput7 = []*RepoItem{
	&RepoItem{Name: "pkga", Version: DebVersion{0, "1", ""}, Architecture: "amd64"},

	&RepoItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}, Architecture: "source"},

	&RepoItem{Name: "pkge", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkge", Version: DebVersion{0, "4", "1"}, Architecture: "amd64"},

	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", ""}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}, Architecture: "amd64"},
	&RepoItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}, Architecture: "amd64"},
}

var testRepoPurge = []struct {
	rules  string
	output []*RepoItem
}{
	{".*_*-*", testPurgeOutput1},
	{".*_*-0", testPurgeOutput2},
	{".*_*-2", testPurgeOutput3},
	{".*_0-*", testPurgeOutput4},
	{".*_2-*", testPurgeOutput5},
	{".*_0-0", testPurgeOutput6},
	{".*_2-2", testPurgeOutput7},
}

func TestPurgeRules(t *testing.T) {
	for i, tt := range testRepoPurge {
		r, err := ParsePurgeRules(tt.rules)
		if err != nil {
			t.Errorf("TestPurgeRules[%d]: ParPurgeRules failed: ", i, err.Error())
		}
		p := r.MakePurger()
		res := make([]*RepoItem, 0)
		for _, j := range testRepoPurgeInput {
			if !p(j) {
				res = append(res, j)
			}
		}
	}
}
