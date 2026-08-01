package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestQueueRemainingReadsWhatAmenboSaidIsWaiting(t *testing.T) {
	t.Setenv(envQueueRemaining, "3")

	if got := queueRemaining(); got != 3 {
		t.Errorf("queueRemaining() = %d, want the 3 amenbo said were waiting", got)
	}
}

// Every way the count can be missing is read as none waiting: an older amenbo that does not set
// it, a hand run dispatched off no queue, a value that is not a count at all. Reading any of them
// as a burst would hold a line back for a run that is never started.
func TestQueueRemainingReadsWhatItCannotUseAsNoneWaiting(t *testing.T) {
	for _, value := range []string{"", "  ", "later", "-1", "1.5"} {
		t.Setenv(envQueueRemaining, value)
		if got := queueRemaining(); got != 0 {
			t.Errorf("queueRemaining() = %d on %q, want none waiting", got, value)
		}
	}
}

// waitingLines is what a set of entries says, for a test that is about the order and not the rest.
func waitingLines(waiting []entry) string {
	return strings.Join(texts(waiting), "|")
}

// aLine is an entry a test writes down when what it was about does not matter.
func aLine(text string) entry {
	return entry{Text: text}
}

func TestKeepPendingWritesTheLineDownBehindTheOnesWaiting(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)

	lines, kept := keepPending(s, []entry{aLine("the first")}, aLine("the second"))
	if !kept {
		t.Fatalf("keepPending said it could not hold a line with somewhere to write it")
	}
	if waitingLines(lines) != "the first|the second" {
		t.Errorf("lines = %v, want the waiting one and then this one", texts(lines))
	}
	if waitingLines(pending(s)) != "the first|the second" {
		t.Errorf("on disk = %v, want what a run that does not come back would leave behind", texts(pending(s)))
	}
}

// A run with nowhere to write cannot hold anything back, so its event is sent on its own.
func TestKeepPendingSendsOnItsOwnWithNowhereToWrite(t *testing.T) {
	lines, kept := keepPending(state{}, nil, aLine("the only one"))

	if kept {
		t.Errorf("keepPending held a line with nowhere to write it")
	}
	if waitingLines(lines) != "the only one" {
		t.Errorf("lines = %v, want this event by itself", lines)
	}
}

// A write that does not land leaves what is already waiting exactly where it was, so this run
// sends its own line and the next message that gets through carries the rest — nothing twice.
func TestKeepPendingLeavesWhatIsWaitingWhenTheWriteFails(t *testing.T) {
	home := t.TempDir()
	s := stateAt(home, someReach)
	if _, kept := keepPending(s, nil, aLine("the first")); !kept {
		t.Fatalf("the line that was to wait could not be written down")
	}
	// A folder no run may write in is how a disk that will not take the write is held still.
	if err := os.Chmod(s.dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(s.dir, 0o700) })
	out := captureErr(t)

	lines, kept := keepPending(s, pending(s), aLine("the second"))
	if kept {
		t.Errorf("keepPending held a line its write did not land")
	}
	if waitingLines(lines) != "the second" {
		t.Errorf("lines = %v, want this event by itself", texts(lines))
	}
	if waitingLines(pending(s)) != "the first" {
		t.Errorf("on disk = %v, want the line that was already waiting, untouched", texts(pending(s)))
	}
	if !strings.Contains(out.String(), "sent on its own") {
		t.Errorf("nothing said about a write that did not land: %q", out.String())
	}
}

// Nothing sent for two hundred lines is a plugin configured wrongly, not a burst — so the oldest
// give way rather than the disk filling up quietly.
func TestKeepPendingDropsTheOldestOverTheLimit(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)
	var waiting []entry
	for i := 0; i < pendingKept; i++ {
		waiting = append(waiting, aLine("line "+strconv.Itoa(i)))
	}
	out := captureErr(t)

	lines, kept := keepPending(s, waiting, aLine("the newest"))
	if !kept {
		t.Fatalf("keepPending said it could not hold a line with somewhere to write it")
	}
	if len(lines) != pendingKept {
		t.Errorf("kept %d lines, want the %d that are held at most", len(lines), pendingKept)
	}
	if lines[0].Text != "line 1" {
		t.Errorf("oldest kept = %q, want the one after the line that gave way", lines[0].Text)
	}
	if lines[len(lines)-1].Text != "the newest" {
		t.Errorf("newest kept = %q, want this run's line", lines[len(lines)-1].Text)
	}
	if !strings.Contains(out.String(), "waited longest were dropped") {
		t.Errorf("nothing said about the line that was dropped: %q", out.String())
	}
}

