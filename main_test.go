package main

import (
	"bytes"
	"errors"
	"mime"
	"strings"
	"testing"
)

// captureErr redirects the diagnostics a run writes, and answers with what it wrote.
func captureErr(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	was := errOut
	errOut = &buf
	t.Cleanup(func() { errOut = was })
	return &buf
}

// message is one message a run handed over, kept instead of sent.
type message struct {
	settings settings
	subject  string
	body     string
	thread   thread
}

// lines is what the body says happened, without the project it opens with.
func (m message) lines() []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(m.body), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "amenbo-plugin-mail") {
			out = append(out, line)
		}
	}
	return out
}

// text is the subject as a reader sees it, the encoding a header travels under undone.
func (m message) text(t *testing.T) string {
	t.Helper()
	subject, err := (&mime.WordDecoder{}).DecodeHeader(m.subject)
	if err != nil {
		t.Fatalf("the subject could not be read back: %v", err)
	}
	return subject
}

// holdMessages stands in for the server a message is handed to, and answers with what was handed
// over. A test of what a hook decides is not a test of SMTP, which send_test.go holds to a server
// of its own.
func holdMessages(t *testing.T) *[]message {
	t.Helper()
	return answerSend(t, nil)
}

// refuseMessages is holdMessages with a server that will not take anything.
func refuseMessages(t *testing.T) *[]message {
	t.Helper()
	return answerSend(t, errors.New("the server would not take the message"))
}

func answerSend(t *testing.T, answer error) *[]message {
	t.Helper()
	var sent []message
	was := sendMessage
	sendMessage = func(s settings, subject, body string, t thread) error {
		sent = append(sent, message{settings: s, subject: subject, body: body, thread: t})
		return answer
	}
	t.Cleanup(func() { sendMessage = was })
	return &sent
}

// event is a payload for one AI-driven event, configured well enough to be sent on.
func event(name string) input {
	return input{
		V:     contractVersion,
		Event: name,
		ID:    42,
		Actor: actorAI,
		At:    "2026-08-01T09:00:00Z",
		Config: map[string]any{
			keySMTPHost: "smtp.example.com",
			keySMTPUser: "you@example.com",
		},
	}
}

// reporting is a run set up to report for real: an account to send on, a project to send about,
// and an amenbo with answers. What it leaves out is a base directory — nowhere to write is one
// event to one message, which is the plain case each test says otherwise about for itself.
func reporting(t *testing.T) {
	t.Helper()
	t.Setenv(secretEnv(keySMTPPassword), "app-password")
	t.Setenv(envReach, someReach)
	t.Setenv(envQueueRemaining, "0")
	answerAmenbo(t, everythingAnswers())
}

// holding is reporting with somewhere to write what waits, and answers with the folder it writes
// in so a test can read back what is still waiting there.
func holding(t *testing.T) state {
	t.Helper()
	reporting(t)
	home := t.TempDir()
	t.Setenv(envHome, home)
	return stateAt(home, someReach)
}

func TestHookSendsAChosenEvent(t *testing.T) {
	reporting(t)
	sent := holdMessages(t)

	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one the event earned", len(*sent))
	}
	got := (*sent)[0]
	if !strings.Contains(got.body, "Ship the thing") {
		t.Errorf("the number was not filled in with what it names: %q", got.body)
	}
	if !strings.Contains(got.text(t), "タスクを完了") {
		t.Errorf("subject = %q, want what happened, in the language amenbo is set to", got.text(t))
	}
	if strings.Join(got.settings.to, ",") != "you@example.com" {
		t.Errorf("sent to %v, want the mailbox the settings name", got.settings.to)
	}
}

// A read that will not come back costs the run its exit code, not its message: what was read is
// still sent, and the failure reaches the execution log beside it.
func TestHookSendsWhatItCouldWhenAReadFails(t *testing.T) {
	reporting(t)
	answers := everythingAnswers()
	answers["task show"] = ""
	answerAmenbo(t, answers)
	sent := holdMessages(t)

	if err := hook(event(eventTaskDone)); err == nil {
		t.Fatalf("hook ended cleanly on a read that did not come back")
	}
	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the message the read did not cost the user", len(*sent))
	}
	if !strings.Contains((*sent)[0].body, "amenbo-plugin-mail") {
		t.Errorf("what did come back was thrown away too: %q", (*sent)[0].body)
	}
}

