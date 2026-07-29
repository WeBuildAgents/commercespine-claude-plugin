---
description: Explains how Ecombrain authentication and token storage work, and how to recover when a token is missing or rejected. Use when an Ecombrain API call fails with an authentication error, when checking whether the user is connected to Ecombrain, or when the user asks about login/tokens/credentials for Ecombrain.
---

# Ecombrain authentication

The Ecombrain plugin authenticates with a per-user **bearer token** obtained
through a browser login. All API access uses this token; there is no MCP server.

## How it works

- `/ecombrain:login` runs `ecombrain-login`, which opens the browser, lets the
  user pick or create a token, and captures it via a temporary loopback callback
  (`http://127.0.0.1:<port>/callback`) protected by a random `state` nonce.
- The token is saved to `~/.config/ecombrain/credentials.json` (mode `0600`).
- Every GraphQL request sends `Authorization: Bearer <token>`.

## Checking status

Run `ecombrain-config` to see the resolved frontend/API URLs and whether a token
is present. It never prints the token itself. Use `ecombrain-config --json` for
a machine-readable form.

## Recovering from auth failures

Ecombrain scripts exit with **code 2** on authentication problems (no token, or a
token the API rejected with 401/403 or an "unauthenticated" GraphQL error). When
you see this:

1. Tell the user their Ecombrain session needs to be (re)established.
2. Instruct them to run `/ecombrain:login`.
3. After they confirm success, retry the original request.

Do not attempt to read, guess, or reconstruct the token yourself — always route
the user through `/ecombrain:login`.

## Configuration

- The frontend/API URLs are hardcoded constants in `lib/config.js`. Every user
  and environment talks to the same two endpoints.
- **No environment variables are consulted for URLs**, and no URL config file is
  read. This is deliberate: a stray or hostile env var can never redirect the
  bearer token to another host. Do not tell the user to set an env var to change
  an endpoint — none exists, and setting one has no effect.
- To target a different environment, edit those two constants in a local checkout.
