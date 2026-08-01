package main

import (
	"errors"
	"strings"
	"testing"
)

// answerAmenbo stands in for the amenbo on PATH, answering each read by the first two arguments
// of the command it was asked for ("task show", "config"). It records what it was asked, so a
// test can say what should never have been asked at all.
func answerAmenbo(t *testing.T, answers map[string]string) *[]string {
	t.Helper()
	var asked []string
	was := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) {
		call := strings.Join(args, " ")
		asked = append(asked, call)
		for prefix, answer := range answers {
			if strings.HasPrefix(call, prefix) {
				if answer == "" {
					return nil, errors.New(prefix + ": refused")
				}
				return []byte(answer), nil
			}
		}
		return nil, errors.New(call + ": nothing to answer with")
	}
	t.Cleanup(func() { runAmenbo = was })
	return &asked
}

// The refs a test needs are written in two pieces on purpose. A whole one is a number that means
// something only inside the store that issued it, and the commit hook stops those leaving the
// repository — which is a guard worth keeping sharp, so it is not switched off for a fixture.
const refNamespace = "AMB-"

const (
	someReach      = refNamespace + "P-1"
	taskAnswer     = `{"id":42,"ref":"` + refNamespace + `T-42","title":"Ship the thing","status":"done"}`
	decisionAnswer = `{"id":7,"ref":"` + refNamespace + `D-7","title":"Send over SMTP"}`
	projectAnswer  = `{"id":1,"name":"amenbo-plugin-mail"}`
	configAnswer   = `{"settings":{"language":"ja","ai_display_name":"Sakura","human_display_name":"Yamada"}}`
)

func everythingAnswers() map[string]string {
	return map[string]string{
		"task show":     taskAnswer,
		"decision show": decisionAnswer,
		"project show":  projectAnswer,
		"config":        configAnswer,
	}
}

func TestLookupReadsBackWhatThePayloadDoesNotCarry(t *testing.T) {
	t.Setenv(envReach, someReach)
	answerAmenbo(t, everythingAnswers())

	got, err := lookup(input{Event: eventTaskDone, ID: 42})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.title != "Ship the thing" {
		t.Errorf("title = %q, want the one read back", got.title)
	}
	if !strings.HasSuffix(got.ref, "-42") {
		t.Errorf("ref = %q, want the one amenbo renders", got.ref)
	}
	if got.project != "amenbo-plugin-mail" {
		t.Errorf("project = %q, want the one the event came from", got.project)
	}
	if got.language != "ja" || got.aiName != "Sakura" || got.userName != "Yamada" {
		t.Errorf("preferences = %q/%q/%q, want what amenbo is set to", got.language, got.aiName, got.userName)
	}
}

// A decision is a different kind of row in a different number space, so it is asked about as one.
func TestLookupReadsADecisionBackAsADecision(t *testing.T) {
	t.Setenv(envReach, someReach)
	asked := answerAmenbo(t, everythingAnswers())

	got, err := lookup(input{Event: eventDecisionAccepted, ID: 7})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.title != "Send over SMTP" {
		t.Errorf("title = %q, want the decision's", got.title)
	}
	if !strings.Contains(strings.Join(*asked, "|"), "decision show 7") {
		t.Errorf("asked %v, want the decision read back", *asked)
	}
}

// What a person wants named on a comment is the task it was written on, not the comment.
func TestLookupNamesTheTaskACommentWasWrittenOn(t *testing.T) {
	t.Setenv(envReach, someReach)
	asked := answerAmenbo(t, everythingAnswers())

	parent := int64(42)
	got, err := lookup(input{Event: eventCommentAdded, ID: 5, Parent: &parent})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.title != "Ship the thing" {
		t.Errorf("title = %q, want the parent task's", got.title)
	}
	if !strings.Contains(strings.Join(*asked, "|"), "task show 42") {
		t.Errorf("asked %v, want the parent read back rather than the comment", *asked)
	}
}

