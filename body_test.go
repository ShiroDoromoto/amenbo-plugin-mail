package main

import (
	"strings"
	"testing"
)

func TestMessageBodyOpensWithTheProjectThenTheEvents(t *testing.T) {
	got := messageBody("amenbo-plugin-mail", []string{"first line", "second line"})

	want := "amenbo-plugin-mail\n\nfirst line\nsecond line\n"
	if got != want {
		t.Errorf("messageBody = %q, want %q", got, want)
	}
}

func TestMessageBodyWithoutAProjectStartsAtTheEvents(t *testing.T) {
	got := messageBody("", []string{"first line", "second line"})

	if want := "first line\nsecond line\n"; got != want {
		t.Errorf("messageBody = %q, want %q", got, want)
	}
	if strings.HasPrefix(got, "\n") {
		t.Errorf("messageBody = %q, want no room left where the heading would be", got)
	}
}

func TestMessageBodyKeepsTheOrderItWasGiven(t *testing.T) {
	lines := []string{"2026-08-01 14:32:05  one", "2026-08-01 14:33:10  two", "2026-08-01 14:51:02  three"}

	got := messageBody("amenbo", lines)

	if want := "amenbo\n\n" + strings.Join(lines, "\n") + "\n"; got != want {
		t.Errorf("messageBody = %q, want %q", got, want)
	}
}

func TestMessageBodyLeavesOutALineWithNothingOnIt(t *testing.T) {
	got := messageBody("amenbo", []string{"first line", "", "   ", "second line"})

	if want := "amenbo\n\nfirst line\nsecond line\n"; got != want {
		t.Errorf("messageBody = %q, want %q", got, want)
	}
}

func TestMessageBodyWithNothingToReportIsJustTheHeading(t *testing.T) {
	if got, want := messageBody("amenbo", nil), "amenbo\n"; got != want {
		t.Errorf("messageBody = %q, want %q", got, want)
	}
	if got := messageBody("", nil); got != "" {
		t.Errorf("messageBody = %q, want nothing at all", got)
	}
}

func TestMessageBodyIsWhatTheEventsSaidAndNothingAroundIt(t *testing.T) {
	d := spoken("ja")
	d.project = "amenbo-plugin-mail"
	first := eventLine(anEventAt(eventTaskCreated, "2026-08-01T05:32:05Z"), d)
	second := eventLine(anEventAt(eventTaskDone, "2026-08-01T05:51:02Z"), d)

	got := messageBody(d.project, []string{first, second})

	if want := "amenbo-plugin-mail\n\n" + first + "\n" + second + "\n"; got != want {
		t.Errorf("messageBody = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "<>") {
		t.Errorf("messageBody = %q, which is not the plain text it is sent as", got)
	}
}
