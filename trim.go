package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tcolgate/godinstall/store"
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
	log.Println("Trim requested")
	count := commitcount
	return func(commit *Release) (trim bool) {
		if count >= 0 {
			count--
			return false
		}
		return true
	}
}

// TrimHistory works through the release history and attempts to apply
// the promvided Trimmer. If the current active release history is longer than
// the trimmer allows, a new release will be created with the number of commits
// after the parent release to trim beyond in the TrimAfter field.
//   If the release history is already shorter than the Trimmer allows, or
// the release history has previous been trimmed to a shorter length than would
// be allowed, the original parentid is returned.
func (release *Release) TrimHistory(s Archiver, trimmer Trimmer) error {
	curr := release.ParentID
	trimCount := int32(0)
	activeTrim := int32(0)

	for {
		if s.IsEmptyFileID(store.ID(curr)) {
			// We reached an empty commit before we decided to trim
			return nil
		}

		if activeTrim == 1 {
			// The current commit is the last commit from previously
			// active trimming, the release history is short enough
			// already
			return nil
		}

		c, err := s.GetRelease(curr)
		if err != nil {
			return errors.New("error while trimming, " + err.Error())
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

		curr = c.ParentID
	}

	if trimCount > 0 {
		log.Println("Trim did something")
		release.Actions = append(release.Actions,
			ReleaseLogAction{
				Type:        ActionTRIM,
				Description: fmt.Sprintf("Release history trimmed to %d revisions", trimCount),
			})
		release.TrimAfter = trimCount
	}
	return nil
}
