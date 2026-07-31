# mail — amenbo's email notification plugin

Report by email what your AI did in a project while you were away from it.

Three things decide what arrives.

- **Only the AI's writes.** Every event names who drove it, and a write you drove yourself is
  one you were present for — a mailbox repeating it back to you is noise. What is worth a
  notification is the work that happened while nobody was watching it.
- **The mailbox is the setting.** A setting belongs to a project, so the value of `to` is which
  mailbox a project reports to. Point two projects at two addresses and they report to two
  mailboxes; there is no address anywhere in the plugin.
- **Which events, you choose.** `events` is a list you tick, from everything amenbo fires. Its
  default is a task created, its status moved, and either terminal — done or decided against.

## Where this is

**The skeleton, not the plugin.** What is here is the repository's shape — the build, the gate,
the release, the manifest, and the plugin's entry point with the payload contract it reads. The
wording of a message and the SMTP conversation that carries it are not written yet, so a hook run
today reads its event and leaves a line in `amenbo plugin log mail` instead of sending anything.

`dev/manifest.json` carries placeholder digests (all zeroes) against a `v1` that has not been cut.
They are replaced from the release run's summary — never transcribed by hand — before the catalog
entry quotes them.

## Settings

| Key | What it is |
| --- | --- |
| `smtp_host` | the SMTP server to hand the message to (required) |
| `smtp_port` | the port on it (defaults to 587) |
| `smtp_user` | the account to authenticate as, where the server asks for one |
| `smtp_password` | that account's password (secret) |
| `from` | the address the message is sent from (required) |
| `to` | the address it is sent to (required) |
| `events` | what to report, from the eleven amenbo fires |

Run the config commands from the folder amenbo is bound to: the settings and the switch are that
project's. The password goes in through `-`, which reads it from stdin — written as an argument it
would sit in the shell's history and in anything reading the process list.

```sh
amenbo plugin config set mail smtp_host smtp.example.com
amenbo plugin config set mail from amenbo@example.com
amenbo plugin config set mail to you@example.com
printf %s '…' | amenbo plugin config set mail smtp_password -
amenbo plugin enable mail            # installing never runs anything; this is the consent
```

## Build

```sh
make test                            # gofmt, vet, tests — the same gate CI runs
make build                           # the binary for this machine
make dist                            # every asset a release publishes, plus its digests
```

Hand-install into a **throwaway** amenbo base to try it against real events:

```sh
make install AMENBO_BASE="$AMENBO_HOME"
```

That skips everything `amenbo plugin install mail` does — resolving the catalog entry, fetching the
released asset, verifying its provenance — so never point it at a base holding work you care about.

## Releasing

Pushing a `v*` tag runs the release workflow: it bakes every asset key the catalog entry publishes,
creates the GitHub release, and prints the digests in the run summary.

**A release is not a distribution.** Nothing installs from those bytes until the catalog entry in
[amenbo-plugins](https://github.com/ShiroDoromoto/amenbo-plugins) points at them, and that entry is
a reviewed pull request. The signature that blesses an asset is the catalog's, made on merge.

## License

Apache-2.0. See [LICENSE](LICENSE).
