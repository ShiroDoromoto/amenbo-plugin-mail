package main

import (
	"encoding/json"
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
//
// Each line is written down with what it was about, because the run that carries it is often not
// the run it arrived on — see entry.

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

// entry is one line waiting for a message, with what it was about kept beside it.
//
// The line alone would do for the body — it is what a reader sees under the heading — but a
// subject is written from the event rather than from the line, and the run that finally carries a
// line is often not the run its event arrived on. Nothing of that run's own says what the waiting
// line was; so the entry carries it, and a message that goes out late can still say what happened
// instead of only how much did.
type entry struct {
	// Text is the line itself, as the body carries it.
	Text string `json:"line"`
	// Event is the event's namespace name, e.g. "task.done", and Ref what it happened to, as
	// amenbo renders it. Both are empty on an entry a build before this one wrote down.
	Event string `json:"event,omitempty"`
	Ref   string `json:"ref,omitempty"`
	// Status is the state a record moved to, for the one event whose subject names it. It is the
	// word off the wire and not a translated one: which language a message is written in is
	// settled where it is sent, and that is not always where the line was held.
	Status string `json:"status,omitempty"`
}

// entryFor is what this run writes down about the event it was started for.
func entryFor(in input, d details) entry {
	return entry{Text: eventLine(in, d), Event: in.Event, Ref: refName(in, d), Status: in.New}
}

// pending is what is waiting for a message, oldest first.
func pending(s state) []entry {
	raw := s.lines(pendingFile)
	if len(raw) == 0 {
		return nil
	}
	waiting := make([]entry, 0, len(raw))
	for _, line := range raw {
		waiting = append(waiting, readEntry(line))
	}
	return waiting
}

// readEntry reads one entry back off the disk.
//
// Anything that does not come back as an entry is taken as a line and nothing more — which is
// exactly what a build that wrote lines and nothing else left behind, so an update carries what
// was already waiting instead of dropping it. A message then says how many rather than what
// happened, and the line still arrives.
func readEntry(raw string) entry {
	var e entry
	if err := json.Unmarshal([]byte(raw), &e); err != nil || e.Text == "" {
		return entry{Text: raw}
	}
	return e
}

// texts is the lines themselves, which is what the body is written from.
func texts(waiting []entry) []string {
	lines := make([]string, 0, len(waiting))
	for _, e := range waiting {
		lines = append(lines, e.Text)
	}
	return lines
}

// written turns the entries into the lines they are kept as. JSON is what holds an entry to one
// line: a title with a newline in it would otherwise be read back as two entries, and the second
// half would arrive looking like an event that never happened.
func written(waiting []entry) []string {
	lines := make([]string, 0, len(waiting))
	for _, e := range waiting {
		raw, err := json.Marshal(e)
		if err != nil {
			// A handful of strings does not fail to marshal. If one ever did, the line is
			// worth more than everything kept beside it.
			lines = append(lines, e.Text)
			continue
		}
		lines = append(lines, string(raw))
	}
	return lines
}

// keepPending writes this run's line down behind the ones already waiting, and answers with what a
// message would carry and whether the burst may go on waiting for one.
//
// A run with nowhere to write, and one whose write did not land, cannot hold anything back: the
// answer is then this line by itself, for sending now. What is already on disk is left exactly as
// it was — a write that failed changed nothing — so it stays waiting for the next message that
// gets through, and no line is sent twice or dropped between the two paths.
func keepPending(s state, waiting []entry, e entry) ([]entry, bool) {
	if !s.remembers() {
		return []entry{e}, false
	}
	lines := append(waiting, e)
	if dropped := len(lines) - pendingKept; dropped > 0 {
		logf("%s: %d line(s) that had waited longest were dropped — %d is as many as are held, and nothing has been sent for that long",
			pluginName, dropped, pendingKept)
		lines = lines[dropped:]
	}
	if err := s.setLines(pendingFile, written(lines)); err != nil {
		logf("%s: what is waiting could not be written down, so this event is sent on its own: %v", pluginName, err)
		return []entry{e}, false
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
