// Package config holds the shared configuration and credential storage for the
// Ecombrain plugin. Standard library only — no external dependencies.
package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// --- Service URLs ----------------------------------------------------------
// Production endpoints. No environment variables are consulted for URLs (so a
// stray/hostile env var can never redirect the bearer token to another host).
const (
	frontendURL = "https://ecombrain.sellerplex.com"
	apiURL      = "https://eb-api.sellerplex.com/graphql"
)

// ---------------------------------------------------------------------------

// FrontendURL is the sign-in / /connect handoff origin.
func FrontendURL() string { return strings.TrimRight(frontendURL, "/") }

// APIURL is the read-only Data Layer GraphQL endpoint.
func APIURL() string { return apiURL }

// Credentials is the on-disk credential file format.
type Credentials struct {
	Token      string `json:"token"`
	ObtainedAt string `json:"obtainedAt,omitempty"`
}

// Dir returns the directory holding the credentials file. XDG_CONFIG_HOME is
// respected when set, else ~/.config (which resolves to C:\Users\<name>\.config
// on Windows). This only affects the local file location, never where the token
// is sent.
func Dir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Fall back to the working directory rather than panicking; the
			// subsequent write will surface a clear error if this is wrong.
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "ecombrain")
}

// CredentialsPath returns the full path to credentials.json.
func CredentialsPath() string { return filepath.Join(Dir(), "credentials.json") }

// hardenPermissions restricts a file or directory to the current user.
//
//	POSIX  : chmod to the given mode (already applied at create time).
//	Windows: mode bits are ignored by NTFS, so reset inherited ACLs and grant
//	         full control to only the current user via icacls.
func hardenPermissions(path string, mode os.FileMode, isDir bool) {
	if runtime.GOOS == "windows" {
		user := os.Getenv("USERNAME")
		if user == "" {
			return // best effort
		}
		grant := user + ":F"
		if isDir {
			grant = user + ":(OI)(CI)F"
		}
		// Best effort — icacls may be unavailable in constrained environments.
		_ = exec.Command("icacls", path, "/inheritance:r", "/grant:r", grant, "/Q").Run()
		return
	}
	_ = os.Chmod(path, mode) // best effort — e.g. non-POSIX filesystem
}

// ReadCredentials loads the stored credentials, or nil when absent/unreadable.
func ReadCredentials() *Credentials {
	raw, err := os.ReadFile(CredentialsPath())
	if err != nil {
		return nil
	}
	var creds Credentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil
	}
	return &creds
}

// WriteCredentials persists the credentials with owner-only permissions and
// returns the path written.
func WriteCredentials(creds Credentials) (string, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	hardenPermissions(dir, 0o700, true)

	raw, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')

	path := CredentialsPath()
	// Create with restrictive permissions from the start on POSIX, then harden
	// for the current platform.
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", err
	}
	hardenPermissions(path, 0o600, false)
	return path, nil
}

// Token returns the stored bearer token, or "" when none is present.
func Token() string {
	creds := ReadCredentials()
	if creds == nil {
		return ""
	}
	return creds.Token
}

// HasToken reports whether a token is stored.
func HasToken() bool { return Token() != "" }
