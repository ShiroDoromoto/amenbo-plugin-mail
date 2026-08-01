package main

import (
	"bytes"
	"strings"
	"testing"
)

// captureErr redirects the diagnostics a run writes, and answers with what it wrote.
func captureErr(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	was := errOut
	errOut = &buf
	t.Cleanup(func() { errOut = was })
	return &buf
}

// event is a payload for one AI-driven event, configured well enough to be sent on.
func event(name string) input {
	return input{
		V:     contractVersion,
		Event: name,
		ID:    42,
		Actor: actorAI,
		At:    "2026-08-01T09:00:00Z",
		Config: map[string]any{
			keySMTPHost: "smtp.example.com",
			keySMTPUser: "you@example.com",
		},
	}
}

func TestHookReportsAChosenEvent(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "app-password")
	out := captureErr(t)

	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if !strings.Contains(out.String(), eventTaskDone) {
		t.Errorf("nothing said about the event: %q", out.String())
	}
}

func TestHookSaysNothingAboutAnEventThatWasNotChosen(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "app-password")
	out := captureErr(t)

	if err := hook(event(eventCommentAdded)); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q about an event nobody asked to hear about", out.String())
	}
}

// The settings are read after the event has earned a message, so an event this plugin was never
// going to report cannot be the one that complains the plugin is unconfigured.
func TestHookDoesNotComplainAboutSettingsForAnEventItSkips(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "")
	out := captureErr(t)

	in := event(eventCommentAdded)
	in.Config = nil

	if err := hook(in); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q about an event nobody asked to hear about", out.String())
	}
}

// A write the user drove is one they were present for, and it is skipped before anything is
// asked of the configuration.
func TestHookSaysNothingAboutTheUsersOwnWrites(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "")
	out := captureErr(t)

	in := event(eventTaskDone)
	in.Actor = "human"
	in.Config = nil

	if err := hook(in); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q about a write the user drove themselves", out.String())
	}
}

func TestHookRefusesAContractItDoesNotRead(t *testing.T) {
	in := event(eventTaskDone)
	in.V = contractVersion + 1

	if err := hook(in); err == nil {
		t.Fatalf("hook read a payload written to a contract it does not know")
	}
}

func TestHookDoesNothingWhenNoEventFired(t *testing.T) {
	out := captureErr(t)

	if err := hook(input{}); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q with no event to report", out.String())
	}
}

// Choosing none leaves the plugin switched on and silent — including on the events it would
// otherwise have reported by default. amenbo delivers that choice as an empty list.
func TestHookReportsNothingWhenNoEventsWereChosen(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "app-password")
	out := captureErr(t)

	in := event(eventTaskDone)
	in.Config[keyEvents] = ""

	if err := hook(in); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q with nothing chosen to report", out.String())
	}
}
