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
// sent on, state.go the per-project folder that whatever has to outlive one run is kept in, and
// seen.go the record of the events already taken in, which is what tells a redelivery from a
// first sight of one. The wording of a message and the SMTP conversation that carries it are
// still to be written — and until they are, nothing calls those last two — so a hook run today
// reads its event, works out
// where it would go, says on stderr that it cannot take it there yet, and ends cleanly. A run
// whose required settings are not filled in ends instead on the error naming them, which is the
// one failure this build already reports for real.
package main

import (
	"bytes"
	"encoding/json"
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

// hook is what one event comes to. It holds the judgements that do not depend on how a message
// is worded or carried — the contract it is written to, who drove the write, and whether the
// settings it would be sent on are there — and stops there for now: what to say is not written
// yet, so an event that gets this far leaves a line in amenbo's execution log instead of a
// message in a mailbox.
//
// The settings are read after the actor, not before: an event this plugin would not report is
// no reason to complain that it is unconfigured.
func hook(in input) error {
	if in.Event == "" {
		return nil
	}
	if in.V != contractVersion {
		return fmt.Errorf("payload contract v%d is not the v%d this build reads", in.V, contractVersion)
	}
	if in.Actor != actorAI {
		return nil
	}
	cfg, err := loadConfig(in.Config)
	if err != nil {
		return err
	}
	logf("%s: %s on %d is for %s — no message is sent by this build", pluginName, in.Event, in.ID, strings.Join(cfg.to, ", "))
	return nil
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

This build sends nothing: the wording of a message and the SMTP conversation that carries it
are not written yet, so an event reaches 'amenbo plugin log mail' and stops there.

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
