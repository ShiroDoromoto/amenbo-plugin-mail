package main

import (
	"encoding/json"
	"os"
	"testing"
)

// manifestPath is the manifest an install lays down beside the binary. It is what the user is
// shown when they fill the settings in, so it and the keys this code reads are two halves of one
// contract — and nothing at runtime notices when they stop agreeing.
const manifestPath = "dev/manifest.json"

type manifestSetting struct {
	Key      string `json:"key"`
	Default  string `json:"default"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

func readManifest(t *testing.T) map[string]manifestSetting {
	t.Helper()
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading %s: %v", manifestPath, err)
	}
	var doc struct {
		Config []manifestSetting `json:"config"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", manifestPath, err)
	}
	by := make(map[string]manifestSetting, len(doc.Config))
	for _, s := range doc.Config {
		by[s.Key] = s
	}
	return by
}

// A setting this code reads under a key the manifest does not declare is one the user is never
// asked for, and so one that is always empty.
func TestManifestDeclaresEverySettingThisCodeReads(t *testing.T) {
	declared := readManifest(t)
	for _, key := range []string{keySMTPHost, keySMTPPort, keySMTPUser, keySMTPPassword, keyFrom, keyTo} {
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
	declared := readManifest(t)

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
	declared := readManifest(t)
	if got := declared[keySMTPPort].Default; got != defaultSMTPPort {
		t.Errorf("%s defaults %s to %q, but an empty one is sent on %q", manifestPath, keySMTPPort, got, defaultSMTPPort)
	}
}

// Only a setting declared secret is kept out of the payload and put in the environment, which is
// the one place loadConfig looks for the password.
func TestManifestKeepsThePasswordSecret(t *testing.T) {
	declared := readManifest(t)
	if !declared[keySMTPPassword].Secret {
		t.Errorf("%s does not declare %s secret, so it arrives in the payload and %s stays empty",
			manifestPath, keySMTPPassword, secretEnv(keySMTPPassword))
	}
}