func TestDropPendingForgetsWhatAMessageCarried(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)
	if _, kept := keepPending(s, nil, aLine("carried away")); !kept {
		t.Fatalf("the line that was to wait could not be written down")
	}

	dropPending(s)

	if got := pending(s); len(got) != 0 {
		t.Errorf("still waiting: %v, want nothing after the message carried it", got)
	}
}

// Two projects report to whatever two mailboxes their settings name, so what waits for one of them
// is never read back by the other.
func TestPendingIsHeldPerProject(t *testing.T) {
	home := t.TempDir()
	if _, kept := keepPending(stateAt(home, someReach), nil, aLine("the first project's")); !kept {
		t.Fatalf("the line that was to wait could not be written down")
	}

	if got := pending(stateAt(home, refNamespace+"P-2")); len(got) != 0 {
		t.Errorf("the other project is waiting on %v, which is not its work", got)
	}
}

// A line is a line: an entry carrying a newline of its own would be read back as two, and the
// second half would arrive looking like an event that never happened.
func TestPendingReadsBackEveryLineItWasGiven(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)
	for _, line := range []string{"first", "second", "third"} {
		if _, kept := keepPending(s, pending(s), aLine(line)); !kept {
			t.Fatalf("the line that was to wait could not be written down")
		}
	}

	if got := waitingLines(pending(s)); got != "first|second|third" {
		t.Errorf("waiting = %q, want every line in the order it arrived", got)
	}
	if _, err := os.Stat(filepath.Join(s.dir, pendingFile)); err != nil {
		t.Errorf("nothing was written where the lines are kept: %v", err)
	}
}

// An entry goes to disk and comes back whole: the line for the body, and what it was about for the
// subject the run that finally carries it has to write.
func TestAnEntryComesBackWithWhatItWasAbout(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)
	in := anEventAt(eventTaskStatusChanged, "2026-08-01T05:33:10Z")
	in.New = statusInProgress
	d := spoken("ja")

	if _, kept := keepPending(s, nil, entryFor(in, d)); !kept {
		t.Fatalf("the line that was to wait could not be written down")
	}

	waiting := pending(s)
	if len(waiting) != 1 {
		t.Fatalf("waiting = %v, want the one line written down", texts(waiting))
	}
	got := waiting[0]
	if got.Event != eventTaskStatusChanged || got.Ref != sampleRef || got.Status != statusInProgress {
		t.Errorf("entry = %+v, want what the subject is written from", got)
	}
	if !strings.Contains(got.Text, sampleRef) {
		t.Errorf("line = %q, want the line the body carries", got.Text)
	}
}

// A newline inside a title would end the entry and make the rest of it look like an event that
// never happened, so an entry is one line on disk whatever is written into it.
func TestAnEntryIsOneLineHoweverTheTitleReads(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)

	if _, kept := keepPending(s, nil, entry{Text: "first\nsecond", Event: eventTaskDone}); !kept {
		t.Fatalf("the line that was to wait could not be written down")
	}

	waiting := pending(s)
	if len(waiting) != 1 {
		t.Fatalf("waiting = %v, want the one entry it was given", texts(waiting))
	}
	if waiting[0].Text != "first\nsecond" {
		t.Errorf("line = %q, want it back as it was written", waiting[0].Text)
	}
}

// A line waiting from a build that wrote lines and nothing else is still a line. It is carried
// rather than dropped, and the message that carries it says how many rather than what happened.
func TestALineFromAnOlderBuildIsStillCarried(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)
	if err := s.setLines(pendingFile, []string{"2026-08-01 14:33:41  AI finished " + sampleRef}); err != nil {
		t.Fatalf("setLines: %v", err)
	}

	waiting := pending(s)
	if len(waiting) != 1 {
		t.Fatalf("waiting = %v, want the line an older build left behind", texts(waiting))
	}
	if waiting[0].Text != "2026-08-01 14:33:41  AI finished "+sampleRef {
		t.Errorf("line = %q, want it read back as the line it is", waiting[0].Text)
	}
	if waiting[0].Event != "" {
		t.Errorf("event = %q, want nothing — an older build wrote none", waiting[0].Event)
	}
}
