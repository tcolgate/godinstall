package main

import (
	"fmt"
	"time"
)

// Trimmer is an type for describing functions that can be used to
// trim the repository history
type Trimmer func(*Release) bool

// MakeTimeTrimmer creates a trimmer function that reduces the repository
// history to a given window of time
func MakeTimeTrimmer(time.Duration) Trimmer {
	return func(commit *Release) (trim bool) {
		return false
	}
}

// MakeLengthTrimmer creates a trimmer function that reduces the repository
// history to a given number of commits
func MakeLengthTrimmer(commitcount int) Trimmer {
	count := commitcount
	return func(commit *Release) (trim bool) {
		if count >= 0 {
			count--
			return false
		}
		return true
	}
}

// TrimReleaseHistory works through the release history and attempts to apply
// the promvided Trimmer. If the current active release history is longer than
// the trimmer allows, a new release will be created with the number of commits
// after the parent release to trim beyond in the TrimAfter field.
//   If the release history is already shorter than the Trimmer allows, or
// the release history has previous been trimmed to a shorter length than would
// be allowed, the original parentid is returned.
func TrimReleaseHistory(store Archiver, parentid StoreID, trimmer Trimmer) (id StoreID, err error) {
	parent, err := store.GetRelease(parentid)
	if err != nil {
		return nil, err
	}

	curr := parentid
	trimCount := 0
	activeTrim := int32(0)

	for {
		if StoreID(curr).String() == store.EmptyFileID().String() {
			// We reached an empty commit before we decided to trim
			// so just return the untrimmed origin StoreID
			return parentid, nil
		}

		if activeTrim == 1 {
			// The current commit is the last commit from previously
			// active trimming, the release history is short enough
			// already
			return parentid, nil
		}

		c, err := store.GetRelease(curr)
		if err != nil {
			return parentid, err
		}

		if c.TrimAfter != 0 {
			if c.TrimAfter < activeTrim {
				activeTrim = c.TrimAfter
			}
		}

		// Check to see if the trimmer would dispose of this
		// commit
		if trimmer(c) {
			break
		} else {
			trimCount++
		}

		if activeTrim > 1 {
			activeTrim--
		}
	}

	if trimCount > 0 {
		release := *parent
		release.ParentID = parentid
		release.Actions = []ReleaseLogAction{
			ReleaseLogAction{
				Type:        ActionTRIM,
				Description: fmt.Sprintf("Release history trimmed to %d revisions", trimCount),
			},
		}
		release.Date = time.Now()
		return store.AddRelease(&release)
	} else {
		return parentid, nil
	}
}
