// Package graphql executes read-only queries against the Ecombrain Data Layer.
// Shared by the `gql` subcommand and the post-login token verification step.
package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/WeBuildAgents/ecombrain-claude-plugin/internal/config"
)

// PingQuery confirms a token actually works after login.
//
// It MUST be a query that passes through the API's auth middleware. `{ __typename }`
// is resolved by the GraphQL executor itself without reaching a resolver, so the
// API answers it with HTTP 200 even with no Authorization header at all — which
// made login report success for any string. `amazonAccounts` is auth-gated; an
// account list that is legitimately empty still returns 200 with no errors, so a
// valid token with no linked accounts still verifies correctly.
const PingQuery = "{ amazonAccounts { id } }"

// AuthError means the token is missing or was rejected. Callers exit 2 on this
// so the authentication skill's recovery path fires.
type AuthError struct{ msg string }

func (e *AuthError) Error() string { return e.msg }

// RequestError is any other failure (network, malformed response, GraphQL errors).
type RequestError struct {
	msg     string
	Details any
}

func (e *RequestError) Error() string { return e.msg }

var (
	blockStringRe = regexp.MustCompile(`(?s)"""(.*?)"""`)
	stringRe      = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
	commentRe     = regexp.MustCompile(`#[^\n]*`)
	writeOpRe     = regexp.MustCompile(`(?i)\b(mutation|subscription)\b`)
)

// assertReadOnly rejects anything that is not a read-only query. Comments and
// string literals are stripped first so the keyword check does not trip on field
// names or values.
func assertReadOnly(query string) error {
	stripped := blockStringRe.ReplaceAllString(query, `""`)
	stripped = stringRe.ReplaceAllString(stripped, `""`)
	stripped = commentRe.ReplaceAllString(stripped, "")
	if writeOpRe.MatchString(stripped) {
		return &RequestError{msg: "Refusing to run: the Ecombrain Data Layer is read-only. " +
			"Only GraphQL queries are permitted (no mutation/subscription)."}
	}
	return nil
}

type gqlError struct {
	Message    string `json:"message"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

// authMessageRe is a fallback for servers that don't set extensions.code.
// The production API returns "Authentication is required to access this API.",
// which matched none of the original four alternatives — hence the explicit
// "authentication is required" branch alongside the extensions.code check.
var authMessageRe = regexp.MustCompile(`(?i)unauthenticated|unauthorized|forbidden|invalid token|authentication is required|not authenticated`)

func isAuthError(errs []gqlError) bool {
	for _, e := range errs {
		switch strings.ToUpper(e.Extensions.Code) {
		case "UNAUTHENTICATED", "UNAUTHORIZED", "FORBIDDEN":
			return true
		}
		if authMessageRe.MatchString(e.Message) {
			return true
		}
	}
	return false
}

func joinMessages(errs []gqlError) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString("- ")
		b.WriteString(e.Message)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Execute runs a read-only GraphQL operation and returns the raw `data` value.
func Execute(query string, variables map[string]any) (json.RawMessage, error) {
	if err := assertReadOnly(query); err != nil {
		return nil, err
	}

	// A lookup failure (e.g. no resolvable config directory) is NOT an auth
	// problem — re-running login cannot fix it — so it must not become an
	// AuthError, which callers translate to exit 2.
	token, err := config.Token()
	if err != nil {
		return nil, &RequestError{msg: err.Error()}
	}
	if token == "" {
		return nil, &AuthError{msg: "No Ecombrain token found. Run /ecombrain:login to authenticate."}
	}

	if variables == nil {
		variables = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, &RequestError{msg: "Could not encode the GraphQL request: " + err.Error()}
	}

	url := config.APIURL()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, &RequestError{msg: "Could not build the GraphQL request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 60 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, &RequestError{msg: fmt.Sprintf("Could not reach the Ecombrain API at %s: %v", url, err)}
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, &AuthError{msg: "Ecombrain rejected the stored token (unauthenticated). " +
			"Run /ecombrain:login to sign in again."}
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &RequestError{msg: "Could not read the Ecombrain API response: " + err.Error()}
	}

	var parsed gqlResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		snippet := string(body)
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return nil, &RequestError{
			msg:     fmt.Sprintf("Ecombrain API returned a non-JSON response (HTTP %d).", res.StatusCode),
			Details: snippet,
		}
	}

	if len(parsed.Errors) > 0 {
		// Surface an auth error hidden inside a 200 GraphQL error, too.
		if isAuthError(parsed.Errors) {
			return nil, &AuthError{msg: "Ecombrain rejected the stored token. " +
				"Run /ecombrain:login to sign in again.\n" + joinMessages(parsed.Errors)}
		}
		return nil, &RequestError{
			msg:     "GraphQL returned errors:\n" + joinMessages(parsed.Errors),
			Details: parsed.Errors,
		}
	}

	return parsed.Data, nil
}
