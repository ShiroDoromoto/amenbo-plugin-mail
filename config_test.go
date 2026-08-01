package main

import (
	"strings"
	"testing"
)

// filled is a configuration with every required setting present, for the tests that vary one
// thing about it. The password is not among them: it never travels in the config object.
func filled() map[string]any {
	return map[string]any{
		keySMTPHost: "smtp.example.com",
		keySMTPUser: "you@example.com",
	}
}

// setPassword puts the secret where amenbo puts it, for the duration of one test.
func setPassword(t *testing.T, v string) {
	t.Helper()
	t.Setenv(secretEnv(keySMTPPassword), v)
}

func TestLoadConfigDerivesWhatIsLeftOut(t *testing.T) {
	setPassword(t, "app-password")

	got, err := loadConfig(filled())
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.port != defaultSMTPPort {
		t.Errorf("port = %q, want the default %q", got.port, defaultSMTPPort)
	}
	if got.from != "you@example.com" {
		t.Errorf("from = %q, want the account it authenticates as", got.from)
	}
	if len(got.to) != 1 || got.to[0] != "you@example.com" {
		t.Errorf("to = %q, want the account's own mailbox", got.to)
	}
	if got.password != "app-password" {
		t.Errorf("password = %q, want the one in the environment", got.password)
	}
}

func TestLoadConfigKeepsWhatIsFilledIn(t *testing.T) {
	setPassword(t, "app-password")

	cfg := filled()
	cfg[keySMTPPort] = "465"
	cfg[keyFrom] = "amenbo@example.com"
	cfg[keyTo] = "someone@example.com"

	got, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.port != "465" || got.from != "amenbo@example.com" {
		t.Errorf("port/from = %q/%q, want what was filled in", got.port, got.from)
	}
	if len(got.to) != 1 || got.to[0] != "someone@example.com" {
		t.Errorf("to = %q, want what was filled in rather than the account", got.to)
	}
}

func TestLoadConfigSplitsSeveralRecipients(t *testing.T) {
	setPassword(t, "app-password")

	cfg := filled()
	cfg[keyTo] = " one@example.com , two@example.com ,, three@example.com,"

	got, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := []string{"one@example.com", "two@example.com", "three@example.com"}
	if len(got.to) != len(want) {
		t.Fatalf("to = %q, want %q", got.to, want)
	}
	for i := range want {
		if got.to[i] != want[i] {
			t.Errorf("to[%d] = %q, want %q", i, got.to[i], want[i])
		}
	}
}

// A separator with nothing but whitespace around it leaves no address at all, so the account is
// what the message goes to — the same as leaving the setting empty.
func TestLoadConfigFallsBackWhenToHoldsNoAddress(t *testing.T) {
	setPassword(t, "app-password")

	cfg := filled()
	cfg[keyTo] = " , "

	got, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(got.to) != 1 || got.to[0] != "you@example.com" {
		t.Errorf("to = %q, want the account's own mailbox", got.to)
	}
}

func TestLoadConfigRefusesWhenARequiredSettingIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		drop string
	}{
		{"no host", keySMTPHost},
		{"no account", keySMTPUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPassword(t, "app-password")

			cfg := filled()
			cfg[tc.drop] = "   "

			_, err := loadConfig(cfg)
			if err == nil {
				t.Fatalf("loadConfig with an empty %s: want an error", tc.drop)
			}
			if !strings.Contains(err.Error(), tc.drop) {
				t.Errorf("error %q does not name %s", err, tc.drop)
			}
		})
	}

	t.Run("no password", func(t *testing.T) {
		setPassword(t, "")

		_, err := loadConfig(filled())
		if err == nil {
			t.Fatalf("loadConfig with no password in the environment: want an error")
		}
		if !strings.Contains(err.Error(), keySMTPPassword) {
			t.Errorf("error %q does not name %s", err, keySMTPPassword)
		}
	})
}

// One run fixes one configuration, so an error that stopped at the first empty setting would
// send the user back for the next one on the next event.
func TestLoadConfigNamesEveryEmptyRequiredSetting(t *testing.T) {
	setPassword(t, "")

	_, err := loadConfig(nil)
	if err == nil {
		t.Fatalf("loadConfig with nothing filled in: want an error")
	}
	for _, key := range []string{keySMTPHost, keySMTPUser, keySMTPPassword} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
}

// The password is a secret, so amenbo keeps it out of the payload. One arriving there anyway is
// not the user's password — it is something else wearing the key's name.
func TestLoadConfigIgnoresAPasswordInThePayload(t *testing.T) {
	setPassword(t, "app-password")

	cfg := filled()
	cfg[keySMTPPassword] = "from-the-payload"

	got, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.password != "app-password" {
		t.Errorf("password = %q, want the one from the environment", got.password)
	}
}

// A port is a number to anything that decides to write it as one, and JSON has no other kind of
// number — so a value that arrives unquoted is the port, not an absent setting.
func TestConfigValueReadsANumericPort(t *testing.T) {
	if got := configValue(map[string]any{keySMTPPort: float64(465)}, keySMTPPort); got != "465" {
		t.Errorf("configValue = %q, want %q", got, "465")
	}
}

func TestConfigValueTreatsWhatItCannotReadAsUnset(t *testing.T) {
	if got := configValue(map[string]any{keyTo: []any{"you@example.com"}}, keyTo); got != "" {
		t.Errorf("configValue = %q, want it treated as unset", got)
	}
}

func TestSecretEnvNamesTheVariableAmenboSetsIt(t *testing.T) {
	if got := secretEnv(keySMTPPassword); got != "AMENBO_CONFIG_SMTP_PASSWORD" {
		t.Errorf("secretEnv = %q, want %q", got, "AMENBO_CONFIG_SMTP_PASSWORD")
	}
}
