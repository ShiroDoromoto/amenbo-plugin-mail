package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// A payload carries a number and never the row it names, so everything a message is written from
// — what the record is called, which project it belongs to, what language to write in, and what
// to call the people involved — is read back out of amenbo here.
//
// It is read by running amenbo, the same way a person would. What makes that work is already in
// the environment amenbo started this run with, and none of it is this plugin's to arrange:
// AMENBO_HOME names the store, and AMENBO_PLUGIN_REACH names the project — which is the window
// the read is allowed through, and the reason a read stays inside the project the event came
// from. So the environment is passed on untouched and nothing is added to it: a read that
// reaches past that window is refused as out_of_reach, and being refused is the point.

// amenboBinary is the command that answers about the store. It is looked up on PATH rather than
// named absolutely: amenbo started this run, so the PATH it was found on is the one inherited
// here.
const amenboBinary = "amenbo"

// runAmenbo runs one read and answers with what it wrote to stdout. It is a variable so a test
// can answer for amenbo without one being installed.
var runAmenbo = execAmenbo

// execAmenbo runs amenbo and reads its answer.
//
// A refusal is reported by its code — `out_of_reach`, `not_found_task` — rather than by the
// sentence explaining it, because this ends up in the execution log where one line per run is
// the format. The arguments are named alongside it so the line says which read failed; they are
// numbers and refs, never a setting, so nothing secret travels with them.
func execAmenbo(args ...string) ([]byte, error) {
	cmd := exec.Command(amenboBinary, append(args, "--json", "--actor", actorAI)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	if code := refusalCode(stderr.Bytes()); code != "" {
		return nil, fmt.Errorf("%s %s: %s", amenboBinary, strings.Join(args, " "), code)
	}
	return nil, fmt.Errorf("%s %s: %v", amenboBinary, strings.Join(args, " "), err)
}

// refusalCode picks the code out of the document amenbo writes to stderr when it refuses. An
// empty answer is one that did not fail that way — the binary was missing, or it died on
// something it had no code for.
func refusalCode(stderr []byte) string {
	var doc struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &doc); err != nil {
		return ""
	}
	return doc.Error.Code
}

// record is the row an event happened to, named as a person knows it.
type record struct {
	// ref is the namespaced number, as amenbo renders it. Empty when it could not be worked out.
	ref string
	// title is what the record is called. Empty when it could not be read back — a message still
	// goes out, saying what happened to a number.
	title string
}

// details is everything a message needs that the payload does not carry. Every field stands on
// its own: one that could not be read back is empty, and what was read is still used.
type details struct {
	record
	// project is the name of the project the event came from, for the heading a message opens
	// with. Empty on a run belonging to no project, which is not a failure — a hand run has none.
	project string
	// language is the code amenbo is set to, e.g. "ja", which decides which wording a message is
	// written in.
	language string
	// aiName and userName are what the two of them are called, as amenbo displays them.
	aiName   string
	userName string
}

// lookup reads back everything an event does not carry.
//
// What could not be read is left empty and the rest is kept: a title that will not come back is
// a message that says what happened to a number, which is worth more than no message at all. The
// failures are joined and answered with, so the run still ends non-zero and the execution log
// says which read did not work.
func lookup(in input) (details, error) {
	d, err := surroundings()
	rec, recErr := lookupRecord(in)
	d.record = rec
	return d, errors.Join(recErr, err)
}

// surroundings is what a message needs about where it comes from: the project it is about, and the
// language and names it is written in. It asks nothing about a record, which is what a run carrying
// only what earlier runs held needs — its own event is not named in the message, so reading it back
// would be a question asked for nothing and an exit code spent on the answer.
func surroundings() (details, error) {
	var errs []error

	d := details{}
	project, err := lookupProject(os.Getenv(envReach))
	if err != nil {
		errs = append(errs, err)
	}
	d.project = project

	prefs, err := lookupPreferences()
	if err != nil {
		errs = append(errs, err)
	}
	d.language, d.aiName, d.userName = prefs.language, prefs.aiName, prefs.userName

	return d, errors.Join(errs...)
}

// lookupRecord answers with the row the event happened to.
//
// Which row that is depends on the event. A deletion has none left to read, so the payload
// carries the row itself and it is taken from there. A comment is not what a person would want
// named — the task it was written on is — so the parent is read back instead.
func lookupRecord(in input) (record, error) {
	switch in.Event {
	case eventTaskDeleted:
		return deletedRecord(in), nil
	case eventDecisionAccepted, eventDecisionRejected:
		return showRecord("decision", in.ID)
	case eventCommentAdded, eventCommentRemoved:
		if in.Parent == nil {
			return record{}, fmt.Errorf("the task %s %d was written on is not named in the payload", in.Event, in.ID)
		}
		return showRecord("task", *in.Parent)
	default:
		return showRecord("task", in.ID)
	}
}

// refPrefixTask is how amenbo renders a task's number. A ref is normally taken from amenbo's own
// answer rather than assembled here; this is for the one record that cannot be read back.
const refPrefixTask = "AMB-T-"

// deletedRecord takes the row out of the payload, which is where a deleted one travels: there is
// nothing left in the store to ask about, so the event carries the row in its place.
func deletedRecord(in input) record {
	rec := record{ref: fmt.Sprintf("%s%d", refPrefixTask, in.ID)}
	if title, ok := in.Record["title"].(string); ok {
		rec.title = title
	}
	return rec
}

// showRecord reads one row back by its number. The ref comes from amenbo's own answer rather
// than being assembled from the number, so how a ref is written stays amenbo's to decide.
func showRecord(kind string, id int64) (record, error) {
	out, err := runAmenbo(kind, "show", fmt.Sprint(id))
	if err != nil {
		return record{}, err
	}
	var doc struct {
		Ref   string `json:"ref"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return record{}, fmt.Errorf("%s show %d answered with something that is not a record: %v", kind, id, err)
	}
	return record{ref: doc.Ref, title: doc.Title}, nil
}

// lookupProject answers with the name of the project an event came from, for the heading a
// message opens with.
//
// A run belonging to no project is a hand run, not a failure: it is answered with no name and no
// error, and a message written from it simply opens with the first event instead.
func lookupProject(reach string) (string, error) {
	if reach == "" {
		return "", nil
	}
	out, err := runAmenbo("project", "show", reach)
	if err != nil {
		return "", err
	}
	var doc struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return "", fmt.Errorf("project show %s answered with something that is not a project: %v", reach, err)
	}
	return doc.Name, nil
}

// preferences is what the user has already told amenbo, and so what this plugin must not ask
// them again: the language a message is written in, and what the two of them are called.
type preferences struct {
	language string
	aiName   string
	userName string
}

// lookupPreferences reads those out of amenbo's configuration. The displayed names are taken
// rather than the raw ones, so a user who has not named their AI gets what amenbo calls it on
// screen instead of an empty space in a sentence.
func lookupPreferences() (preferences, error) {
	out, err := runAmenbo("config")
	if err != nil {
		return preferences{}, err
	}
	var doc struct {
		Settings struct {
			Language         string `json:"language"`
			AIDisplayName    string `json:"ai_display_name"`
			HumanDisplayName string `json:"human_display_name"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return preferences{}, fmt.Errorf("config answered with something that is not a configuration: %v", err)
	}
	return preferences{
		language: doc.Settings.Language,
		aiName:   doc.Settings.AIDisplayName,
		userName: doc.Settings.HumanDisplayName,
	}, nil
}
