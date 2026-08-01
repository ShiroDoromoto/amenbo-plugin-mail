package main

import (
	"os"
	"strconv"
	"strings"
)

// One thing the user does is often many events. Deleting a project, or clearing out the tasks
// that piled up in it, is one decision to them and a dozen writes to amenbo — and a dozen
// messages about one decision is not a notification, it is an interruption repeated. So a run
// does not send what it is given: it adds its line to the ones waiting and lets the run that ends
// the burst carry all of them in a single message.
//
// Which run that is comes from amenbo. A plugin cannot see the queue it is being dispatched off —
// it is started once per event and knows nothing of what is behind it — so the count amenbo puts
// in the environment is the only answer to "is there more coming", and it is taken at its word.
//
// The lines are written to disk before any of this, because a run that never comes back takes
// everything it was only holding in memory with it. They are written per project, under the
// folder state.go gives out: two projects report to whatever two mailboxes their settings name,
// and one shared pile would post the first project's work to the second one's address.

// envQueueRemaining names how many events amenbo still has for this project after this one.
const envQueueRemaining = "AMENBO_PLUGIN_REACH_QUEUE_REMAINING"

// pendingFile is what the lines waiting for a message are kept under, in the project's own folder.
const pendingFile = "pending"

// pendingKept is how many lines wait at most. Nothing sent for that long is a plugin configured
// wrongly rather than a burst, and without a limit that state fills the disk quietly and then, on
// the day it is fixed, arrives as one message thousands of lines long. Counting lines rather than
// measuring how long they have waited keeps this the same shape as the record of what has been
// taken in, and owes nothing to what the machine's clock says.
const pendingKept = 200

// queueRemaining is how many events are still waiting behind this one.
//
// Anything that is not a count this run can read is none waiting: an amenbo older than the
// variable does not set it, and a hand run from a terminal was dispatched off no queue at all.
// Reading either of those as a burst still to come would hold a line back for an ending run that
// is never started.
func queueRemaining() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envQueueRemaining)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// pending is what is waiting for a message, oldest first.
func pending(s state) []string {
	return s.lines(pendingFile)
}

// keepPending writes this run's line down behind the ones already waiting, and answers with what a
// message would carry and whether the burst may go on waiting for one.
//
// A run with nowhere to write, and one whose write did not land, cannot hold anything back: the
// answer is then this line by itself, for sending now. What is already on disk is left exactly as
// it was — a write that failed changed nothing — so it stays waiting for the next message that
// gets through, and no line is sent twice or dropped between the two paths.
func keepPending(s state, waiting []string, line string) ([]string, bool) {
	if !s.remembers() {
		return []string{line}, false
	}
	lines := append(waiting, line)
	if dropped := len(lines) - pendingKept; dropped > 0 {
		logf("%s: %d line(s) that had waited longest were dropped — %d is as many as are held, and nothing has been sent for that long",
			pluginName, dropped, pendingKept)
		lines = lines[dropped:]
	}
	if err := s.setLines(pendingFile, lines); err != nil {
		logf("%s: what is waiting could not be written down, so this event is sent on its own: %v", pluginName, err)
		return []string{line}, false
	}
	return lines, true
}

// dropPending forgets the lines a message has just carried.
//
// It runs after the server has taken the message, never before: a line forgotten first is one a
// failed send loses for good, while a line forgotten late at worst reaches the same mailbox twice.
// So a clear that will not write is reported and nothing more — the next message repeats those
// lines, which is the harmless half of the trade.
func dropPending(s state) {
	if !s.remembers() {
		return
	}
	if err := s.setLines(pendingFile, nil); err != nil {
		logf("%s: the lines just sent could not be forgotten, so the next message carries them again: %v",
			pluginName, err)
	}
}
