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

func TestKeepPendingWritesTheLineDownBehindTheOnesWaiting(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)

	lines, held := keepPending(s, []string{"the first"}, "the second")
	if !held {
		t.Fatalf("keepPending said it could not hold a line with somewhere to write it")
	}
	if strings.Join(lines, "|") != "the first|the second" {
		t.Errorf("lines = %v, want the waiting one and then this one", lines)
	}
	if strings.Join(pending(s), "|") != "the first|the second" {
		t.Errorf("on disk = %v, want what a run that does not come back would leave behind", pending(s))
	}
}

// A run with nowhere to write cannot hold anything back, so its event is sent on its own.
func TestKeepPendingSendsOnItsOwnWithNowhereToWrite(t *testing.T) {
	lines, held := keepPending(state{}, nil, "the only one")

	if held {
		t.Errorf("keepPending held a line with nowhere to write it")
	}
	if strings.Join(lines, "|") != "the only one" {
		t.Errorf("lines = %v, want this event by itself", lines)
	}
}

// A write that does not land leaves what is already waiting exactly where it was, so this run
// sends its own line and the next message that gets through carries the rest — nothing twice.
func TestKeepPendingLeavesWhatIsWaitingWhenTheWriteFails(t *testing.T) {
	home := t.TempDir()
	s := stateAt(home, someReach)
	if _, held := keepPending(s, nil, "the first"); !held {
		t.Fatalf("the line that was to wait could not be written down")
	}
	// A folder no run may write in is how a disk that will not take the write is held still.
	if err := os.Chmod(s.dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(s.dir, 0o700) })
	out := captureErr(t)

	lines, held := keepPending(s, pending(s), "the second")
	if held {
		t.Errorf("keepPending held a line its write did not land")
	}
	if strings.Join(lines, "|") != "the second" {
		t.Errorf("lines = %v, want this event by itself", lines)
	}
	if strings.Join(pending(s), "|") != "the first" {
		t.Errorf("on disk = %v, want the line that was already waiting, untouched", pending(s))
	}
	if !strings.Contains(out.String(), "sent on its own") {
		t.Errorf("nothing said about a write that did not land: %q", out.String())
	}
}

// Nothing sent for two hundred lines is a plugin configured wrongly, not a burst — so the oldest
// give way rather than the disk filling up quietly.
func TestKeepPendingDropsTheOldestOverTheLimit(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)
	var waiting []string
	for i := 0; i < pendingKept; i++ {
		waiting = append(waiting, "line "+strconv.Itoa(i))
	}
	out := captureErr(t)

	lines, held := keepPending(s, waiting, "the newest")
	if !held {
		t.Fatalf("keepPending said it could not hold a line with somewhere to write it")
	}
	if len(lines) != pendingKept {
		t.Errorf("kept %d lines, want the %d that are held at most", len(lines), pendingKept)
	}
	if lines[0] != "line 1" {
		t.Errorf("oldest kept = %q, want the one after the line that gave way", lines[0])
	}
	if lines[len(lines)-1] != "the newest" {
		t.Errorf("newest kept = %q, want this run's line", lines[len(lines)-1])
	}
	if !strings.Contains(out.String(), "waited longest were dropped") {
		t.Errorf("nothing said about the line that was dropped: %q", out.String())
	}
}

func TestDropPendingForgetsWhatAMessageCarried(t *testing.T) {
	s := stateAt(t.TempDir(), someReach)
	if _, held := keepPending(s, nil, "carried away"); !held {
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
	if _, held := keepPending(stateAt(home, someReach), nil, "the first project's"); !held {
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
		if _, held := keepPending(s, pending(s), line); !held {
			t.Fatalf("the line that was to wait could not be written down")
		}
	}

	if got := strings.Join(pending(s), "|"); got != "first|second|third" {
		t.Errorf("waiting = %q, want every line in the order it arrived", got)
	}
	if _, err := os.Stat(filepath.Join(s.dir, pendingFile)); err != nil {
		t.Errorf("nothing was written where the lines are kept: %v", err)
	}
}
