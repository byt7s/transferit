package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	got := formatVersion("devel", []debug.BuildSetting{
		{Key: "vcs.revision", Value: "28d2020abcdef1234567890"},
		{Key: "vcs.time", Value: "2026-10-15T12:37:05Z"},
		{Key: "vcs.modified", Value: "true"},
	})
	want := "transferit devel\n28d2020abcde-dirty  ·  2026-10-15 12:37 UTC"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestReleaseVersionIgnoresPseudoVersions(t *testing.T) {
	for _, v := range []string{"", "(devel)", "v0.0.0-20261015123631-28d20203e373+dirty"} {
		if got := releaseVersion(v); got != "" {
			t.Fatalf("releaseVersion(%q) = %q", v, got)
		}
	}
	if got := releaseVersion("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatVersionRelease(t *testing.T) {
	got := formatVersion("v1.2.3", nil)
	if got != "transferit v1.2.3" {
		t.Fatalf("got %q", got)
	}
}

func TestCurrentVersionMentionsTransferit(t *testing.T) {
	if !strings.HasPrefix(Current(), "transferit ") {
		t.Fatal(Current())
	}
}
