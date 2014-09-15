package main

import "testing"

var testDebVersionComparison = []struct {
	a      string
	b      string
	result int
}{
	{"1.0", "1.0", 0},
	{"1.0", "0.9", 1},
	{"1.0", "1.1", -1},
	{"1.0", "1.1", -1},
	{"10.0", "1.0", 1},
	{"1.0", "10.1", -1},
	{"1a", "1", 1},
	{"1a", "1b", -1},
	{"1b.0", "1a.1", 1},
	{"1a.0", "1b.1", -1},
	{"1.0~1", "1.0", -1},

	{"1:1.0~1", "1.0", 1},
	{"1.0~1", "2:1.0", -1},

	{"1.0-1~1", "1.0", 1},
	{"1.0-1~1", "1.0-1", -1},
	{"1.0-1", "1.0~1", 1},

	{"1:1:1.0-1-1~1", "1.0-1", 1},
}

func TestDebVersionComparison(t *testing.T) {
	for i, tt := range testDebVersionComparison {
		var err error
		aVer, err := DebVersionFromString(tt.a)
		if err != nil {
			t.Errorf("%d. failed: %q\n", i, err.Error())
		}
		bVer, err := DebVersionFromString(tt.b)
		if err != nil {
			t.Errorf("%d. failed: %q\n", i, err.Error())
		}

		//log.Println("a: ", aVer)
		//log.Println("b: ", bVer)
		comparison := DebVersionCompare(aVer, bVer)

		switch {
		case tt.result == 0 && comparison != 0:
			t.Errorf("%d. failed: expected 0 returned %d\n", i, comparison)
		case tt.result == -1 && comparison >= 0:
			t.Errorf("%d. failed: expected less than 0 , returned %d\n", i, comparison)
		case tt.result == 1 && comparison <= 0:
			t.Errorf("%d. failed: expected greater than 0, returned %d\n", i, comparison)
		}
	}
}

var testDebVersionFromString = []struct {
	in  string
	out *DebVersion
	err error
}{
	{"1:1:1.0-1-1~1", &DebVersion{1, "1:1.0-1", "1~1"}, nil},
}

func TestDebVersionFromString(t *testing.T) {
	for i, tt := range testDebVersionFromString {
		var err error
		inVer, err := DebVersionFromString(tt.in)

		if inVer != *tt.out || err != tt.err {
			t.Errorf("%d. failed: expected %q got %q\n", i, tt.out, inVer)
		}
	}
}
