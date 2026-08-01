package main

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// A project's messages belong together, and the subject cannot be what says so: it changes shape
// with how many events a message carries — what happened on one, how many on several — while
// matching subjects is how most mail clients decide two messages are one conversation. So the
// messages say it in headers instead. The first one from a project is the thread, and every one
// after it names that first message, which is what a client files them all under.
//
// What has to be remembered for that is one string per project, kept beside the lines waiting and
// the events taken in. A run that cannot read it back is not broken: its message begins a thread
// of its own. That costs the mailbox one more thread and loses no notification, which is the right
// way round for a note to self.

// threadFile is what the name of the message that began the project's thread is kept under, in the
// project's own folder.
const threadFile = "thread"

// thread is where one message sits in its project's thread: a name of its own, and the name of the
// message the thread was begun with — empty on the message that begins it.
type thread struct {
	id   string
	root string
}

// threadFor names this message and finds the thread it joins.
func threadFor(s state, from string) thread {
	t := thread{id: messageID(from)}
	if kept := s.lines(threadFile); len(kept) > 0 {
		t.root = kept[0]
	}
	return t
}

// began remembers this message as the one the project's thread hangs off.
//
// It runs after the server has taken the message, never before. A name written down first would,
// on a send that then failed, be answered by every message after it and never itself arrive — and
// a client with nothing to file them under is as likely to scatter them as to group them.
//
// A message that had a thread to join already changes nothing. A name that will not save is
// reported and no more: the next message then begins a thread of its own, which is the same
// harmless outcome as a project reporting for the first time.
func (t thread) began(s state) {
	if t.root != "" || !s.remembers() {
		return
	}
	if err := s.setLines(threadFile, []string{t.id}); err != nil {
		logf("%s: the message this project's thread began with could not be written down, so the next message begins a thread of its own: %v",
			pluginName, err)
	}
}

// headers is what the message says about its thread, ready to be written into one.
//
// In-Reply-To is what a client files a message under, and References is the chain it walks when
// what that names is not in the mailbox. Both carry the one message the thread began with: the
// messages under it are not answers to one another, they are all reports on the same project.
func (t thread) headers() []string {
	own := "Message-ID: " + t.id
	if t.root == "" {
		return []string{own}
	}
	return []string{own, "In-Reply-To: " + t.root, "References: " + t.root}
}

// messageID is this message's own name, which is what a mail client files replies and repeats
// against. The domain is the sender's, since that is the one name in the message that belongs
// to whoever is sending it.
func messageID(from string) string {
	var b [16]byte
	rand.Read(b[:])
	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 && at+1 < len(from) {
		domain = from[at+1:]
	}
	return fmt.Sprintf("<%x@%s>", b, domain)
}
