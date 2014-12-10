package main

import (
	"strconv"
	"strings"
	"unicode"
)

// DebVersion contains the componenet of a debian version
type DebVersion struct {
	Epoch    int    // Primarily used for fixing previous errors in versiongin
	Version  string // The "upstream" version, of the actual packaged applications
	Revision string // This is the revision for the packaging of the main upstream version
}

func (d *DebVersion) String() string {
	output := ""

	if d.Epoch != 0 {
		output = strconv.FormatInt(int64(d.Epoch), 10) + ":"
	}

	output = output + d.Version

	if d.Revision != "" {
		output = output + "-" + d.Revision
	}

	return output
}

// DebVersionFromString converts a string to the debian version components
func DebVersionFromString(str string) (version DebVersion, err error) {
	epochSplit := strings.SplitN(str, ":", 2)
	if len(epochSplit) > 1 {
		var epoch int64
		epochStr := epochSplit[0]
		epoch, err = strconv.ParseInt(epochStr, 10, 64)
		if err != nil {
			return
		}
		version.Epoch = int(epoch)
	}

	verRevStr := epochSplit[len(epochSplit)-1]

	verRevSplit := strings.Split(verRevStr, "-")

	if len(verRevSplit) > 1 {
		version.Version = strings.Join(verRevSplit[:len(verRevSplit)-1], "-")
		version.Revision = verRevSplit[len(verRevSplit)-1]
	} else {
		version.Version = verRevSplit[0]
	}

	return
}

// DebVersionCompare compares two version, returning 0 if equal, < 0 if a < b or
// > 0 if a > b. Annoyingly 'subtle' code, poached from
// http://anonscm.debian.org/cgit/dpkg/dpkg.git/tree/lib/dpkg/version.c?h=wheezy
func DebVersionCompare(a DebVersion, b DebVersion) int {
	if a.Epoch > b.Epoch {
		return 1
	}
	if a.Epoch < b.Epoch {
		return -1
	}

	res := compareComponent(a.Version, b.Version)

	if res == 0 {
		res = compareComponent(a.Revision, b.Revision)
	}

	return res
}
func charOrder(c int) int {
	/**
	 * @param c An ASCII character.
	 */
	switch {
	case unicode.IsDigit(rune(c)):
		return 0
	case unicode.IsLetter(rune(c)):
		return c
	case c == '~':
		return -1
	case c != 0:
		return c + 256
	default:
		return 0
	}
}

func compareComponent(a string, b string) int {
	var i, j int
	for {
		if i < len(a) || j < len(b) {
			firstdiff := 0
			for {
				if (i < len(a) && !unicode.IsDigit(rune(a[i]))) ||
					(j < len(b) && !unicode.IsDigit(rune(b[j]))) {
					var ac, bc int
					if i < len(a) {
						ac = charOrder(int(a[i]))
					}
					if j < len(b) {
						bc = charOrder(int(b[j]))
					}
					if ac != bc {
						return ac - bc
					}
					i++
					j++
				} else {
					break
				}
			}
			for {
				if i < len(a) && rune(a[i]) == '0' {
					i++
				} else {
					break
				}
			}
			for {
				if j < len(b) && rune(b[j]) == '0' {
					j++
				} else {
					break
				}
			}
			for {
				if (i < len(a) && unicode.IsDigit(rune(a[i]))) &&
					(j < len(b) && unicode.IsDigit(rune(b[j]))) {
					if firstdiff == 0 {
						firstdiff = int(a[i]) - int(b[j])
					}
					i++
					j++
				} else {
					break
				}
			}
			if i < len(a) && unicode.IsDigit(rune(a[i])) {
				return 1
			}
			if j < len(b) && unicode.IsDigit(rune(b[j])) {
				return -1
			}
			if firstdiff != 0 {
				return firstdiff
			}
		} else {
			break
		}
	}

	return 0
}
