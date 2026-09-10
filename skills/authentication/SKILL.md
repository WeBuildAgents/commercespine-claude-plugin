---
description: Explains how Commerce Spine authentication and token storage work, and how to recover when a token is missing or rejected. Use when a Commerce Spine API call fails with an authentication error, when checking whether the user is connected to Commerce Spine, or when the user asks about login/tokens/credentials for Commerce Spine.
---

# Commerce Spine authentication

The Commerce Spine plugin authenticates with a per-user **bearer token** obtained
through a browser login. All API access uses this token; there is no MCP server.

## How it works

- `/commercespine:login` runs `commercespine-login`, which opens the browser, has the
  user **create a new token**, and captures it via a temporary loopback callback
  (`http://127.0.0.1:<port>/callback`) protected by a random `state` nonce.
  Existing tokens are never listed or offered for reuse — Commerce Spine cannot
  re-reveal a token secret once created, so each login mints a new one.
- The token is saved to `~/.config/commercespine/credentials.json` (mode `0600`).
- Every GraphQL request sends `Authorization: Bearer <token>`.

## Checking status

Run `commercespine-config` to see the resolved frontend/API URLs and whether a token
is present. It never prints the token itself. Use `commercespine-config --json` for
a machine-readable form.

## Recovering from auth failures

Commerce Spine commands exit with **code 2** on authentication problems: no token
stored, or a token the API rejected — either an HTTP 401/403, or an HTTP 200
carrying a GraphQL error whose `extensions.code` is `UNAUTHENTICATED`. Any other
failure exits 1, so treat exit 2 as the authoritative signal to re-authenticate.
When you see it:

1. Tell the user their Commerce Spine session needs to be (re)established.
2. Instruct them to run `/commercespine:login`.
3. After they confirm success, retry the original request.

Do not attempt to read, guess, or reconstruct the token yourself — always route
the user through `/commercespine:login`.

## Configuration

- The frontend/API URLs are hardcoded constants in `internal/config/config.go`,
  compiled into the binary. Every user and environment talks to the same two
  endpoints.
- **No environment variables are consulted for URLs**, and no URL config file is
  read. This is deliberate: a stray or hostile env var can never redirect the
  bearer token to another host. Do not tell the user to set an env var to change
  an endpoint — none exists, and setting one has no effect.
- To target a different environment, edit those two constants and rebuild the
  binaries with `scripts/build.sh`.
