// Package login implements the browser sign-in and token capture flow.
//
// Flow:
//  1. Start a loopback-only HTTP server on an ephemeral port.
//  2. Open the browser to the frontend /connect handoff, passing our callback_url
//     and a random state nonce.
//  3. The frontend (after the user logs in and creates a token) redirects back to
//     http://127.0.0.1:<port>/callback#token=...&state=... (fragment, not query —
//     fragments are never sent to remote servers or written to access logs).
//  4. /callback serves a tiny page that reads location.hash and POSTs the values
//     to /complete on the same loopback server.
//  5. Validate the state, save the token (0600), verify it, then exit.
package login

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/WeBuildAgents/ecombrain-claude-plugin/internal/config"
	"github.com/WeBuildAgents/ecombrain-claude-plugin/internal/graphql"
)

// timeout allows for a first-time sign-in that includes 2FA and token creation.
const timeout = 10 * time.Minute

// Run executes the login flow. It returns the process exit code.
func Run(args []string) int {
	noBrowser := os.Getenv("ECOMBRAIN_NO_BROWSER") != ""
	for _, a := range args {
		if a == "--no-browser" {
			noBrowser = true
		}
		if a == "--help" || a == "-h" {
			fmt.Println("Usage: ecombrain-login [--no-browser]")
			return 0
		}
	}

	state, err := randomState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not generate a state nonce: %v\n", err)
		return 1
	}

	// Bind to loopback only, ephemeral port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not start local callback server: %v\n", err)
		return 1
	}
	port := listener.Addr().(*net.TCPAddr).Port

	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)

	mux := http.NewServeMux()

	// First hop: the browser lands on /callback#token=… — the server only ever
	// sees /callback, never the fragment.
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, bridgeHTML)
	})

	// Second hop: the bridge page POSTs the values it read from the fragment.
	// A POST body keeps the token out of the browser's history, which a
	// /complete?token=… redirect would not.
	mux.HandleFunc("/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Token string `json:"token"`
			State string `json:"state"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, "Could not read the callback.")
			return
		}
		// Constant-time-ish comparison is unnecessary here (the nonce never
		// leaves this machine), but a mismatch must not settle the flow.
		if payload.State == "" || payload.State != state {
			writeJSON(w, http.StatusBadRequest, "State mismatch. Please run /ecombrain:login again.")
			return
		}
		if payload.Token == "" {
			writeJSON(w, http.StatusBadRequest, "No token was returned. Please try again.")
			return
		}
		select {
		case done <- result{token: payload.Token}:
			writeJSON(w, http.StatusOK, "Connected to Ecombrain. You can close this tab and return to Claude.")
		default:
			// Already settled — idempotent response for a duplicate submit.
			writeJSON(w, http.StatusOK, "Already connected. You can close this tab.")
		}
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			done <- result{err: err}
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	connectURL := fmt.Sprintf("%s/connect?callback_url=%s&state=%s",
		config.FrontendURL(), url.QueryEscape(callbackURL), url.QueryEscape(state))

	if noBrowser {
		fmt.Println("Open this URL in your browser to sign in to Ecombrain:")
	} else {
		fmt.Println("Opening your browser to sign in to Ecombrain...")
		fmt.Println("If it does not open automatically, paste this URL into your browser:")
	}
	fmt.Println(connectURL)

	if !noBrowser {
		openBrowser(connectURL)
	}

	var token string
	select {
	case res := <-done:
		if res.err != nil {
			fmt.Fprintf(os.Stderr, "Local callback server failed: %v\n", res.err)
			return 1
		}
		token = res.token
	case <-time.After(timeout):
		fmt.Fprintf(os.Stderr, "Login timed out after %s. Run /ecombrain:login to try again.\n", timeout)
		fmt.Fprintln(os.Stderr, "If you are on a remote or SSH machine, the browser cannot reach this host.")
		return 1
	}

	path, err := config.WriteCredentials(config.Credentials{
		Token:      token,
		ObtainedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not save the token: %v\n", err)
		return 1
	}
	fmt.Printf("Token saved to %s\n", path)

	// Distinguish "the API rejected this token" (re-running login helps, exit 2)
	// from "we could not complete the check" (it does not, exit 1). Collapsing
	// both into 2 makes the authentication skill advise a pointless re-login
	// whenever the network hiccups.
	fmt.Println("Verifying token...")
	if _, err := graphql.Execute(graphql.PingQuery, nil); err != nil {
		var authErr *graphql.AuthError
		if errors.As(err, &authErr) {
			fmt.Fprintf(os.Stderr, "The token was saved but Ecombrain rejected it: %v\n", err)
			return 2
		}
		fmt.Fprintf(os.Stderr, "The token was saved but could not be verified: %v\n", err)
		fmt.Fprintln(os.Stderr, "This is usually a network problem — signing in again will not help.")
		return 1
	}
	fmt.Println("Success — Ecombrain is connected. You can now ask data questions.")
	return 0
}

func randomState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeJSON(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}

// bridgeHTML reads #token/#state (never sent over HTTP) and POSTs them to the
// loopback server. The values never enter a URL, so they never reach browser
// history.
const bridgeHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Connecting…</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;text-align:center;color:#1a1a1a}h1{font-size:1.3rem}</style>
</head><body><p id="m">Completing Ecombrain sign-in…</p>
<script>
(function () {
  function show(title, msg) {
    document.body.innerHTML = '<h1>' + title + '</h1><p>' + msg + '</p>';
  }
  try {
    var params = new URLSearchParams(location.hash.replace(/^#/, ''));
    var token = params.get('token');
    var state = params.get('state');
    if (!token || !state) {
      show('Authentication failed', 'No token was returned. Please try again.');
      return;
    }
    // Clear the fragment so the token is not left in the address bar.
    history.replaceState(null, '', location.pathname);
    fetch('/complete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: token, state: state })
    }).then(function (r) {
      return r.json().then(function (b) { return { ok: r.ok, body: b }; });
    }).then(function (res) {
      show(res.ok ? 'Connected to Ecombrain' : 'Authentication failed', res.body.message || '');
    }).catch(function () {
      show('Authentication failed', 'Could not reach the local sign-in helper.');
    });
  } catch (e) {
    show('Authentication failed', 'Could not read the callback.');
  }
})();
</script></body></html>`

// isWSL reports whether we are running under the Windows Subsystem for Linux,
// where GOOS is "linux" but xdg-open cannot reach the Windows browser.
func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	raw, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(raw)), "microsoft")
}

// openBrowser launches the user's default browser. Failures are non-fatal: the
// connect URL is always printed first, so the manual path still works.
func openBrowser(target string) {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", target)
	case runtime.GOOS == "windows":
		// Deliberately NOT `cmd /c start`: cmd.exe treats & as a command
		// separator, and Go only quotes arguments containing spaces or quotes,
		// so the connect URL's &state=… would be split off and executed.
		// rundll32 receives the URL as a single argument with no shell parsing.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case isWSL():
		cmd = exec.Command("powershell.exe", "-NoProfile", "-Command", "Start-Process", "'"+target+"'")
	default:
		cmd = exec.Command("xdg-open", target)
	}
	// Detach: never block on, or inherit stdio from, the browser process.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return // handled by the printed-URL fallback
	}
	go func() { _ = cmd.Wait() }() // reap without blocking
}
