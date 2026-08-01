package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A run of this plugin is one process, started for one event and gone again, so whatever it
// has to still know next time — which events it has already taken in, the lines not sent yet,
// the message that began a project's thread — has to be written down between runs.
//
// Where it is written is per project. amenbo names the base directory in the environment and
// the project the event came from beside it, and the two together give one folder per project
// under this plugin's own. Keeping them apart is not tidiness: lines held back for one project
// are that project's work, and one shared pile would post them to whichever mailbox the next
// project's settings happen to name.

const (
	// envHome names the amenbo base directory this run belongs to.
	envHome = "AMENBO_HOME"
	// envReach names the project the event came from, as `AMB-P-<n>`. A run nobody
	// dispatched — a hand run from a terminal — comes from no project and carries no value.
	envReach = "AMENBO_PLUGIN_REACH"
)

// noProject is the folder a run belonging to no project keeps things under. Hand runs are for
// trying the plugin out, and a folder of their own keeps what they leave behind from being
// read back as some real project's.
const noProject = "no-project"

// state is the folder this plugin keeps what it remembers in, or nothing at all when amenbo
// named no base directory to keep it under. Nothing kept here is needed for a message to be
// sent, so a caller finding that it remembers nothing is meant to carry on without it — one
// event to one message — rather than to fail.
//
// What is kept is lines, and a line is a line: an entry carrying a newline of its own would be
// read back as two, so callers write entries in a form that holds none (JSON escapes them).
type state struct {
	dir string
}

// stateFromEnv locates the folder from what amenbo put in the environment.
func stateFromEnv() state {
	return stateAt(os.Getenv(envHome), os.Getenv(envReach))
}

// stateAt is stateFromEnv with those two values handed in.
func stateAt(home, reach string) state {
	if home == "" {
		return state{}
	}
	return state{dir: filepath.Join(home, "plugins", pluginName, folderFor(reach))}
}

// remembers reports whether there is anywhere to keep anything. False is a run told of no base
// directory, which is a run that has to get through on the event in front of it alone.
func (s state) remembers() bool {
	return s.dir != ""
}

// folderFor turns a reach into the single path element it is kept under. Everything outside
// letters, digits, `-` and `_` becomes `_`, because a reach is a value out of the environment
// and a value out of the environment is not a name to be trusted with a path: a separator or a
// `..` in it would write outside the folder this is supposed to stay inside. A reach left with
// nothing by that, the empty one included, is a run with no project.
func folderFor(reach string) string {
	kept := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '_'
	}, reach)
	if kept == "" {
		return noProject
	}
	return kept
}

// lines reads back what was last kept under name, one entry per line.
//
// Everything that can go wrong answers the same way, with nothing: a file never written, a
// folder not there, a read that fails. What is here is a note to self, and a note that cannot
// be read is a note this run does not have — exactly where it stands the first time it runs.
// Reporting it would turn a message that can still be sent into a run that failed.
func (s state) lines(name string) []string {
	if !s.remembers() {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSuffix(line, "\r"); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// setLines replaces what is kept under name with these lines — all of them, or none.
//
// The write goes to a temporary file beside the real one and is renamed over it. The process
// can end at any moment: amenbo starts it for an event and nobody is waiting on the answer.
// Written straight to the file, a run cut short would leave half a line behind, and half a
// line is worse than none — it is read back next time as a whole one.
func (s state) setLines(name string, lines []string) error {
	if !s.remembers() {
		return fmt.Errorf("no %s is set, so there is nowhere to keep %s", envHome, name)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	var body strings.Builder
	for _, line := range lines {
		body.WriteString(line)
		body.WriteString("\n")
	}
	tmp, err := os.CreateTemp(s.dir, name+".*")
	if err != nil {
		return err
	}
	// Once the rename has carried the file off this removes nothing; it is here for every
	// path that does not get that far, so a run that fails leaves no litter behind it.
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(body.String()); err != nil {
		tmp.Close()
		return err
	}
	// Reaching the disk is the point of writing this down: the run that reads it back is a
	// different process, and may be one started after the machine came back up.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(s.dir, name))
}
