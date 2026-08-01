package main

import "fmt"

// amenbo cannot promise it delivers an event once. A plugin that finishes with an event and
// dies before amenbo has taken that event off its queue is handed the same one again on the
// next flush, and amenbo has no way of knowing the first run got that far. So the second copy
// is this plugin's to notice: nothing in a message says which of two identical lines was the
// replay, and a reader has no way to tell either.

// seenFile is what the events taken in are kept under, in the project's own folder.
const seenFile = "seen"

// seenKept is how many are remembered. A redelivery happens inside the queue it was delivered
// from, so what has to be remembered is one burst of work and not the store's whole history —
// and the record is read and written whole on every event, which is a reason of its own not to
// let it grow without end.
const seenKept = 200

// seenKey is the line one event is remembered by: what happened, what it happened to, and when
// it happened. The moment is what separates a replay from the user acting twice — amenbo hands
// a redelivery the same `at` it gave the first copy, while a person doing the same thing again
// makes a second event at a second moment, and both of those are worth reporting.
func seenKey(in input) string {
	return fmt.Sprintf("%s\t%d\t%s", in.Event, in.ID, in.At)
}

// takeIn records an event as taken in and reports whether it is this project's first sight of
// it. False is a redelivery, and a caller that gets it has nothing to add to the message.
//
// It is recorded here, on the way in, rather than where the message is finally sent. Lines wait
// for the send that carries them, and a record written at the send would let a redelivery
// arriving during that wait be taken in a second time — putting the same line in the message
// twice, which is the one thing this is for.
//
// An event that cannot be remembered is taken in all the same: a project with nowhere to write,
// a record that will not save. Both leave the plugin able to report and unable to tell a second
// copy from a first, and of the two failures a notification nobody gets is the worse one.
func takeIn(s state, in input) bool {
	// Without the moment there is no telling a replay from the same thing done twice, and
	// answering that question wrongly here silences a real event. So it is not remembered,
	// and every copy of it is reported.
	if in.At == "" {
		return true
	}
	key := seenKey(in)
	kept := s.lines(seenFile)
	for _, k := range kept {
		if k == key {
			return false
		}
	}
	if !s.remembers() {
		return true
	}
	kept = append(kept, key)
	if len(kept) > seenKept {
		kept = kept[len(kept)-seenKept:]
	}
	if err := s.setLines(seenFile, kept); err != nil {
		logf("%s: %s on %d is reported without being remembered, so a second copy of it would be reported too: %v",
			pluginName, in.Event, in.ID, err)
	}
	return true
}