// While amenbo says more is waiting, the line is written down instead of sent — one thing the
// user did is one message, however many events amenbo made of it.
func TestHookHoldsALineWhileMoreIsWaiting(t *testing.T) {
	s := holding(t)
	t.Setenv(envQueueRemaining, "2")
	sent := holdMessages(t)

	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(*sent) != 0 {
		t.Errorf("sent %d message(s) with a burst still coming", len(*sent))
	}
	if got := pending(s); len(got) != 1 {
		t.Errorf("waiting = %v, want the line written down for the run that ends the burst", got)
	}
}

func TestHookSendsTheWholeBurstWhenNothingIsWaiting(t *testing.T) {
	s := holding(t)
	sent := holdMessages(t)

	t.Setenv(envQueueRemaining, "1")
	first := event(eventTaskCreated)
	if err := hook(first); err != nil {
		t.Fatalf("hook: %v", err)
	}
	t.Setenv(envQueueRemaining, "0")
	last := event(eventTaskDone)
	last.At = "2026-08-01T09:00:01Z"
	if err := hook(last); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one the burst became", len(*sent))
	}
	got := (*sent)[0]
	if lines := got.lines(); len(lines) != 2 {
		t.Errorf("body carries %d line(s): %q, want both events", len(lines), got.body)
	}
	if !strings.Contains(got.text(t), "2") {
		t.Errorf("subject = %q, want how many events it carries", got.text(t))
	}
	if waiting := pending(s); len(waiting) != 0 {
		t.Errorf("still waiting: %v, want nothing after the message carried it", waiting)
	}
}

// amenbo does not hand a failed event back, so a message that could not be delivered is one the
// next message has to carry.
func TestHookKeepsTheLinesWhenTheSendFails(t *testing.T) {
	s := holding(t)
	refuseMessages(t)

	if err := hook(event(eventTaskDone)); err == nil {
		t.Fatalf("hook ended cleanly on a message the server would not take")
	}
	if got := pending(s); len(got) != 1 {
		t.Errorf("waiting = %v, want the line the failed send did not lose", got)
	}
}

// The run that ends a burst is often not the run the line in it arrived on — amenbo fires
// task.created and task.assigned together, and the default reports only the first. The message
// still carries one event, so it says what happened rather than how many there were.
func TestHookSaysWhatHappenedWhenAnotherRunCarriesTheLine(t *testing.T) {
	holding(t)
	t.Setenv(envQueueRemaining, "1")
	sent := holdMessages(t)
	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	t.Setenv(envQueueRemaining, "0")
	last := event(eventTaskAssigned)
	last.At = "2026-08-01T09:00:01Z"
	if err := hook(last); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one carrying what was held", len(*sent))
	}
	if got := (*sent)[0].text(t); !strings.Contains(got, "タスクを完了") {
		t.Errorf("subject = %q, want what the one event it carries was", got)
	}
}

// A line held by a build that wrote no event has nothing to say but how many, and saying that is
// better than an update nobody gets.
func TestHookSaysHowManyForALineItCannotDescribe(t *testing.T) {
	s := holding(t)
	if err := s.setLines(pendingFile, []string{"2026-08-01 14:33:41  AI finished " + sampleRef}); err != nil {
		t.Fatalf("setLines: %v", err)
	}
	sent := holdMessages(t)

	last := event(eventTaskAssigned)
	if err := hook(last); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one carrying the waiting line", len(*sent))
	}
	if got := (*sent)[0].text(t); !strings.Contains(got, "1") {
		t.Errorf("subject = %q, want how many it carries", got)
	}
}

