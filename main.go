// Command mail is amenbo's email notification plugin: it reports by email what the user's AI
// did in a project while nobody was watching it.
//
// It has one face, the observation hook. amenbo fires it with NO arguments and the event's
// JSON on stdin, nobody is waiting for the answer, and what it does with the event is send
// one message. There is no command face: nothing here is worth invoking on purpose, and a
// plugin that only observes says so by refusing an argument rather than by inventing a verb.
//
// Two things shape what it sends.
//
//   - **Only the AI's writes.** The payload names who drove the write, and a write the user
//     drove is one they were present for — a mailbox that repeats it back is noise. What is
//     worth a notification is the work that happened while they were away from the desk.
//   - **The mailbox is the setting.** Where a project reports to is `to`, and a setting belongs
//     to a project, so the value itself is which mailbox that project reports to. There is no
//     address anywhere in this code.
//
// A payload carries an id, never the record, so the title in a message is read back by
// running `amenbo task show <id> --json`. amenbo names the store and the window it may be
// read through in the environment; this plugin passes neither on and adds nothing of its own.
//
// Here are the entry point and the payload contract; config.go has the settings a message is
// sent on, events.go which events earn one, reader.go what amenbo is asked to fill a number in
// with, state.go the per-project folder that whatever has to outlive one run is kept in, seen.go
// the record of the events already taken in — what tells a redelivery from a first sight of one
// — pending.go the lines waiting for the message that carries them, send.go the SMTP conversation
// that carries it, wording.go the one line an event becomes in the language amenbo is set to,
// subject.go the line a message opens with, and body.go what is written under it. The hook below
// is the order those are put in.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// contractVersion is the payload contract this plugin reads. amenbo leads every document it
// writes with `v` and raises it only on a breaking change — new fields are added silently —
// so a document announcing a different version is one this plugin must not guess at.
const contractVersion = 1

// actorAI is the one actor whose writes are reported. The other is the user themselves, who
// was present for their own writes.
const actorAI = "ai"

// pluginName is what amenbo knows this plugin as: its manifest's name, its installed directory, and
// the word a user types after `plugin`. One spelling, so what is written under it is found again.
const pluginName = "mail"

// errOut is where everything a person reads goes, indirected so the tests can read it back.
// A hook's stdout is not a return value, so nothing here writes to it: stderr is where
// amenbo's execution log looks when a run has to explain itself.
var errOut io.Writer = os.Stderr

// logf writes one diagnostic line to stderr.
func logf(format string, a ...any) {
	fmt.Fprintf(errOut, format+"\n", a...)
}

// input is the JSON document amenbo writes to the plugin's stdin. Unknown keys are ignored —
// the contract grows by addition, so a plugin that refused them would break on the next
// amenbo.
type input struct {
	// V is the contract version the document is written to.
	V int `json:"v"`
	// Event is the event's namespace name, e.g. "task.done". Empty when nothing fired.
	Event string `json:"event"`
	// ID is the affected record's conversational number — the id a person knows it by.
	ID int64 `json:"id"`
	// Actor is who drove the write: "human" or "ai".
	Actor string `json:"actor"`
	// At is when the event fired, as "2026-07-22T09:00:00Z". Redelivery of one event carries the
	// same moment, which is what tells a replay from the user acting twice.
	At string `json:"at"`
	// New is the record's state after the change, for the events whose name does not
	// already say it.
	New string `json:"new"`
	// Record is the vanished record itself, on the deletion events alone: there is nothing
	// left to read back, so the row travels on the wire in its place. Its keys are the
	// record's own columns.
	Record map[string]any `json:"record"`
	// Parent is what a child record hangs on, by number — the task of a comment, added or
	// taken back. Nil on every event that has no parent, and on an older amenbo that carried
	// none for one that does.
	Parent *int64 `json:"parent"`
	// Config holds the plugin's own non-secret settings, as the user filled them in. Secrets
	// never appear here: amenbo puts those in the environment instead.
	Config map[string]any `json:"config"`
}

// readInput reads the document amenbo feeds on stdin.
//
// amenbo always writes one and closes the pipe, so the read finishes promptly. A hand run
// from a terminal is fed nothing at all, and waiting for a person to type JSON would hang
// the plugin on `mail help` — so an interactive stdin is skipped rather than read. A
// document that will not parse is reported and dropped: nobody is waiting on the answer, and
// there is no event left to report.
func readInput(f *os.File) input {
	if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		return input{}
	}
	raw, err := io.ReadAll(f)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return input{}
	}
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		logf("%s: ignoring an input document that will not parse: %v", pluginName, err)
		return input{}
	}
	return in
}

