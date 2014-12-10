package main

import (
	"fmt"
	"reflect"
	"testing"
)

var testRepoPruneInput = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "3"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", ""}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}}},
}

// .*_*-*
var testPruneOutput1 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "3"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", ""}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}}},
}

// .*_*-0
var testPruneOutput2 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "3"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}}},
}

// .*_*-2
var testPruneOutput3 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "3"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "1", "1"}}},
}

// .*_0-*
var testPruneOutput4 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
}

// .*_2-*
var testPruneOutput5 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "2", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "3"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", ""}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}}},
}

// .*_0-0
var testPruneOutput6 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
}

// .*_2-2
var testPruneOutput7 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "2", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "3"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "1"}}},
}

// pkgf_2-0,.*_0-0
var testPruneOutput8 = []*ReleaseIndexEntry{
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkga", Version: DebVersion{0, "1", ""}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgb", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgc", Version: DebVersion{0, "1", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgd", Version: DebVersion{0, "2", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkge", Version: DebVersion{0, "4", "1"}}},

	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{1, "1", "1"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "3", "3"}}},
	&ReleaseIndexEntry{SourceItem: ReleaseIndexEntryItem{Name: "pkgf", Version: DebVersion{0, "2", "2"}}},
}

var testRepoPrune = []struct {
	rules  string
	output []*ReleaseIndexEntry
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

func formatTestItemList(items []*ReleaseIndexEntry) string {
	output := ""

	for _, item := range items {
		output += fmt.Sprintf("%v_%v:%v-%v.%v\n",
			item.SourceItem.Name,
			item.SourceItem.Version.Epoch,
			item.SourceItem.Version.Version,
			item.SourceItem.Version.Revision,
			item.SourceItem.Architecture,
		)
	}

	return output
}

func TestPruneRules(t *testing.T) {
	for i, tt := range testRepoPrune {
		r, err := ParsePruneRules(tt.rules)
		if err != nil {
			t.Errorf("TestPruneRules[%d]: ParPruneRules failed: %s", i, err.Error())
		}
		p := r.MakePruner()
		var res []*ReleaseIndexEntry
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
