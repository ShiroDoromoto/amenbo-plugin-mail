package main

import (
	"mime"
	"strings"
	"testing"
	"unicode/utf8"
)

// decoded reads an encoded word back, so a test can check what a mail client would show.
func decoded(t *testing.T, subject string) string {
	t.Helper()
	got, err := new(mime.WordDecoder).DecodeHeader(subject)
	if err != nil {
		t.Fatalf("DecodeHeader(%q): %v", subject, err)
	}
	return got
}

func TestEverySubjectLanguageIsOneTheSentencesHaveToo(t *testing.T) {
	for _, w := range wordings {
		sw, ok := subjectWords[w.language]
		if !ok {
			t.Errorf("%s writes its lines in its own language and its subject in English", w.language)
			continue
		}
		for _, event := range reportableEvents {
			if sw.events[event] == "" {
				t.Errorf("%s has no subject for %s", w.language, event)
			}
		}
		if !strings.Contains(sw.events[eventTaskStatusChanged], "{status}") {
			t.Errorf("%s says %q for a status change, which never says which status",
				w.language, sw.events[eventTaskStatusChanged])
		}
		if !strings.Contains(sw.many, "{n}") {
			t.Errorf("%s says %q for a burst, which never says how many", w.language, sw.many)
		}
	}
	if len(subjectWords) != len(wordings) {
		t.Errorf("%d subject languages against %d written ones", len(subjectWords), len(wordings))
	}
}

