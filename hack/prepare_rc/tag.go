package main

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/Masterminds/semver/v3"
)

var (
	rcRe = regexp.MustCompile(`^rc.(\d+)$`)
)

// Tag is a helper for the tag operations.
type Tag struct {
	semver.Version
	rc int
}

// NewTag constructs a valid tag.
func NewTag(tag string) (Tag, error) {
	v, err := semver.NewVersion(tag)
	if err != nil {
		return Tag{}, err
	}
	rcNum := -1
	if rc := v.Prerelease(); rc != "" {
		// We expect only -rc.X suffixes, nothing else.
		matches := rcRe.FindStringSubmatch(rc)
		if len(matches) < 2 {
			return Tag{}, fmt.Errorf(`tag suffix has to be empty or match <tag>-rc.(\d+)$ regex; got %v`, rc)
		}
		rcNum, err = strconv.Atoi(matches[1])
		if err != nil {
			panic(fmt.Errorf("regexp suppose to validate it's a number, but we can't parse it %v; %w", matches[1], err))
		}
	}
	return Tag{Version: *v, rc: rcNum}, nil
}

func (t Tag) String() string {
	return "v" + t.Version.String()
}

func (t Tag) RC() int {
	return t.rc
}

// NextRC returns the next RC tag to release, bases on the current one.
// If the current one is not an RC, new patch RC.0 will be returned.
func (t Tag) NextRC() Tag {
	version := t.Version
	if t.rc == -1 {
		// Current tag is not a RC, we need to bump patch version to cut a new RC.
		version = t.Version.IncPatch()
	}
	rcNum := t.rc + 1
	var err error
	version, err = version.SetPrerelease(fmt.Sprintf("rc.%v", rcNum))
	if err != nil {
		panic(err)
	}
	return Tag{Version: version, rc: rcNum}
}
