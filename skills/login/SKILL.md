---
description: Sign in to Ecombrain and store an API token for the plugin. Use when the user runs /ecombrain:login, asks to connect/authenticate/sign in to Ecombrain, or when another Ecombrain skill reports a missing or rejected token.
disable-model-invocation: true
---

# Connect to Ecombrain

Run the login command to authenticate the user and store their Ecombrain API token.

## Steps

1. Run the login command:

   ```bash
   ecombrain-login
   ```

2. This opens the user's browser to the Ecombrain sign-in / token page. The user
   logs in, **creates a new API token**, and confirms. The browser redirects back
   to a temporary local callback (token in the URL fragment) and the command
   stores the token.

   The flow **only ever mints a brand-new token** — it does not list, display, or
   let the user select their existing tokens. Ecombrain cannot re-reveal a token
   secret after it is created, so there is nothing to pick from. Every
   `/ecombrain:login` therefore issues an additional token on the user's account.
   If they ask to reuse an existing token, or to see which tokens they already
   have, tell them that is not possible from this flow and point them at the
   Ecombrain web app to review or revoke tokens.

3. Report the outcome to the user based on the command's exit status:
   - **Exit 0** — connected and verified. Tell the user they can now ask data
     questions (the `ecombrain:data-layer` skill will answer them). Then go to
     step 4.
   - **Exit 2** — the token was saved but Ecombrain **rejected** it. Signing in
     again is the right fix: tell the user to run `/ecombrain:login` once more,
     and if it keeps failing, to check that their account can create API tokens.
     Stop here.
   - **Exit 1** — the login did not complete. This covers everything that is not
     a rejected token: the sign-in timed out, the callback server could not
     start, the token could not be saved, or it was saved but the API was
     unreachable. **Read the command's error message and relay that specific
     cause** — do not assume it is an authentication problem, and do not tell the
     user that signing in again will fix it unless the message says so. If the
     message mentions a timeout on a remote or SSH machine, explain that the
     browser cannot reach this host's `127.0.0.1`. Stop here.

4. **Only on Exit 0**, ask the user whether they'd like to see the Amazon
   accounts this token can access — for example: "Want me to list the Amazon
   accounts you have access to?"
   - If they say **yes**, hand off to the `ecombrain:data-layer` skill to run the
     small `amazonAccounts` query and present the results (id, store name,
     marketplace, country, currency, active status). This is also a good
     end-to-end confirmation that the connection works.
   - If they say **no** (or don't want to), stop — do not run any query.

   Do not run the query without the user's approval.

## Notes

- The browser opens automatically. If it does not, the command prints a URL for
  the user to paste manually — pass that URL along to the user.
- The token is stored at `~/.config/ecombrain/credentials.json` with `0600`
  permissions. Never print the token.
- To check connection status without re-authenticating, run `ecombrain-config`.
