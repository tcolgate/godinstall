package main

import (
	"fmt"
	"reflect"
	"testing"
)

var testRepoPruneInput = []*RepoItem{
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
var testPruneOutput1 = []*RepoItem{
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
var testPruneOutput2 = []*RepoItem{
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
var testPruneOutput3 = []*RepoItem{
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
var testPruneOutput4 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
}

// .*_2-*
var testPruneOutput5 = []*RepoItem{
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
var testPruneOutput6 = []*RepoItem{
	&RepoItem{Name: "pkga", Architecture: "amd64", Version: DebVersion{0, "1", ""}},

	&RepoItem{Name: "pkgb", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgc", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "amd64", Version: DebVersion{0, "1", "1"}},

	&RepoItem{Name: "pkgd", Architecture: "source", Version: DebVersion{0, "2", "1"}},

	&RepoItem{Name: "pkge", Architecture: "amd64", Version: DebVersion{0, "4", "1"}},

	&RepoItem{Name: "pkgf", Architecture: "amd64", Version: DebVersion{1, "1", "1"}},
}

// .*_2-2
var testPruneOutput7 = []*RepoItem{
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
var testPruneOutput8 = []*RepoItem{
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

var testRepoPrune = []struct {
	rules  string
	output []*RepoItem
}{
	{".*_*-*", testPruneOutput1},
	{".*_*-0", testPruneOutput2},
	{".*_*-2", testPruneOutput3},
	{".*_0-*", testPruneOutput4},
	{".*_2-*", testPruneOutput5},
	{".*_0-0", testPruneOutput6},
	{".*_2-2", testPruneOutput7},
	{"pkgf_2-0,.*_0-0", testPruneOutput8},
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

func TestPruneRules(t *testing.T) {
	for i, tt := range testRepoPrune {
		r, err := ParsePruneRules(tt.rules)
		if err != nil {
			t.Errorf("TestPruneRules[%d]: ParPruneRules failed: ", i, err.Error())
		}
		p := r.MakePruner()
		res := make([]*RepoItem, 0)
		for _, j := range testRepoPruneInput {
			if !p(j) {
				res = append(res, j)
			}
		}
		if !reflect.DeepEqual(res, tt.output) {
			t.Errorf("TestPruneRules[%d]: %v, failed:\nExpected:\n%v\nGot:\n%v\n",
				i+1,
				tt.rules,
				formatTestItemList(tt.output),
				formatTestItemList(res))
		}
	}
}