func TestSubjectForOneSaysWhatHappenedAndToWhat(t *testing.T) {
	for _, tc := range []struct{ language, want string }{
		{"ja", "[amenbo-plugin-mail] タスクを完了 " + sampleRef},
		{"en", "[amenbo-plugin-mail] Task finished " + sampleRef},
	} {
		t.Run(tc.language, func(t *testing.T) {
			d := spoken(tc.language)
			d.project = "amenbo-plugin-mail"

			got := decoded(t, subjectForOne(entryFor(anEventAt(eventTaskDone, "2026-08-01T05:51:02Z"), d), d))

			if got != tc.want {
				t.Errorf("subjectForOne = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSubjectForOneCallsAStatusWhatAmenboCallsIt(t *testing.T) {
	in := anEventAt(eventTaskStatusChanged, "2026-08-01T05:33:10Z")
	in.New = statusInProgress
	d := spoken("ja")
	d.project = "amenbo"

	if got, want := decoded(t, subjectForOne(entryFor(in, d), d)), "[amenbo] タスクを進行中に変更 "+sampleRef; got != want {
		t.Errorf("subjectForOne = %q, want %q", got, want)
	}
}

func TestSubjectForManySaysHowMany(t *testing.T) {
	for _, tc := range []struct {
		language, want string
	}{
		{"ja", "[amenbo] 更新 3件"},
		{"en", "[amenbo] 3 updates"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			d := spoken(tc.language)
			d.project = "amenbo"

			if got := decoded(t, subjectForMany(d, 3)); got != tc.want {
				t.Errorf("subjectForMany = %q, want %q", got, tc.want)
			}
		})
	}
}

// A message carrying one event is not always one this can say in full — a burst ending on an event
// nobody asked to hear about is sent by a run that never learned what its own event named — so the
// count is written the way the language writes one of something.
func TestSubjectForManyCountsOneAsTheLanguageDoes(t *testing.T) {
	for _, tc := range []struct {
		language, want string
	}{
		{"en", "[amenbo] 1 update"},
		{"de", "[amenbo] 1 Änderung"},
		{"ja", "[amenbo] 更新 1件"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			d := spoken(tc.language)
			d.project = "amenbo"

			if got := decoded(t, subjectForMany(d, 1)); got != tc.want {
				t.Errorf("subjectForMany = %q, want %q", got, tc.want)
			}
		})
	}
}

// A singular is only ever the plural in another form, so one that says nothing about how many
// would leave a reader with a subject that does not count at all.
func TestEverySingularStillSaysHowMany(t *testing.T) {
	for language, sw := range subjectWords {
		if sw.one != "" && !strings.Contains(sw.one, "{n}") {
			t.Errorf("%s says %q for a single event, which never says how many", language, sw.one)
		}
	}
}

func TestSubjectNamesTheSenderWhenTheProjectCouldNotBeRead(t *testing.T) {
	for _, project := range []string{"", "   "} {
		if got := subjectOf(project, "Task finished "+sampleRef); !strings.HasPrefix(got, "["+fallbackProject+"]") {
			t.Errorf("subjectOf(%q, …) = %q, want it to fall back to %q", project, got, fallbackProject)
		}
	}
}

func TestSubjectIsCountedInCharactersAndCutInTheProjectAlone(t *testing.T) {
	what := "Task finished " + sampleRef
	// A project name long enough that only cutting it can make the subject fit.
	long := strings.Repeat("なが", 40)

	got := subjectOf(long, what)

	if n := utf8.RuneCountInString(got); n != subjectLimit {
		t.Errorf("subjectOf ran to %d characters, want exactly %d: %q", n, subjectLimit, got)
	}
	if !strings.HasSuffix(got, ellipsis+"] "+what) {
		t.Errorf("subjectOf = %q, want the project cut and %q kept whole", got, what)
	}
}

func TestSubjectThatAlreadyFitsIsLeftAlone(t *testing.T) {
	what := "Task finished " + sampleRef
	// A project name that takes the subject to exactly the limit.
	room := subjectLimit - utf8.RuneCountInString("[] "+what)
	project := strings.Repeat("p", room)

	got := subjectOf(project, what)

	if want := "[" + project + "] " + what; got != want {
		t.Errorf("subjectOf = %q, want %q", got, want)
	}
	if utf8.RuneCountInString(got) != subjectLimit {
		t.Errorf("the case being tested is not the boundary: %d characters", utf8.RuneCountInString(got))
	}
}

func TestSubjectKeepsWhatHappenedEvenWithNoRoomForTheProject(t *testing.T) {
	what := "Task finished " + strings.Repeat("x", subjectLimit)

	got := subjectOf("a-long-project-name", what)

	if !strings.HasSuffix(got, "] "+what) {
		t.Errorf("subjectOf = %q, want what happened kept whole", got)
	}
	if !strings.HasPrefix(got, "["+ellipsis+"]") {
		t.Errorf("subjectOf = %q, want the project cut away to nothing rather than what happened", got)
	}
}

func TestSubjectTravelsAsAnEncodedWordOnlyWhenItHasTo(t *testing.T) {
	plain := headerReady("[amenbo] Task finished")
	if plain != "[amenbo] Task finished" {
		t.Errorf("headerReady = %q, want an ASCII subject left as it is", plain)
	}

	encoded := headerReady("[amenbo] タスクを完了")
	if !strings.HasPrefix(encoded, "=?UTF-8?") {
		t.Errorf("headerReady = %q, want an encoded word", encoded)
	}
	for _, r := range encoded {
		if r > 127 {
			t.Errorf("headerReady = %q, which is not a header any server has to accept", encoded)
			break
		}
	}
	if got := decoded(t, encoded); got != "[amenbo] タスクを完了" {
		t.Errorf("the encoded word reads back as %q", got)
	}
}

func TestEveryLanguageWritesASubjectForEveryEvent(t *testing.T) {
	for _, w := range wordings {
		d := spoken(w.language)
		d.project = "amenbo"
		for _, event := range reportableEvents {
			in := anEventAt(event, "2026-08-01T05:32:05Z")
			in.New = statusInProgress

			one := decoded(t, subjectForOne(entryFor(in, d), d))
			if strings.Contains(one, "{") {
				t.Errorf("%s / %s left something unfilled: %q", w.language, event, one)
			}
			if !strings.Contains(one, sampleRef) {
				t.Errorf("%s / %s = %q, want the record named", w.language, event, one)
			}
		}
		if many := decoded(t, subjectForMany(d, 7)); !strings.Contains(many, "7") {
			t.Errorf("%s = %q, want the count said", w.language, many)
		}
	}
}

func TestSubjectForAnEventNoLanguageHasAWordFor(t *testing.T) {
	d := spoken("ja")
	d.project = "amenbo"

	got := decoded(t, subjectForOne(entryFor(anEventAt("task.hibernated", "2026-08-01T05:32:05Z"), d), d))

	if !strings.Contains(got, "task.hibernated") {
		t.Errorf("subjectForOne = %q, want it to still say what happened", got)
	}
}
