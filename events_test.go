package main

import "testing"

// A user who has not chosen hears the outline of what happened while they were away, and nothing
// past it.
func TestSelectedEventsFallsBackToTheDefaultFour(t *testing.T) {
	got := selectedEvents(map[string]any{keySMTPHost: "smtp.example.com"})

	for _, event := range defaultEvents {
		if !got[event] {
			t.Errorf("%s is not reported, but it is one of the defaults", event)
		}
	}
	if len(got) != len(defaultEvents) {
		t.Errorf("%d events reported, want the %d defaults", len(got), len(defaultEvents))
	}
	if got[eventCommentAdded] {
		t.Errorf("%s is reported without being chosen", eventCommentAdded)
	}
}

func TestSelectedEventsHonoursWhatWasChosen(t *testing.T) {
	got := selectedEvents(map[string]any{keyEvents: " comment.added , task.deleted ,"})

	if !got[eventCommentAdded] || !got[eventTaskDeleted] {
		t.Errorf("chosen events not reported: %v", got)
	}
	if got[eventTaskDone] {
		t.Errorf("%s is reported, but choosing replaces the defaults rather than adding to them", eventTaskDone)
	}
}

// Picking none of the candidates is an answer. amenbo delivers it as an empty list, so an empty
// list must not be read back as a setting waiting to be filled in with the default.
func TestSelectedEventsReportsNothingWhenNoneWereChosen(t *testing.T) {
	for _, chosen := range []string{"", "  ", ","} {
		if got := selectedEvents(map[string]any{keyEvents: chosen}); len(got) != 0 {
			t.Errorf("events=%q reports %v, want nothing", chosen, got)
		}
	}
}

// amenbo refuses to store a candidate the manifest does not offer, so a name that is not an
// event only ever comes from a payload written by hand — and matching it against the event is
// the whole of the check either way.
func TestSelectedEventsMatchesNothingOnAnUnknownName(t *testing.T) {
	got := selectedEvents(map[string]any{keyEvents: "task.dne,task.done"})

	if !got[eventTaskDone] {
		t.Errorf("%s is not reported, but it was chosen alongside a name that is not an event", eventTaskDone)
	}
	if got["task.dne"] && len(got) != 2 {
		t.Errorf("selection = %v", got)
	}
}

func TestDefaultEventsAreAllReportable(t *testing.T) {
	reportable := eventSet(reportableEvents)
	for _, event := range defaultEvents {
		if !reportable[event] {
			t.Errorf("%s is reported by default but is not one a user can pick", event)
		}
	}
}
