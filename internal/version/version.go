// Package version reports the build tag set at link time and the embedded commit.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// Label is replaced with -ldflags "-X transferit/internal/version.Label=v1.2.3".
var Label = "devel"

func Print() {
	fmt.Println(Current())
}

func Current() string {
	label := Label
	info, ok := debug.ReadBuildInfo()
	if !ok {
		if label == "" {
			label = "devel"
		}
		return "transferit " + label
	}
	if label == "" || label == "devel" {
		if v := releaseVersion(info.Main.Version); v != "" {
			label = v
		}
	}
	if label == "" {
		label = "devel"
	}
	return formatVersion(label, info.Settings)
}

func formatVersion(label string, settings []debug.BuildSetting) string {
	line := "transferit " + label
	rev, when, dirty := vcsStamp(settings)
	var extra []string
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if dirty {
			rev += "-dirty"
		}
		extra = append(extra, rev)
	}
	if when != "" {
		if t, err := time.Parse(time.RFC3339, when); err == nil {
			extra = append(extra, t.UTC().Format("2006-01-02 15:04 UTC"))
		} else {
			extra = append(extra, when)
		}
	}
	if len(extra) == 0 {
		return line
	}
	return line + "\n" + strings.Join(extra, "  ·  ")
}

func releaseVersion(v string) string {
	if v == "" || v == "(devel)" || strings.HasPrefix(v, "v0.0.0-") {
		return ""
	}
	return v
}

func vcsStamp(settings []debug.BuildSetting) (rev, when string, dirty bool) {
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			rev = setting.Value
		case "vcs.time":
			when = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	return rev, when, dirty
}
