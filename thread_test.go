package main

import (
	"strings"
	"testing"
)

func TestMessageIDIsThisMessagesOwnName(t *testing.T) {
	one, two := messageID("amenbo@example.test"), messageID("amenbo@example.test")

	if one == two {
		t.Error("two messages were given the same name")
	}
	if !strings.HasSuffix(one, "@example.test>") || !strings.HasPrefix(one, "<") {
		t.Errorf("messageID = %q, want it named under the sender's domain", one)
	}
	if got := messageID("nobody"); !strings.HasSuffix(got, "@localhost>") {
		t.Errorf("messageID = %q, want a name even without a domain to use", got)
	}
}

func TestTheFirstMessageBeginsTheThreadAndTheNextJoinsIt(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")

	first := threadFor(s, "amenbo@example.test")
	if first.root != "" {
		t.Errorf("root = %q, want nothing to join on the message that begins the thread", first.root)
	}
	first.began(s)

	second := threadFor(s, "amenbo@example.test")
	if second.root != first.id {
		t.Errorf("root = %q, want the message the thread began with (%q)", second.root, first.id)
	}
	if second.id == first.id {
		t.Error("the second message was given the first one's name")
	}
}

// The thread hangs off the first message and stays there: a later one that took its own name as
// the root would leave every message answering the one before it, which is a chain and not a
// conversation in the clients that only look at the parent.
func TestTheThreadKeepsTheMessageItBeganWith(t *testing.T) {
	s := stateAt(t.TempDir(), "project-a")
	first := threadFor(s, "amenbo@example.test")
	first.began(s)

	second := threadFor(s, "amenbo@example.test")
	second.began(s)

	if got := threadFor(s, "amenbo@example.test").root; got != first.id {
		t.Errorf("root = %q, want it still the message the thread began with (%q)", got, first.id)
	}
}

func TestOneProjectDoesNotJoinAnothersThread(t *testing.T) {
	home := t.TempDir()
	one, two := stateAt(home, "project-a"), stateAt(home, "project-b")
	threadFor(one, "amenbo@example.test").began(one)

	if got := threadFor(two, "amenbo@example.test").root; got != "" {
		t.Errorf("root = %q, want a project's own thread and not the other's", got)
	}
}

// With nowhere to write, every message begins a thread of its own. That is a mailbox holding one
// thread per message, which is what this plugin did before it remembered anything — and it is a
// far smaller loss than a message that is not sent.
func TestAMessageIsStillNamedWithNowhereToRememberTheThread(t *testing.T) {
	s := stateAt("", "project-a")

	first := threadFor(s, "amenbo@example.test")
	first.began(s)
	second := threadFor(s, "amenbo@example.test")

	if first.id == "" || second.id == "" {
		t.Error("a message went out without a name of its own")
	}
	if second.root != "" {
		t.Errorf("root = %q, want nothing remembered where there is nowhere to write", second.root)
	}
}

func TestHeadersNameTheThreadOnEveryMessageButTheFirst(t *testing.T) {
	first := thread{id: "<one@example.test>"}
	if got, want := strings.Join(first.headers(), "\n"), "Message-ID: <one@example.test>"; got != want {
		t.Errorf("headers = %q, want %q — the message that begins a thread answers nothing", got, want)
	}

	next := thread{id: "<two@example.test>", root: "<one@example.test>"}
	got := strings.Join(next.headers(), "\n")
	for _, want := range []string{
		"Message-ID: <two@example.test>",
		"In-Reply-To: <one@example.test>",
		"References: <one@example.test>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("headers are missing %q:\n%s", want, got)
		}
	}
}