// A project is one conversation in the mailbox: the first message begins the thread, and every one
// after it names that same first message rather than the one just before it.
func TestHookGathersAProjectsMessagesIntoOneThread(t *testing.T) {
	holding(t)
	sent := holdMessages(t)

	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}
	later := event(eventTaskDone)
	later.At = "2026-08-01T09:00:01Z"
	if err := hook(later); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 2 {
		t.Fatalf("sent %d message(s), want one per event", len(*sent))
	}
	first, second := (*sent)[0], (*sent)[1]
	if first.thread.root != "" {
		t.Errorf("root = %q, want the first message to begin the thread", first.thread.root)
	}
	if second.thread.root != first.thread.id {
		t.Errorf("root = %q, want the message the thread began with (%q)", second.thread.root, first.thread.id)
	}
}

// A thread is begun by a message that arrived. One the server would not take is remembered by
// nothing, so the next message begins the thread instead of answering a message nobody has.
func TestHookBeginsNoThreadOnAMessageThatDidNotGoOut(t *testing.T) {
	holding(t)
	refuseMessages(t)
	if err := hook(event(eventTaskCreated)); err == nil {
		t.Fatalf("hook ended cleanly on a message the server would not take")
	}
	sent := holdMessages(t)

	later := event(eventTaskDone)
	later.At = "2026-08-01T09:00:01Z"
	if err := hook(later); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one carrying both", len(*sent))
	}
	if got := (*sent)[0].thread.root; got != "" {
		t.Errorf("root = %q, want a thread begun by a message that arrived", got)
	}
}

func TestHookCarriesWhatAFailedSendLeftBehind(t *testing.T) {
	holding(t)
	refuseMessages(t)
	if err := hook(event(eventTaskCreated)); err == nil {
		t.Fatalf("hook ended cleanly on a message the server would not take")
	}
	sent := holdMessages(t)

	later := event(eventTaskDone)
	later.At = "2026-08-01T09:00:01Z"
	if err := hook(later); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one carrying both", len(*sent))
	}
	if lines := (*sent)[0].lines(); len(lines) != 2 {
		t.Errorf("body carries %d line(s): %q, want the line held back beside this one", len(lines), (*sent)[0].body)
	}
}

// A redelivery adds no line — it was counted the first time — but it is still a run, and one that
// ends a burst carries what an earlier run held and did not get to send.
func TestHookSendsWhatIsWaitingOnARedelivery(t *testing.T) {
	holding(t)
	t.Setenv(envQueueRemaining, "1")
	sent := holdMessages(t)
	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	t.Setenv(envQueueRemaining, "0")
	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one the waiting line earned", len(*sent))
	}
	if lines := (*sent)[0].lines(); len(lines) != 1 {
		t.Errorf("body carries %d line(s): %q, want the event once", len(lines), (*sent)[0].body)
	}
}

// A second copy of an event with nothing waiting behind it is the whole reason the record of what
// has been taken in is kept: nothing is sent, and nothing is asked of amenbo either.
func TestHookSaysNothingAboutARedeliveryWithNothingWaiting(t *testing.T) {
	holding(t)
	sent := holdMessages(t)
	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Errorf("sent %d message(s), want the one the first copy earned", len(*sent))
	}
}

// The run that ends a burst is often one this plugin says nothing of its own about — amenbo's
// default reports four of the eleven events, and one it skips is as likely as any to be last. The
// lines held behind it leave on it all the same, or they wait for an event that may be days away.
func TestHookSendsWhatIsWaitingOnAnEventItSaysNothingAbout(t *testing.T) {
	holding(t)
	t.Setenv(envQueueRemaining, "1")
	sent := holdMessages(t)
	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	t.Setenv(envQueueRemaining, "0")
	last := event(eventTaskAssigned)
	last.At = "2026-08-01T09:00:01Z"
	if err := hook(last); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one carrying what was held", len(*sent))
	}
	if lines := (*sent)[0].lines(); len(lines) != 1 {
		t.Errorf("body carries %d line(s): %q, want the reported event, and only it", len(lines), (*sent)[0].body)
	}
}

