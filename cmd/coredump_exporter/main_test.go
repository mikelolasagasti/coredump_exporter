package main

import "testing"

func TestVersionString(t *testing.T) {
	oldVersion := version
	oldDate := date
	version = "1.2.3"
	date = "2026-07-12T12:34:56Z"
	t.Cleanup(func() {
		version = oldVersion
		date = oldDate
	})

	got := versionString()
	want := "coredump_exporter version=1.2.3 date=2026-07-12T12:34:56Z"
	if got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}
