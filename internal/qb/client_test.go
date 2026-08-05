package qb

import "testing"

func TestNormalizeState(t *testing.T) {
	cases := map[string]string{
		"downloading":   "running",
		"forcedDL":      "running",
		"metaDL":        "running",
		"stalledDL":     "running",
		"uploading":     "seeding",
		"forcedUP":      "seeding",
		"pausedDL":      "paused",
		"pausedUP":      "paused",
		"error":         "error",
		"missingFiles":  "error",
		"completed":     "finished",
		"unknownState":  "running",
	}
	for in, want := range cases {
		if got := normalizeState(in); got != want {
			t.Fatalf("normalizeState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinURLs(t *testing.T) {
	if got := joinURLs([]string{" magnet:1 ", "", " magnet:2"}); got != "magnet:1\nmagnet:2" {
		t.Fatalf("joinURLs = %q", got)
	}
}