// An older amenbo carries no parent for a comment. The comment's own number belongs to a
// different space than a task's, so asking with it would answer with some unrelated task's
// title — worse than answering with none.
func TestLookupWillNotGuessTheTaskWhenNoParentIsNamed(t *testing.T) {
	t.Setenv(envReach, someReach)
	asked := answerAmenbo(t, everythingAnswers())

	got, err := lookup(input{Event: eventCommentAdded, ID: 5})
	if err == nil {
		t.Fatalf("lookup did not say the parent was missing")
	}
	if got.title != "" {
		t.Errorf("title = %q, want none rather than a guess", got.title)
	}
	if strings.Contains(strings.Join(*asked, "|"), "task show 5") {
		t.Errorf("asked %v — that number names a comment, not a task", *asked)
	}
	if got.project == "" || got.language == "" {
		t.Errorf("the rest was dropped along with it: %+v", got)
	}
}

// A deleted task cannot be read back, so the row travels in the payload instead.
func TestLookupTakesADeletedTaskFromThePayload(t *testing.T) {
	t.Setenv(envReach, someReach)
	asked := answerAmenbo(t, everythingAnswers())

	in := input{Event: eventTaskDeleted, ID: 42, Record: map[string]any{"title": "Ship the thing"}}
	got, err := lookup(in)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.title != "Ship the thing" {
		t.Errorf("title = %q, want the one carried in the payload", got.title)
	}
	if !strings.HasSuffix(got.ref, "-42") {
		t.Errorf("ref = %q, want it named by its number", got.ref)
	}
	if strings.Contains(strings.Join(*asked, "|"), "task show") {
		t.Errorf("asked %v about a task that no longer exists", *asked)
	}
}

// A message that says what happened to a number is worth more than no message, so what could not
// be read back is left empty and the rest is kept — and the failure is still answered with, so
// the run ends non-zero and the log says which read did not work.
func TestLookupKeepsWhatItCouldReadWhenOneReadFails(t *testing.T) {
	t.Setenv(envReach, someReach)
	answers := everythingAnswers()
	answers["task show"] = ""
	answerAmenbo(t, answers)

	got, err := lookup(input{Event: eventTaskDone, ID: 42})
	if err == nil {
		t.Fatalf("lookup said nothing about the read that failed")
	}
	if got.title != "" || got.ref != "" {
		t.Errorf("record = %+v, want it empty", got.record)
	}
	if got.project != "amenbo-plugin-mail" || got.language != "ja" {
		t.Errorf("the reads that worked were thrown away too: %+v", got)
	}
}

// A run nobody dispatched belongs to no project. That is a hand run, not a failure: a message
// written from it simply opens with the first event instead of a heading.
func TestLookupTreatsNoProjectAsAHandRunRatherThanAFailure(t *testing.T) {
	t.Setenv(envReach, "")
	asked := answerAmenbo(t, everythingAnswers())

	got, err := lookup(input{Event: eventTaskDone, ID: 42})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.project != "" {
		t.Errorf("project = %q, want none", got.project)
	}
	if strings.Contains(strings.Join(*asked, "|"), "project show") {
		t.Errorf("asked %v about a project the run does not belong to", *asked)
	}
}

// A refusal reaches the execution log as its code, where one line per run is the format.
func TestExecAmenboReportsARefusalByItsCode(t *testing.T) {
	got := refusalCode([]byte(`{"error":{"code":"out_of_reach","message":"a long sentence explaining"}}`))
	if got != "out_of_reach" {
		t.Errorf("refusalCode = %q, want the code", got)
	}
	if got := refusalCode([]byte("not a document at all")); got != "" {
		t.Errorf("refusalCode = %q, want nothing for a failure amenbo had no code for", got)
	}
}

func TestShowRecordReportsAnAnswerItCannotRead(t *testing.T) {
	answerAmenbo(t, map[string]string{"task show": "not a record"})

	if _, err := showRecord("task", 42); err == nil {
		t.Fatalf("showRecord read a record out of something that is not one")
	}
}
