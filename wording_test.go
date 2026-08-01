package main

import (
	"strings"
	"testing"
	"time"
)

// sampleRef is the reference the tests write about. It is assembled rather than written out,
// because a reference spelled in full in this tree is one amenbo's own commit lint refuses — an
// id means nothing outside the store that issued it.
var sampleRef = refPrefixTask + "42"

// taskStatuses is every state a task is reported as moving to.
var taskStatuses = []string{statusTodo, statusInProgress, statusDone, statusBlocked, statusRejected}

// amenboLanguages is every language amenbo can be set to. A message is written in the one the
// user already chose there, so this list and the wordings are the same list, or the choice
// silently stops being honoured.
var amenboLanguages = []string{
	"de", "en", "es", "fr", "hi", "id", "it", "ja", "ko", "nl",
	"pl", "pt-BR", "ru", "th", "tr", "uk", "vi", "zh-Hans", "zh-Hant",
}

func TestEveryLanguageAmenboHasIsWrittenHere(t *testing.T) {
	held := make(map[string]bool, len(wordings))
	for _, w := range wordings {
		if held[w.language] {
			t.Errorf("%q is written twice, and which one is used is then a matter of order", w.language)
		}
		held[w.language] = true
	}
	for _, language := range amenboLanguages {
		if !held[language] {
			t.Errorf("a user reading %q is written to in English, having chosen otherwise", language)
		}
	}
	if len(wordings) != len(amenboLanguages) {
		t.Errorf("%d languages are written here, want the %d amenbo has", len(wordings), len(amenboLanguages))
	}
}

func TestEveryLanguageSaysEveryEventAndEveryStatus(t *testing.T) {
	for _, w := range wordings {
		for _, event := range reportableEvents {
			sentence, ok := w.sentences[event]
			if !ok {
				t.Errorf("%s says nothing for %s", w.language, event)
				continue
			}
			for _, part := range []string{"{who}", "{ref}"} {
				if !strings.Contains(sentence, part) {
					t.Errorf("%s says %q for %s, which never names %s", w.language, sentence, event, part)
				}
			}
		}
		if s := w.sentences[eventTaskStatusChanged]; !strings.Contains(s, "{status}") {
			t.Errorf("%s says %q for a status change, which never says which status", w.language, s)
		}
		for _, status := range taskStatuses {
			if w.statuses[status] == "" {
				t.Errorf("%s has no word for %q, so a message would show amenbo's raw one", w.language, status)
			}
		}
	}
}

func TestWordingForPicksTheLanguageAmenboNames(t *testing.T) {
	for _, tc := range []struct {
		name, language, want string
	}{
		{"the code as amenbo gives it", "ja", "ja"},
		{"a script the list carries", "zh-Hant", "zh-Hant"},
		{"a region the list does not", "ja-JP", "ja"},
		{"a bare code with two scripts resolves the same way every time", "zh", "zh-Hans"},
		{"case is not what tells two languages apart", "PT-br", "pt-BR"},
		{"a language there are no sentences for", "eo", defaultLanguage},
		{"no language at all", "", defaultLanguage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wordingFor(tc.language).language; got != tc.want {
				t.Errorf("wordingFor(%q) = %q, want %q", tc.language, got, tc.want)
			}
		})
	}
}

// anEventAt is one event with the moment and the record a test wants to see written.
func anEventAt(event, at string) input {
	return input{V: contractVersion, Event: event, ID: 42, Actor: actorAI, At: at}
}

// spoken is what the details of a message look like once everything has been read back.
func spoken(language string) details {
	return details{
		record:   record{ref: sampleRef, title: "Ship the thing"},
		project:  "amenbo",
		language: language,
		aiName:   "Sora",
		userName: "you",
	}
}

// atLocal is the moment written the way the machine reading it would.
func atLocal(t *testing.T, at string) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return parsed.Local().Format(timeLayout)
}

func TestEventLineIsTheMomentThenWhatHappenedThenWhatItWasCalled(t *testing.T) {
	at := "2026-08-01T05:32:05Z"

	got := eventLine(anEventAt(eventTaskCreated, at), spoken("en"))

	if want := atLocal(t, at) + "  Sora created " + sampleRef + " — Ship the thing"; got != want {
		t.Errorf("eventLine = %q, want %q", got, want)
	}
}

