package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// anEvent is one event as amenbo delivers it, for the tests that vary one thing about it.
func anEvent() input {
	return input{V: contractVersion, Event: "task.done", ID: 42, Actor: actorAI, At: "2026-08-01T09:00:00Z"}
}

// catchStderr reads back what the run said, for the duration of one test.
func catchStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	was := errOut
	errOut = &buf
	t.Cleanup(func() { errOut = was })
	return &buf
}

func TestTakeInReportsTheSameEventOnlyOnce(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")
	in := anEvent()

	if !takeIn(s, in) {
		t.Fatal("the first sight of an event was taken for a redelivery")
	}
	if takeIn(s, in) {
		t.Error("a redelivery was taken in a second time")
	}
}

func TestTakeInTellsTheSameThingDoneTwiceApartFromAReplay(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")
	first := anEvent()

	if !takeIn(s, first) {
		t.Fatal("the first sight of an event was taken for a redelivery")
	}
	for _, tc := range []struct {
		name  string
		event input
	}{
		{"a second moment is a second event", func() input { e := anEvent(); e.At = "2026-08-01T09:30:00Z"; return e }()},
		{"another record is another event", func() input { e := anEvent(); e.ID = 43; return e }()},
		{"another thing happening is another event", func() input { e := anEvent(); e.Event = "task.rejected"; return e }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !takeIn(s, tc.event) {
				t.Error("it was taken for a redelivery of the first")
			}
		})
	}
}

func TestTakeInRecordsOnTheWayInRatherThanAtTheSend(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")

	takeIn(s, anEvent())

	if got := s.lines(seenFile); len(got) != 1 || got[0] != seenKey(anEvent()) {
		t.Errorf("the record holds %q, want the event that was just taken in", got)
	}
}

func TestTakeInForgetsTheOldestOnceItIsFull(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")
	at := func(i int) input {
		e := anEvent()
		e.At = fmt.Sprintf("2026-08-01T09:00:00.%03dZ", i)
		return e
	}

	for i := range seenKept + 10 {
		if !takeIn(s, at(i)) {
			t.Fatalf("event %d was taken for a redelivery", i)
		}
	}
	if got, want := len(s.lines(seenFile)), seenKept; got != want {
		t.Errorf("the record holds %d, want no more than %d", got, want)
	}
	if !takeIn(s, at(0)) {
		t.Error("the oldest event is still remembered, so nothing was forgotten")
	}
	if takeIn(s, at(seenKept+9)) {
		t.Error("the newest event was forgotten before the oldest")
	}
}

func TestTakeInReportsWhatItCannotRemember(t *testing.T) {
	stderr := catchStderr(t)
	s := stateAt(t.TempDir(), "project-a")
	// A file where the project's folder should be: nothing can be written under it.
	if err := os.MkdirAll(filepath.Dir(s.dir), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(s.dir, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if !takeIn(s, anEvent()) {
		t.Error("an event that could not be remembered was dropped instead of reported")
	}
	if !strings.Contains(stderr.String(), "task.done") {
		t.Errorf("stderr = %q, want the event it could not remember", stderr.String())
	}
}

func TestTakeInWithNowhereToRememberReportsEveryCopy(t *testing.T) {
	stderr := catchStderr(t)
	s := stateAt("", "project-a")

	if !takeIn(s, anEvent()) || !takeIn(s, anEvent()) {
		t.Error("an event was dropped by a run with nowhere to remember it")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing: having nowhere to write is not a fault to report on every event", stderr.String())
	}
}

func TestTakeInReportsAnEventCarryingNoMoment(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")
	in := anEvent()
	in.At = ""

	if !takeIn(s, in) || !takeIn(s, in) {
		t.Error("an event with no moment was taken for a redelivery, which cannot be told from a repeat")
	}
	if got := s.lines(seenFile); got != nil {
		t.Errorf("the record holds %q, want nothing it cannot tell apart", got)
	}
}

func TestOneProjectDoesNotRememberForAnother(t *testing.T) {
	home := t.TempDir()
	one, two := stateAt(home, "project-a"), stateAt(home, "project-b")

	takeIn(one, anEvent())

	if !takeIn(two, anEvent()) {
		t.Error("the other project treated it as already seen")
	}
}
