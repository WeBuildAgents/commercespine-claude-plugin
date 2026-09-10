# Commerce Spine — Claude plugin

Gives Claude access to your Commerce Spine account: **read-only** queries against the
Data Layer (Amazon ads, retail and inventory data) over GraphQL, plus the Action
Layer, where Claude can propose, review and approve Amazon Ads changes.
Authenticate once with `/commercespine:login`.

**Approving an Action Layer proposal enqueues a real change against your live
Amazon Ads account.** Creating and listing proposals changes nothing; only
approval executes. The skills require explicit confirmation before approving or
rejecting.

There is **no MCP server** — all API access goes through a small, dependency-free
native binary that uses a per-user bearer token.

## Requirements

- **No runtime to install.** Prebuilt native binaries ship with the plugin (see
  [Supported platforms](#supported-platforms)).
- A machine where Claude runs locally (Claude Desktop or Claude Code CLI) so the
  browser login and its temporary `localhost` callback work

## Supported platforms

`bin/` holds three-line wrappers; the shared dispatcher and one static binary per
platform live in `libexec/`. The dispatcher picks the right binary from
`uname -s`/`uname -m` (or `PROCESSOR_ARCHITECTURE` on Windows). It sits in
`libexec/` rather than `bin/` because `bin/` is placed on PATH when the plugin is
enabled, and a helper there would appear as a runnable command.

| OS | Architectures | Minimum version |
| --- | --- | --- |
| macOS | arm64 (Apple Silicon), amd64 (Intel) | macOS 12 Monterey |
| Linux | amd64, arm64 | kernel 3.2+, **glibc or musl** |
| Windows | amd64, arm64 | Windows 10 / Server 2016 |

Linux builds are fully static (`CGO_ENABLED=0`, no `INTERP` segment), so a single
binary runs on Debian/Ubuntu/RHEL *and* Alpine with no `GLIBC_2.xx` errors. The
macOS builds link only `libSystem`/`CoreFoundation`/`Security`, which are present
on every macOS install — Apple does not support fully static executables.

Unsupported platforms (32-bit ARM, 32-bit x86) fail with a clear message rather
than a confusing exec error. To add one, append its `GOOS/GOARCH` pair to
`TARGETS` in [`scripts/build.sh`](scripts/build.sh) and rebuild.

The floors above are those of the toolchain pinned in `go.mod`
(`toolchain go1.26.1`), so they are a property this repo enforces rather than of
whoever happens to run the build. Building with an older Go lowers them — see
[go.dev/wiki/MinimumRequirements](https://go.dev/wiki/MinimumRequirements) for
each release's floors, and change the pin deliberately if you need to support
older systems.

## Building

`go.mod` pins `toolchain go1.26.1`, so the Go tool fetches that exact version if
needed and every build produces the same OS floors. Cross-compiles every target
from any one machine:

```bash
./scripts/build.sh 0.5.0
```

Binaries are committed to `libexec/` so the plugin works straight from a clone,
with no build step or network fetch during install.

## Installation

This repo is both a Claude plugin **and** its marketplace. Install in two steps.

### Claude Code (terminal)

```bash
/plugin marketplace add WeBuildAgents/commercespine-claude-plugin
```

```bash
/plugin install commercespine@commercespine-marketplace
```

Then reload if needed and sign in:

```bash
/reload-plugins
```

```bash
/commercespine:login
```

### Claude Desktop app

If your app version exposes the `/plugin` command, use the same commands above.
Otherwise add the marketplace through the app's **plugins / marketplace settings
UI** using the repo `WeBuildAgents/commercespine-claude-plugin`, enable **commercespine**,
then run `/commercespine:login`.

### Updating

```bash
/plugin update commercespine
```

New versions ship when `version` in `.claude-plugin/plugin.json` is bumped.

### Verify

After installing, `/commercespine:login` appears in the `/` menu and the
`commercespine:data-layer` / `commercespine:authentication` skills load automatically.
Check status any time by asking Claude to run `commercespine-config`.

## How authentication works

1. `/commercespine:login` runs `commercespine-login`, which starts a loopback-only HTTP
   server on an ephemeral port and opens your browser to the Commerce Spine frontend
   `/connect` handoff, passing a `callback_url` and a random `state` nonce.
2. You sign in and create an API token.
3. The frontend redirects back to
   `http://127.0.0.1:<port>/callback#token=…&state=…` (URL **fragment**, not
   query — fragments are never sent to remote servers or written to access logs /
   Referer headers). The local server serves a tiny page that reads the hash and
   **POSTs** the values back to `127.0.0.1` as a JSON body — never as a query
   string, so the token never enters browser history either.
4. The command validates the `state`, stores the token at
   `~/.config/commercespine/credentials.json` (mode `0600`), and verifies it with an
   auth-gated query. Exit `2` means the token was rejected; exit `1` means the
   check could not be completed (e.g. no network).

Every GraphQL request then sends `Authorization: Bearer <token>`.

## Skills

| Skill | Invocation | Purpose |
| --- | --- | --- |
| `login` | `/commercespine:login` | Browser sign-in + token capture/storage |
| `authentication` | model-invoked | Explains auth/token status and re-login recovery |
| `data-layer` | model-invoked | Runs read-only GraphQL queries, discovering the schema from the API's own `dataCatalog` |
| `action-catalog` | model-invoked | Reads the live `/action/v1/action-catalog` to map a change to a published action key |
| `action-proposals` | model-invoked | Creates, lists, approves and rejects Action Layer proposals |

## Bundled commands (`bin/`, on PATH when the plugin is enabled)

Each is a wrapper that execs `libexec/dispatch.sh`, which in turn execs the
platform binary.

- `commercespine-login` — run the auth flow and store a token.
- `commercespine-gql --query '<gql>' [--variables '<json>']` — run a read-only query
  (also accepts a query on stdin). Exit code `2` means an authentication problem
  (re-run login).
- `commercespine-config [--json]` — show resolved URLs and token status (never prints
  the token).

## Configuration

The plugin talks to exactly two endpoints, hardcoded in
[`internal/config/config.go`](internal/config/config.go):

| Constant | Endpoint |
| --- | --- |
| `frontendURL` | `https://console.commercespine.com` (sign-in / `/connect` handoff) |
| `apiURL` | `https://api.commercespine.com/graphql` (read-only Data Layer) |

**No environment variables are consulted for URLs** and no URL config file is
read — so a stray or hostile env var can never redirect the bearer token to
another host. To target a different environment, edit those two constants and rebuild.

`~/.config/commercespine/` holds **only** `credentials.json` (the stored token). On
macOS and Linux the file and its directory are created `0600`/`0700` and that is
guaranteed. On Windows, NTFS ignores those mode bits, so the plugin additionally
resets inherited ACLs with `icacls` — but that step is **best effort**: it is
skipped when `USERNAME` is unset, and `icacls` failures are not fatal.

If no config directory can be determined at all (no `HOME`, no
`XDG_CONFIG_HOME`), the commands fail with a clear error and write nothing —
a token is never placed in the current working directory as a fallback.

Check the resolved values any time with `commercespine-config` (it never prints the
token).

- `COMMERCESPINE_NO_BROWSER=1` (or `commercespine-login --no-browser`) skips opening a
  browser and prints the connect URL instead, for headless/CI testing. It has no
  effect on URLs or the token. Note this does **not** make login work over SSH:
  the callback still has to reach this host's `127.0.0.1`.

## Frontend `/connect` handoff contract (implemented by the frontend + console API)

`GET /connect?callback_url=<url>&state=<opaque>`:

- If the user is not signed in, route them through login, then return to `/connect`.
- Let the user create a new API token (existing secrets cannot be re-revealed).
- On confirm, the console API (`POST /api/v1/connect/handoff`) **must** validate
  `callback_url` server-side, mint the token, and return a redirect URL. The
  browser then navigates to
  `<callback_url>#token=<token>&state=<state>` (fragment, not query).
- **Open-redirect guard (required, server-side):** only accept a `callback_url`
  whose host is `127.0.0.1` or `localhost` over `http`, with no userinfo. Reject
  anything else so a token is never handed to an arbitrary origin. Client-side
  checks are defense in depth only.

## Privacy & data handling

- The plugin runs entirely on your machine. It sends your API token only to
  `https://api.commercespine.com/graphql`, as an `Authorization: Bearer` header.
- Your token is stored locally at `~/.config/commercespine/credentials.json`,
  restricted to your user account. It is never printed, logged, or transmitted
  anywhere else.
- All **GraphQL** access is read-only; `commercespine-gql` refuses mutations and
  subscriptions.
- The **Action Layer** (`/action/v1`) is the one write path. Approving a
  proposal enqueues a real Amazon Ads change; creating or listing one does not.
  The Action skills call it with `curl` and require an explicit confirmation
  before approve or reject. They send the token only to the configured API host,
  or to a host you name yourself in the conversation.

## Releasing (maintainers)

- Bump `version` in **three** places: `.claude-plugin/plugin.json`, and both
  `metadata.version` and the plugin entry's `version` in
  `.claude-plugin/marketplace.json`.
- **Rebuild the binaries at the new version** and commit them — skipping this
  ships binaries whose `commercespine version` disagrees with the manifest:
  ```bash
  ./scripts/build.sh 0.5.0
  ```
- Validate both manifests:
  ```bash
  claude plugin validate .claude-plugin/plugin.json
  claude plugin validate .claude-plugin/marketplace.json
  ```
- Push to `main`. Users pick up the new version with `/plugin update commercespine`.