func TestEventLineIsWrittenInTheLanguageAmenboIsSetTo(t *testing.T) {
	at := "2026-08-01T05:32:05Z"

	got := eventLine(anEventAt(eventTaskCreated, at), spoken("ja"))

	if want := atLocal(t, at) + "  Sora が " + sampleRef + " を作成しました — Ship the thing"; got != want {
		t.Errorf("eventLine = %q, want %q", got, want)
	}
}

func TestEventLineCallsAStatusWhatAmenboCallsIt(t *testing.T) {
	in := anEventAt(eventTaskStatusChanged, "2026-08-01T05:33:10Z")
	in.New = statusInProgress

	for _, tc := range []struct{ language, want string }{
		{"ja", "進行中"},
		{"en", "In progress"},
		{"de", "In Arbeit"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			if got := eventLine(in, spoken(tc.language)); !strings.Contains(got, tc.want) {
				t.Errorf("eventLine = %q, want the status written as %q", got, tc.want)
			}
		})
	}
}

func TestEventLineWritesAStatusItHasNoWordForAsAmenboWroteIt(t *testing.T) {
	in := anEventAt(eventTaskStatusChanged, "2026-08-01T05:33:10Z")
	in.New = "hibernating"

	if got := eventLine(in, spoken("ja")); !strings.Contains(got, "hibernating") {
		t.Errorf("eventLine = %q, want the status amenbo named", got)
	}
}

func TestEventLineWithoutATitleStopsAtWhatHappened(t *testing.T) {
	d := spoken("en")
	d.title = ""

	got := eventLine(anEventAt(eventTaskDone, "2026-08-01T05:51:02Z"), d)

	if strings.Contains(got, "—") {
		t.Errorf("eventLine = %q, want nothing trailing where the title would be", got)
	}
	if !strings.HasSuffix(got, "Sora finished "+sampleRef) {
		t.Errorf("eventLine = %q, want what happened and to what", got)
	}
}

func TestEventLineNamesTheNumberWhenTheRefCouldNotBeRead(t *testing.T) {
	d := spoken("en")
	d.ref = ""

	if got := eventLine(anEventAt(eventTaskDone, "2026-08-01T05:51:02Z"), d); !strings.Contains(got, "#42") {
		t.Errorf("eventLine = %q, want the number the payload carried", got)
	}
}

func TestEventLineNamesAnActorEvenWhenNobodyWasRead(t *testing.T) {
	d := spoken("en")
	d.aiName = ""

	if got := eventLine(anEventAt(eventTaskDone, "2026-08-01T05:51:02Z"), d); !strings.Contains(got, unknownActor) {
		t.Errorf("eventLine = %q, want a subject for the sentence", got)
	}
}

func TestEventLineWritesTheMomentInTheTimeOfThisMachine(t *testing.T) {
	at := "2026-08-01T23:45:00Z"

	got := eventLine(anEventAt(eventTaskDone, at), spoken("en"))

	if !strings.HasPrefix(got, atLocal(t, at)) {
		t.Errorf("eventLine = %q, want it to open with %q", got, atLocal(t, at))
	}
	if strings.HasPrefix(got, at) {
		t.Error("the moment was left in UTC, as it arrived")
	}
}

func TestEventLineKeepsAMomentItCannotReadAsItArrived(t *testing.T) {
	if got := eventLine(anEventAt(eventTaskDone, "sometime"), spoken("en")); !strings.HasPrefix(got, "sometime") {
		t.Errorf("eventLine = %q, want the moment as it was given rather than one made up", got)
	}
}

func TestEventLineReportsAnEventNoLanguageHasASentenceFor(t *testing.T) {
	got := eventLine(anEventAt("task.hibernated", "2026-08-01T05:51:02Z"), spoken("ja"))

	for _, want := range []string{"task.hibernated", sampleRef, "Sora"} {
		if !strings.Contains(got, want) {
			t.Errorf("eventLine = %q, want it to still carry %q", got, want)
		}
	}
}

func TestEveryLanguageWritesALineForEveryEvent(t *testing.T) {
	for _, w := range wordings {
		for _, event := range reportableEvents {
			in := anEventAt(event, "2026-08-01T05:32:05Z")
			in.New = statusInProgress

			got := eventLine(in, spoken(w.language))

			if strings.Contains(got, "{") {
				t.Errorf("%s / %s left something unfilled: %q", w.language, event, got)
			}
			if !strings.Contains(got, sampleRef) || !strings.Contains(got, "Sora") {
				t.Errorf("%s / %s = %q, want it to name who and what", w.language, event, got)
			}
		}
	}
}
