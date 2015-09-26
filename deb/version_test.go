package deb

import "testing"

var testVersionComparison = []struct {
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

	{"1.0-7+bbm+b9.g24102c6.wheezy1", "1.0-6+bbm-b6.g442cbf8.wheezy1", -1},

	{"2.3~pre1003.wheezy1", "2.3~pre1002.wheezy1", 1},
	{"2.3~pre1003.wheezy1", "2.30~pre1002.wheezy1", -1},
}

func TestVersionComparison(t *testing.T) {
	for i, tt := range testVersionComparison {
		var err error
		aVer, err := ParseVersion(tt.a)
		if err != nil {
			t.Errorf("%d. failed: %q\n", i, err.Error())
		}
		bVer, err := ParseVersion(tt.b)
		if err != nil {
			t.Errorf("%d. failed: %q\n", i, err.Error())
		}

		//log.Println("a: ", aVer)
		//log.Println("b: ", bVer)
		comparison := VersionCompare(aVer, bVer)

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

var testParseVersion = []struct {
	in  string
	out *Version
	err error
}{
	{"20a", &Version{0, "20a", ""}, nil},
	{"20a-1", &Version{0, "20a", "1"}, nil},
	{"2.01-1-1", &Version{0, "2.01-1", "1"}, nil},
	{"1:2.3-1", &Version{1, "2.3", "1"}, nil},
	{"1:2.3-1-1", &Version{1, "2.3-1", "1"}, nil},
	{"1:1:1.0-1-1~1", &Version{1, "1:1.0-1", "1~1"}, nil},
	{"2.3~pre13.wheezy1", &Version{0, "2.3~pre13.wheezy1", ""}, nil},
}

func TestParseVersion(t *testing.T) {
	for i, tt := range testParseVersion {
		var err error
		inVer, err := ParseVersion(tt.in)

		if inVer != *tt.out || err != tt.err {
			t.Errorf("%d. failed: expected %q got %q\n", i, tt.out, inVer)
		}
	}
}

var testgsionString = []struct {
	in  *Version
	out string
}{
	{&Version{0, "20a", ""}, "20a"},
	{&Version{0, "20a", "1"}, "20a-1"},
	{&Version{0, "2.01-1", "1"}, "2.01-1-1"},
	{&Version{1, "2.3", "1"}, "1:2.3-1"},
	{&Version{1, "2.3-1", "1"}, "1:2.3-1-1"},
	{&Version{1, "1:1.0-1", "1~1"}, "1:1:1.0-1-1~1"},
}

func TestgsionString(t *testing.T) {
	for i, tt := range testgsionString {
		outStr := tt.in.String()

		if outStr != tt.out {
			t.Errorf("%d. failed: expected %s got %s\n", i, tt.out, outStr)
		}
	}
}