// hook is what one event comes to, from the judgement that it is worth reporting to the message
// that reports it.
//
// The order is what makes a quiet run quiet. An event this plugin is not going to report is no
// reason to complain that it is unconfigured, and neither is one it could not have sent — so the
// settings are read after the event has earned a message, and amenbo is asked about the record
// only once there is somewhere to send what it says.
//
// Whether the event adds a line and whether a message goes out are separate questions, and
// answering them separately is what keeps a line from being stranded. A run says nothing of its own
// when the user drove the write, when nobody asked to hear about the event, or when it is a second
// copy of one already taken in — and any of those can be the run amenbo ends a burst with. Held
// lines have to leave on it all the same, or they wait for an event that may be days away.
//
// A read that will not come back does not stop the run: what came back is used and the failure is
// answered with, so the exit code puts it in the execution log without costing the user the
// message.
func hook(in input) error {
	if in.Event == "" {
		return nil
	}
	if in.V != contractVersion {
		return fmt.Errorf("payload contract v%d is not the v%d this build reads", in.V, contractVersion)
	}

	s := stateFromEnv()
	reported := in.Actor == actorAI && selectedEvents(in.Config)[in.Event]
	fresh := reported && takeIn(s, in)
	lines := pending(s)
	if !fresh && (len(lines) == 0 || queueRemaining() > 0) {
		// Nothing of this run's own to say, and nothing waiting that this run has to carry.
		// Answering here is what keeps an event nobody asked about from being the one that
		// complains the plugin is unconfigured.
		return nil
	}

	cfg, err := loadConfig(in.Config)
	if err != nil {
		return err
	}

	var d details
	var readErr error
	held := true
	if fresh {
		d, readErr = lookup(in)
		lines, held = keepPending(s, lines, eventLine(in, d))
	} else {
		d, readErr = surroundings()
	}
	if held && queueRemaining() > 0 {
		// The burst is not over. The lines are on disk, and the run amenbo ends it with sends them.
		return readErr
	}

	subject := subjectForMany(d, len(lines))
	if fresh && len(lines) == 1 {
		// The message carries this event and nothing else, so it can say what happened.
		subject = subjectForOne(in, d)
	}
	if err := sendMessage(cfg, subject, messageBody(d.project, lines)); err != nil {
		// The lines stay where they are. amenbo does not hand a failed event back, so a message
		// this could not deliver is one the next message has to carry.
		return errors.Join(readErr, err)
	}
	logf("%s: %d event(s) reported to %s", pluginName, len(lines), strings.Join(cfg.to, ", "))
	if held {
		dropPending(s)
	}
	return readErr
}

func main() {
	in := readInput(os.Stdin)
	args := os.Args[1:]

	// No arguments is the observation face — amenbo fired us for an event.
	if len(args) == 0 {
		do(hook(in))
		return
	}

	switch args[0] {
	case "help", "-h", "--help":
		usage()
	default:
		logf("%s: %q is not a command — this plugin only reports events", pluginName, strings.Join(args, " "))
		usage()
		os.Exit(2)
	}
}

// do ends the run on the verdict the exit code carries. A hook's failure reaches nobody who
// was listening, so the exit code is what puts it in amenbo's execution log, beside the
// stderr that says why.
func do(err error) {
	if err != nil {
		logf("%s: %v", pluginName, err)
		os.Exit(1)
	}
}

func usage() {
	logf(`mail — amenbo's email notification plugin: report your AI's writes to a mailbox

This plugin is not called. amenbo starts it when an event fires, and it reports the event by
email, under a heading naming the project it came from.

Events that come one after another arrive as one message: while amenbo says more is waiting, each
line is written down instead of sent, and the event that ends the burst carries all of them.

Only the writes an AI drove are reported: the ones you drove yourself, you were there for.
Which of them reach the mailbox is yours to choose — by default a task created, its status
moved, and either terminal (done or decided against).

Settings — three are required, and the rest are derived from them:
  smtp_host      the SMTP server to hand the message to (required)
  smtp_user      the account to authenticate as (required)
  smtp_password  that account's password (required, secret)
  smtp_port      the port on the server (defaults to 587)
  from           the address the message is sent from (defaults to smtp_user)
  to             where it is sent (defaults to smtp_user; comma-separated for several)
  events         what to report, from the eleven amenbo fires (defaults to the four above;
                 choosing none is honoured, and reports nothing)

Fill in the three and the plugin reports to the account's own mailbox; 'to' is what sends it
somewhere else. Every setting belongs to a project, so the value is which mailbox that project
reports to. Fill them in with 'amenbo plugin config set mail smtp_host <host>', then switch the
plugin on for the project with 'amenbo plugin enable mail'.

Why nothing arrived is in 'amenbo plugin log mail' — one line per run, and the diagnostics
of any run that did not end cleanly.`)
}
