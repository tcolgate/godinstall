package main

import (
	"fmt"
	"reflect"
	"testing"
)

var testRepoPurgeInput = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "3"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", ""}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},
}

// .*_*-*
var testPurgeOutput1 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "3"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", ""}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},
}

// .*_*-0
var testPurgeOutput2 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "3"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},
}

// .*_*-2
var testPurgeOutput3 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "3"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},
}

// .*_0-*
var testPurgeOutput4 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
}

// .*_2-*
var testPurgeOutput5 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "3"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", ""}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
}

// .*_0-0
var testPurgeOutput6 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
}

// .*_2-2
var testPurgeOutput7 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "3"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "2"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "1"}},
}

// pkgf_2-0,.*_0-0
var testPurgeOutput8 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "3", "3"}},
	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{0, "2", "2"}},
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
	{"pkgf_2-0,.*_0-0", testPurgeOutput8},
}

func formatTestItemList(items []*RepoItem) string {
	output := ""

	for _, item := range items {
		output += fmt.Sprintf("%v_%v:%v-%v.%v\n",
			item.Name,
			item.Version.Epoch,
			item.Version.Version,
			item.Version.Revision,
			item.Architecture,
		)
	}

	return output
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
		if !reflect.DeepEqual(res, tt.output) {
			t.Errorf("TestPurgeRules[%d]: %v, failed:\nExpected:\n%v\nGot:\n%v\n",
				i+1,
				tt.rules,
				formatTestItemList(tt.output),
				formatTestItemList(res))
		}
	}
}
