# tt — the site CLI

`tt` drives a go-torrent-trader site over its REST API. Everything it does is
also reachable in the web interface; the point is **automation** — cron jobs,
shell pipelines, CI, and operators on a box with no browser.

This is the foundation: configuration, authentication, output formatting, and
enough commands to prove the plumbing works end to end. The command surface grows
from here, and it is meant to be **designed rather than accreted** — command names
are an interface, and anything that teaches an agent or a script to drive `tt`
depends on them staying stable.

## Install

```bash
task cli:build            # builds ./cli/tt
# or
go install github.com/williamokano/go-torrent-trader/cli/cmd/tt@latest
```

Release builds are published as archives alongside the server and migration tool,
with the version stamped in (`tt version`).

## Getting started

```bash
tt profile set prod --url https://tracker.example.com
tt auth set-token prod          # prompts, without echoing
tt whoami
```

To get a token, log in against the site — see [Authentication](#authentication)
below, and read it before building anything scheduled, because today's token
expires in an hour.

## Configuration

Two files live in `$XDG_CONFIG_HOME/go-torrent-trader` (or `~/.config/go-torrent-trader`),
overridable with `TT_CONFIG_DIR`:

| File | Holds | Mode |
| --- | --- | --- |
| `config.yaml` | profile definitions — site URLs | `0600` |
| `credentials.yaml` | one bearer token per profile | `0600`, enforced on read **and** write |

They are separate on purpose: sharing your profiles with a colleague, or baking
them into a CI image, should never mean sharing a token.

`tt` refuses to touch a `credentials.yaml` that group or others can access — it
does not quietly repair the mode, because that would fix the symptom and leave you
believing your tokens were never exposed. If you see that error, rotate the tokens,
then `chmod 600` the file. Writes also tighten the containing directory: a
world-writable config directory lets someone repoint a profile's URL at their own
host and collect your token on the next command, which the file's own mode does
nothing to prevent.

### Precedence

For every setting: **flag → environment → config file**.

| Setting | Flag | Environment | Config |
| --- | --- | --- | --- |
| Profile | `--profile` | `TT_PROFILE` | `current_profile` |
| Site URL | `--url` | `TT_URL` | the profile's `url` |
| Token | `--token` | `TT_TOKEN` | `credentials.yaml` |

That ordering is what makes CI work without editing files:

```bash
export TT_URL=https://tracker.example.com
export TT_TOKEN="$CI_SITE_TOKEN"
tt whoami -o json
```

Prefer `TT_TOKEN` over `--token`: a flag lands in shell history and in the
process list, where every other user on the box can read it.

**A stored token is bound to its profile's URL.** If `--url` or `TT_URL` points
somewhere other than the profile's own site, `tt` refuses to send the stored
credential and tells you to pass one explicitly. A typo in a hostname should not
hand your bearer token to whoever happens to answer.

**`config.yaml` must not be writable by anyone else, and `tt` refuses to read it
if it is.** The binding above compares the target against the profile's URL, so
it is only worth what that file is worth: anyone who can rewrite `config.yaml`
can repoint a profile at a host they control and collect the token on your next
command, which `credentials.yaml` being `0600` does nothing to prevent — the
token is read correctly and sent to the wrong place. The directory is checked
too, since replacing a file only needs write access to the directory holding it.
Being *readable* is fine; that file holds no secret.

**A redirect is never followed to another origin.** Go's default policy drops the
`Authorization` header only when the *hostname* changes — it ignores the scheme
and the port. So a redirect to another port on the same host forwards your token
to whatever is listening there, and an `https` → `http` redirect forwards it in
cleartext, which is exactly what a reverse proxy missing `X-Forwarded-Proto`
produces. Same-origin redirects are followed normally.

**Plaintext `http://` to a remote host warns on stderr.** It is not refused —
`http` against localhost or a private link is legitimate — but a full-account
credential crossing the network in the clear should not be silent. Loopback is
exempt, so the warning stays worth reading.

On Windows the file-mode checks above are skipped, because the permission bits Go
reports there are synthesised rather than real: every file would look
world-readable and no `chmod` could fix it. `%AppData%` is already per-user.

## Output

`--output table` (default), `json`, or `yaml`.

`table` is a **summary** — each command picks the columns worth reading. `json`
and `yaml` emit the API's response as it arrived, including fields no Go struct
here names, so a script never gets `null` for a field the CLI simply forgot.
Anything you intend to parse should use `json`.

```bash
tt whoami                 # a readable summary
tt whoami -o json | jq -r .user.email
```

> **`tt whoami -o json` includes your passkey.** It is the tracker download
> credential embedded in every announce URL. The table deliberately omits it, but
> the JSON is the server's own response — do not pipe it into a CI log.

Shell completion comes from Cobra:

```bash
tt completion bash > /etc/bash_completion.d/tt
tt completion zsh  > "${fpath[1]}/_tt"
```

## Exit codes

A scheduled job's whole reason for shelling out is to branch on the result, so
failures are classified rather than all collapsing to 1:

| Code | Meaning |
| --- | --- |
| `0` | success |
| `1` | general failure, including a site error such as a 500 |
| `2` | usage — bad flag, bad argument, unknown command |
| `3` | authentication — no credential, or the site rejected it (401/403) |
| `4` | network — unreachable, timed out, TLS failure |

Telling `3` from `4` is what lets a cron wrapper re-authenticate on an expired
token instead of paging someone about an outage.

## Timeouts

`--timeout` (default 30s) bounds each request. It must be positive: `net/http`
reads a zero timeout as "wait forever", which is the exact hang the flag exists to
prevent, so `tt` rejects `0` rather than silently doing the opposite of what a
reader would expect.

## Authentication

`tt` sends whatever token you give it as `Authorization: Bearer <token>` and lets
the server decide what it permits. It deliberately does **not** know whether that
is a session access token or a scoped API key.

Today a site issues a **session access token that expires after an hour**:

```bash
curl -sX POST https://tracker.example.com/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"you","password":"..."}' \
  | jq -r .tokens.access_token
```

That is fine at a terminal and poor in a cron job. Long-lived scoped API keys are
[#170](https://github.com/williamokano/go-torrent-trader/issues/170), and they are
the real credential for this tool; when they land, `tt auth set-token` stores one
with no change here.

**There is no `tt auth login`, and that is a decision worth flagging rather than
assuming.** The site also issues a 30-day refresh token, and a login command could
store that and refresh transparently — the web frontend already keeps one in
`localStorage`, so a `0600` file would be no worse. The argument against is #211's:
teaching everyone to script against a long-lived full-account credential is the
habit scoped tokens exist to prevent, and it is easier not to start than to undo.
That trade is the operator's call, not this PR's.

Two further design questions from
[#211](https://github.com/williamokano/go-torrent-trader/issues/211) are **not**
settled by this foundation, and neither is blocked by it:

- **One binary or two?** Admin commands could live under an `admin` subtree here,
  or in a separate operator binary matching the two OpenAPI documents. Nothing in
  this foundation forecloses either.
- **Break-glass database access?** Deliberately absent. `tt` is a thin REST
  client with one authorisation path — the server's. A tool that reaches past the
  API at 3am is a different tool and probably a different binary.

## Why the client is hand-written

Generating from `backend/api/openapi.yaml` is the better answer *eventually*, and
the public document is a genuine third-party-grade contract to generate against.
But 152 of 189 routes are currently undocumented
([#155](https://github.com/williamokano/go-torrent-trader/issues/155)), so
generating today produces a client full of holes with no signal about where they
are. The types here are the CLI's own, which the shared-nothing rule in
`docs/ARCHITECTURE.md` requires anyway — a third module cannot import
`backend/internal`. The migration tool sets the same precedent.

The hand-written types are kept to the **table columns only**. Response types are
where a hand-written client rots: a struct silently drops every field it does not
name, and a script reading one gets `null`, indistinguishable from `false`. So
`json`/`yaml` bypass the structs entirely and emit what the server sent.

## Layout

```
cli/
  cmd/tt/            main, thin wiring only
  internal/command/  the Cobra tree
  internal/client/   REST client, error mapping
  internal/config/   profiles and credentials
  internal/output/   table / json / yaml rendering
```

Commands live under `internal/command` rather than in `cmd/tt` so the tree can be
exercised in tests with its output captured. Every command prints through a
`Printer` instead of calling `fmt` directly — a command that only knows how to
print a table is a command nobody can script against.

`tt profile` manages which sites exist; `tt auth` manages credentials. The split
keeps an obvious home for server-side key management when #170 lands
(`tt auth create-key` reads naturally; `tt profile create-key` would not).

## Development

```bash
task cli:test
task cli:lint
task cli:coverage    # checks against the CI floor, writes coverage.html
```
