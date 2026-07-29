---
description: Sign in to Ecombrain and store an API token for the plugin. Use when the user runs /ecombrain:login, asks to connect/authenticate/sign in to Ecombrain, or when another Ecombrain skill reports a missing or rejected token.
disable-model-invocation: true
---

# Connect to Ecombrain

Run the login script to authenticate the user and store their Ecombrain API token.

## Steps

1. Run the login command:

   ```bash
   ecombrain-login
   ```

2. This opens the user's browser to the Ecombrain sign-in / token page. The user
   logs in, **creates a new API token**, and confirms. The browser redirects back
   to a temporary local callback (token in the URL fragment) and the script
   stores the token.

   The flow **only ever mints a brand-new token** — it does not list, display, or
   let the user select their existing tokens. Ecombrain cannot re-reveal a token
   secret after it is created, so there is nothing to pick from. Every
   `/ecombrain:login` therefore issues an additional token on the user's account.
   If they ask to reuse an existing token, or to see which tokens they already
   have, tell them that is not possible from this flow and point them at the
   Ecombrain web app to review or revoke tokens.

3. Report the outcome to the user based on the script's exit status:
   - **Exit 0** — connected and verified. Tell the user they can now ask data
     questions (the `ecombrain:data-layer` skill will answer them). Then go to
     step 4.
   - **Exit 2** — the token was saved but verification failed (e.g. the API was
     unreachable or the token was rejected). Suggest checking that the Ecombrain
     backend is reachable, then running `/ecombrain:login` again. Stop here.
   - **Non-zero otherwise** — show the script's error message and suggest
     retrying. Stop here.

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

- The browser opens automatically. If it does not, the script prints a URL for
  the user to paste manually — pass that URL along to the user.
- The token is stored at `~/.config/ecombrain/credentials.json` with `0600`
  permissions. Never print the token.
- To check connection status without re-authenticating, run `ecombrain-config`.
