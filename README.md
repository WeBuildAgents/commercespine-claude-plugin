# Ecombrain — Claude plugin

Gives Claude secure, **read-only** access to the Ecombrain Data Layer via its
GraphQL API. Authenticate once with `/ecombrain:login`, then ask Claude data
questions that it answers by querying your Ecombrain account.

There is **no MCP server** — all API access goes through small, zero-dependency
Node scripts in `bin/` that use a per-user bearer token.

## Requirements

- Node.js 18+ (uses the built-in global `fetch`)
- A machine where Claude runs locally (Claude Desktop or Claude Code CLI) so the
  browser login and its temporary `localhost` callback work

## Installation

This repo is both a Claude plugin **and** its marketplace. Install in two steps.

### Claude Code (terminal)

```bash
/plugin marketplace add WeBuildAgents/ecombrain-claude-plugin
```

```bash
/plugin install ecombrain@ecombrain-marketplace
```

Then reload if needed and sign in:

```bash
/reload-plugins
```

```bash
/ecombrain:login
```

### Claude Desktop app

If your app version exposes the `/plugin` command, use the same commands above.
Otherwise add the marketplace through the app's **plugins / marketplace settings
UI** using the repo `WeBuildAgents/ecombrain-claude-plugin`, enable **ecombrain**,
then run `/ecombrain:login`.

### Updating

```bash
/plugin update ecombrain
```

New versions ship when `version` in `.claude-plugin/plugin.json` is bumped.

### Verify

After installing, `/ecombrain:login` appears in the `/` menu and the
`ecombrain:data-layer` / `ecombrain:authentication` skills load automatically.
Check status any time by asking Claude to run `ecombrain-config`.

## How authentication works

1. `/ecombrain:login` runs `ecombrain-login`, which starts a loopback-only HTTP
   server on an ephemeral port and opens your browser to the Ecombrain frontend
   `/connect` handoff, passing a `callback_url` and a random `state` nonce.
2. You sign in and create an API token.
3. The frontend redirects back to
   `http://127.0.0.1:<port>/callback#token=…&state=…` (URL **fragment**, not
   query — fragments are never sent to remote servers or written to access logs /
   Referer headers). The local server serves a tiny page that reads the hash and
   completes capture on loopback only.
4. The script validates the `state`, stores the token at
   `~/.config/ecombrain/credentials.json` (mode `0600`), and verifies it.

Every GraphQL request then sends `Authorization: Bearer <token>`.

## Skills

| Skill | Invocation | Purpose |
| --- | --- | --- |
| `login` | `/ecombrain:login` | Browser sign-in + token capture/storage |
| `authentication` | model-invoked | Explains auth/token status and re-login recovery |
| `data-layer` | model-invoked | Runs read-only GraphQL queries using the bundled schema reference |

## Bundled commands (`bin/`, on PATH when the plugin is enabled)

- `ecombrain-login` — run the auth flow and store a token.
- `ecombrain-gql --query '<gql>' [--variables '<json>']` — run a read-only query
  (also accepts a query on stdin). Exit code `2` means an authentication problem
  (re-run login).
- `ecombrain-config [--json]` — show resolved URLs and token status (never prints
  the token).

## Configuration

The plugin talks to exactly two endpoints, hardcoded in `lib/config.js`:

| Constant | Endpoint |
| --- | --- |
| `FRONTEND_URL` | `https://ecombrain.sellerplex.com` (sign-in / `/connect` handoff) |
| `API_URL` | `https://eb-api.sellerplex.com/graphql` (read-only Data Layer) |

**No environment variables are consulted for URLs** and no URL config file is
read — so a stray or hostile env var can never redirect the bearer token to
another host. To target a different environment, edit those two constants.

`~/.config/ecombrain/` holds **only** `credentials.json` (the stored token). The
file and its directory are locked to the current user on every platform:
`0600`/`0700` via `chmod` on macOS/Linux, and owner-only ACLs via `icacls` on
Windows.

Check the resolved values any time with `ecombrain-config` (it never prints the
token).

- Testing aid: `ECOMBRAIN_NO_BROWSER=1` makes `ecombrain-login` skip opening a
  browser and instead print the connect URL (for headless/CI testing). It has no
  effect on URLs or the token.

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
  `https://eb-api.sellerplex.com/graphql`, as an `Authorization: Bearer` header.
- Your token is stored locally at `~/.config/ecombrain/credentials.json`,
  restricted to your user account. It is never printed, logged, or transmitted
  anywhere else.
- All queries are **read-only**; the plugin refuses GraphQL mutations and
  subscriptions.

## Releasing (maintainers)

- Bump `version` in both `.claude-plugin/plugin.json` and the marketplace entry
  in `.claude-plugin/marketplace.json`.
- Validate both manifests:
  ```bash
  claude plugin validate .claude-plugin/plugin.json
  claude plugin validate .claude-plugin/marketplace.json
  ```
- Push to `main`. Users pick up the new version with `/plugin update ecombrain`.