// The same holds for a write the user drove themselves: it says nothing, and carries everything.
func TestHookSendsWhatIsWaitingOnTheUsersOwnWrite(t *testing.T) {
	holding(t)
	t.Setenv(envQueueRemaining, "1")
	sent := holdMessages(t)
	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	t.Setenv(envQueueRemaining, "0")
	last := event(eventTaskCreated)
	last.Actor = "human"
	if err := hook(last); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the one carrying what was held", len(*sent))
	}
	if lines := (*sent)[0].lines(); len(lines) != 1 {
		t.Errorf("body carries %d line(s): %q, want the AI's write, and not the user's", len(lines), (*sent)[0].body)
	}
}

// Mid-burst, a run with nothing of its own to say leaves what is waiting where it is: the run
// amenbo ends the burst with is the one that carries it, and sending early would only split one
// message into two.
func TestHookHoldsOnAnEventItSaysNothingAboutMidBurst(t *testing.T) {
	s := holding(t)
	t.Setenv(envQueueRemaining, "2")
	sent := holdMessages(t)
	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	t.Setenv(envQueueRemaining, "1")
	if err := hook(event(eventTaskAssigned)); err != nil {
		t.Fatalf("hook: %v", err)
	}

	if len(*sent) != 0 {
		t.Errorf("sent %d message(s) with a burst still coming", len(*sent))
	}
	if got := pending(s); len(got) != 1 {
		t.Errorf("waiting = %v, want the line still waiting for the end of the burst", got)
	}
}

// With nowhere to write there is nothing to hold a line in, so the burst is given up on rather
// than the event: one event, one message.
func TestHookSendsOnItsOwnWithNowhereToWrite(t *testing.T) {
	reporting(t)
	t.Setenv(envQueueRemaining, "3")
	sent := holdMessages(t)

	if err := hook(event(eventTaskDone)); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(*sent) != 1 {
		t.Fatalf("sent %d message(s), want the event on its own", len(*sent))
	}
	if lines := (*sent)[0].lines(); len(lines) != 1 {
		t.Errorf("body carries %d line(s): %q, want this event alone", len(lines), (*sent)[0].body)
	}
}

func TestHookSendsNothingAboutAnEventThatWasNotChosen(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "app-password")
	sent := holdMessages(t)
	out := captureErr(t)

	if err := hook(event(eventCommentAdded)); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(*sent) != 0 || out.Len() != 0 {
		t.Errorf("sent %d message(s) and said %q about an event nobody asked to hear about", len(*sent), out.String())
	}
}

// The settings are read after the event has earned a message, so an event this plugin was never
// going to report cannot be the one that complains the plugin is unconfigured.
func TestHookDoesNotComplainAboutSettingsForAnEventItSkips(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "")
	out := captureErr(t)

	in := event(eventCommentAdded)
	in.Config = nil

	if err := hook(in); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q about an event nobody asked to hear about", out.String())
	}
}

// A write the user drove is one they were present for, and it is skipped before anything is
// asked of the configuration.
func TestHookSaysNothingAboutTheUsersOwnWrites(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "")
	out := captureErr(t)

	in := event(eventTaskDone)
	in.Actor = "human"
	in.Config = nil

	if err := hook(in); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q about a write the user drove themselves", out.String())
	}
}

func TestHookRefusesAContractItDoesNotRead(t *testing.T) {
	in := event(eventTaskDone)
	in.V = contractVersion + 1

	if err := hook(in); err == nil {
		t.Fatalf("hook read a payload written to a contract it does not know")
	}
}

func TestHookDoesNothingWhenNoEventFired(t *testing.T) {
	out := captureErr(t)

	if err := hook(input{}); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q with no event to report", out.String())
	}
}

// Choosing none leaves the plugin switched on and silent — including on the events it would
// otherwise have reported by default. amenbo delivers that choice as an empty list.
func TestHookReportsNothingWhenNoEventsWereChosen(t *testing.T) {
	t.Setenv(secretEnv(keySMTPPassword), "app-password")
	out := captureErr(t)

	in := event(eventTaskDone)
	in.Config[keyEvents] = ""

	if err := hook(in); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("said %q with nothing chosen to report", out.String())
	}
}
