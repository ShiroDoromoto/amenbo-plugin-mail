package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// manifestPath is the manifest an install lays down beside the binary. It is what the user is
// shown when they fill the settings in, so it and the keys this code reads are two halves of one
// contract — and nothing at runtime notices when they stop agreeing.
const manifestPath = "dev/manifest.json"

type manifestSetting struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Default  string `json:"default"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	Options  []struct {
		Value string `json:"value"`
	} `json:"options"`
}

type manifest struct {
	Config []manifestSetting `json:"config"`
	Events []string          `json:"events"`
}

func readManifest(t *testing.T) manifest {
	t.Helper()
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading %s: %v", manifestPath, err)
	}
	var doc manifest
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", manifestPath, err)
	}
	return doc
}

// settings indexes the declared settings by the key the user fills them in under.
func (m manifest) settings(t *testing.T) map[string]manifestSetting {
	t.Helper()
	by := make(map[string]manifestSetting, len(m.Config))
	for _, s := range m.Config {
		by[s.Key] = s
	}
	return by
}

// A setting this code reads under a key the manifest does not declare is one the user is never
// asked for, and so one that is always empty.
func TestManifestDeclaresEverySettingThisCodeReads(t *testing.T) {
	declared := readManifest(t).settings(t)
	for _, key := range []string{keySMTPHost, keySMTPPort, keySMTPUser, keySMTPPassword, keyFrom, keyTo, keyEvents} {
		if _, ok := declared[key]; !ok {
			t.Errorf("%s declares no %q, so nothing ever fills it in", manifestPath, key)
		}
	}
}

// The manifest's required settings are what amenbo refuses to enable the plugin without;
// requiredSettings is what this code refuses to send without. One required in the manifest alone
// is a setting the user is made to fill in for nothing; one required here alone is a plugin that
// enables cleanly and then fails on every event.
func TestManifestRequiresExactlyWhatCannotBeDerived(t *testing.T) {
	declared := readManifest(t).settings(t)

	required := make(map[string]bool, len(requiredSettings))
	for _, key := range requiredSettings {
		required[key] = true
		if !declared[key].Required {
			t.Errorf("%s does not require %q, but nothing is sent without it", manifestPath, key)
		}
	}
	for key, s := range declared {
		if s.Required && !required[key] {
			t.Errorf("%s requires %q, but an empty one is derived rather than refused", manifestPath, key)
		}
	}
}

// The manifest's default is what the user is shown; defaultSMTPPort is what an empty setting
// actually sends on. Two different numbers would mean the form promises one port and the plugin
// uses another.
func TestManifestPortDefaultMatchesTheOneDerivedHere(t *testing.T) {
	declared := readManifest(t).settings(t)
	if got := declared[keySMTPPort].Default; got != defaultSMTPPort {
		t.Errorf("%s defaults %s to %q, but an empty one is sent on %q", manifestPath, keySMTPPort, got, defaultSMTPPort)
	}
}

// An event the manifest does not subscribe to never arrives, however the user's setting names
// it: what is subscribed to is fixed at install, and the choosing is left to the setting, so the
// two lists have to be the same eleven.
func TestManifestSubscribesToEveryReportableEvent(t *testing.T) {
	doc := readManifest(t)

	reportable := eventSet(reportableEvents)
	subscribed := eventSet(doc.Events)
	for _, event := range reportableEvents {
		if !subscribed[event] {
			t.Errorf("%s does not subscribe to %q, so choosing it reports nothing", manifestPath, event)
		}
	}
	for _, event := range doc.Events {
		if !reportable[event] {
			t.Errorf("%s subscribes to %q, but nothing here can report it", manifestPath, event)
		}
	}
}

// The options are what the user is offered, and amenbo refuses to store anything else — so an
// event missing from them is one that can be reported and never chosen.
func TestManifestOffersEveryReportableEventAndTheDefaultFour(t *testing.T) {
	declared := readManifest(t).settings(t)

	offered := make(map[string]bool)
	for _, o := range declared[keyEvents].Options {
		offered[o.Value] = true
	}
	for _, event := range reportableEvents {
		if !offered[event] {
			t.Errorf("%s does not offer %q, so nobody can choose it", manifestPath, event)
		}
	}
	if len(offered) != len(reportableEvents) {
		t.Errorf("%s offers %d events, want the %d that can be reported", manifestPath, len(offered), len(reportableEvents))
	}

	if got, want := declared[keyEvents].Default, strings.Join(defaultEvents, ","); got != want {
		t.Errorf("%s defaults %s to %q, but an unchosen one reports %q", manifestPath, keyEvents, got, want)
	}
}

// The label is the whole of what a setting says for itself: the manifest has no room for a
// description or a placeholder beside it, so a setting without one is a blank field with a key
// above it.
func TestManifestNamesEverySetting(t *testing.T) {
	for _, s := range readManifest(t).Config {
		if strings.TrimSpace(s.Label) == "" {
			t.Errorf("%s gives %q no label, so it is shown as a field with nothing said about it", manifestPath, s.Key)
		}
	}
}

// A default is where a setting the user never filled in actually goes, which is why only the two
// that have a real one carry it. The server is the setting this matters most for: a provider's
// name left there as an example would hand the account and its password of anyone who forgot the
// field to a service they never chose.
func TestManifestDefaultsOnlyWhereThereIsARealDefault(t *testing.T) {
	real := map[string]bool{keySMTPPort: true, keyEvents: true}
	for _, s := range readManifest(t).Config {
		if s.Default != "" && !real[s.Key] {
			t.Errorf("%s defaults %q to %q, but an unfilled one has no right answer to fall back on",
				manifestPath, s.Key, s.Default)
		}
	}
}

// Only a setting declared secret is kept out of the payload and put in the environment, which is
// the one place loadConfig looks for the password.
func TestManifestKeepsThePasswordSecret(t *testing.T) {
	declared := readManifest(t).settings(t)
	if !declared[keySMTPPassword].Secret {
		t.Errorf("%s does not declare %s secret, so it arrives in the payload and %s stays empty",
			manifestPath, keySMTPPassword, secretEnv(keySMTPPassword))
	}
}
