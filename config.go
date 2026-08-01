package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The settings a user fills in, by the key amenbo knows each one as. The same spellings are
// declared in dev/manifest.json — that is the form the user is shown — so they are constants
// here and a test reads the manifest back against them: a key renamed on one side and not the
// other would otherwise show up as a setting that is filled in and silently never read.
const (
	keySMTPHost     = "smtp_host"
	keySMTPPort     = "smtp_port"
	keySMTPUser     = "smtp_user"
	keySMTPPassword = "smtp_password"
	keyFrom         = "from"
	keyTo           = "to"
)

// requiredSettings are the ones a user has to fill in, in the order a complaint names them.
// Without a server, an account and its password there is no conversation to have with anyone,
// so the manifest declares these three required and refuses to switch the plugin on until they
// are there — and a test reads that back, because a key required on one side and optional on the
// other is either a plugin that cannot be enabled or one that is enabled and cannot send.
var requiredSettings = []string{keySMTPHost, keySMTPUser, keySMTPPassword}

// defaultSMTPPort is where a message goes when the user leaves smtp_port empty. 587 is the
// submission port, which is what a provider expects an authenticated client to use, and it is
// what nearly every account is reached on — so the setting exists for the accounts that are not,
// rather than as a question every user has to answer.
const defaultSMTPPort = "587"

// secretEnvPrefix is how amenbo names a secret setting in the environment. A secret is kept out
// of the payload on stdin, so smtp_password never appears in the config object and is read from
// AMENBO_CONFIG_SMTP_PASSWORD instead.
const secretEnvPrefix = "AMENBO_CONFIG_"

// settings is what a run sends with: everything the user filled in, plus everything derived from
// it for what they left out. Nothing here is optional by the time it is built — a settings value
// that exists is one a message can be sent on.
type settings struct {
	host     string
	port     string
	user     string
	password string
	from     string
	// to is every address the message goes to, in the order the user wrote them. Never empty.
	to []string
}

// secretEnv is the environment variable a secret setting arrives in.
func secretEnv(key string) string {
	return secretEnvPrefix + strings.ToUpper(key)
}

// configValue reads one setting out of the payload's config object.
//
// A setting is text the user typed, so a string is what arrives; a number is accepted too,
// because a port is a number to anything that decides to write it as one, and reading it as
// absent would silently send to 587 instead. Anything else is treated as unset — a value this
// plugin cannot make sense of is not one to guess at.
func configValue(cfg map[string]any, key string) string {
	switch v := cfg[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// loadConfig turns what the user filled in into the settings a run sends with, deriving what
// they left out.
//
// Three settings have to be there — the server, the account, and its password — because without
// them there is no conversation to have with anyone. The rest are derived from the account: it
// is the address most providers will let a message claim to be from, and it is the mailbox its
// owner reads, so filling in three settings is enough to be reporting to yourself. An address in
// `to` is what changes that, and it holds as many as the user separates with commas, since a
// notification several people read is the ordinary case rather than a second setting.
//
// A required setting that is empty ends the run rather than sending something incomplete: the
// error names every one of them at once, so the user fixes the configuration in one go instead
// of discovering the next missing key on the next event.
func loadConfig(cfg map[string]any) (settings, error) {
	s := settings{
		host:     configValue(cfg, keySMTPHost),
		port:     configValue(cfg, keySMTPPort),
		user:     configValue(cfg, keySMTPUser),
		password: strings.TrimSpace(os.Getenv(secretEnv(keySMTPPassword))),
		from:     configValue(cfg, keyFrom),
		to:       splitAddresses(configValue(cfg, keyTo)),
	}

	read := map[string]string{keySMTPHost: s.host, keySMTPUser: s.user, keySMTPPassword: s.password}
	var missing []string
	for _, key := range requiredSettings {
		if read[key] == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return settings{}, fmt.Errorf("no message is sent while a required setting is empty: %s — fill each in with `amenbo plugin config set %s <key> <value>`",
			strings.Join(missing, ", "), pluginName)
	}

	if s.port == "" {
		s.port = defaultSMTPPort
	}
	if s.from == "" {
		s.from = s.user
	}
	if len(s.to) == 0 {
		s.to = []string{s.user}
	}
	return s, nil
}

// splitAddresses breaks a comma-separated list of addresses apart, dropping the empties a
// trailing comma or a stray separator leaves behind.
func splitAddresses(v string) []string {
	var out []string
	for _, addr := range strings.Split(v, ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			out = append(out, addr)
		}
	}
	return out
}
