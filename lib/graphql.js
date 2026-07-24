'use strict';

// Shared GraphQL execution used by both `ecombrain-gql` and the login token
// verification step. Read-only: mutations/subscriptions are rejected.

const config = require('./config');

class AuthError extends Error {}
class GraphQLRequestError extends Error {
  constructor(message, details) {
    super(message);
    this.details = details;
  }
}

// Reject anything that is not a read-only query. Strips comments and string
// literals first so the keyword check does not trip on field names or values.
function assertReadOnly(query) {
  const stripped = query
    .replace(/"""[\s\S]*?"""/g, '""') // block strings
    .replace(/"(?:\\.|[^"\\])*"/g, '""') // regular strings
    .replace(/#[^\n]*/g, ''); // comments
  if (/\b(mutation|subscription)\b/i.test(stripped)) {
    throw new GraphQLRequestError(
      'Refusing to run: the Ecombrain Data Layer is read-only. ' +
        'Only GraphQL queries are permitted (no mutation/subscription).'
    );
  }
}

// Execute a GraphQL operation and return the parsed `data`.
// Throws AuthError on 401/authentication failures, GraphQLRequestError otherwise.
async function execute(query, variables) {
  assertReadOnly(query);

  const token = config.getToken();
  if (!token) {
    throw new AuthError(
      'No Ecombrain token found. Run /ecombrain:login to authenticate.'
    );
  }

  const url = config.apiUrl();
  let res;
  try {
    res = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ query, variables: variables || {} }),
    });
  } catch (err) {
    throw new GraphQLRequestError(
      `Could not reach the Ecombrain API at ${url}: ${err.message}`
    );
  }

  if (res.status === 401 || res.status === 403) {
    throw new AuthError(
      'Ecombrain rejected the stored token (unauthenticated). ' +
        'Run /ecombrain:login to sign in again.'
    );
  }

  const text = await res.text();
  let body;
  try {
    body = JSON.parse(text);
  } catch (_) {
    throw new GraphQLRequestError(
      `Ecombrain API returned a non-JSON response (HTTP ${res.status}).`,
      text.slice(0, 500)
    );
  }

  if (body.errors && body.errors.length) {
    // Surface an auth error hidden inside a 200 GraphQL error, too.
    const authy = body.errors.some((e) =>
      /unauthenticated|unauthorized|forbidden|invalid token/i.test(e.message || '')
    );
    if (authy) {
      throw new AuthError(
        'Ecombrain rejected the stored token. Run /ecombrain:login to sign in again.\n' +
          body.errors.map((e) => `- ${e.message}`).join('\n')
      );
    }
    throw new GraphQLRequestError(
      'GraphQL returned errors:\n' +
        body.errors.map((e) => `- ${e.message}`).join('\n'),
      body.errors
    );
  }

  return body.data;
}

// Minimal query used to confirm a token works after login.
const PING_QUERY = '{ __typename }';

module.exports = { execute, assertReadOnly, PING_QUERY, AuthError, GraphQLRequestError };
