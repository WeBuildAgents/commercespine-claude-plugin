// Package config holds the shared configuration and credential storage for the
// Commerce Spine plugin. Standard library only — no external dependencies.
package config

import (
	"encoding/json"
	"fmt"
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
	frontendURL = "https://console.commercespine.com"
	apiURL      = "https://api.commercespine.com/graphql"
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

// dir returns the directory holding the credentials file. XDG_CONFIG_HOME is
// respected when set, else ~/.config (which resolves to C:\Users\<name>\.config
// on Windows). This only affects the local file location, never where the token
// is sent.
//
// There is deliberately no fallback to the working directory: silently writing a
// bearer token into whatever directory the command happened to run from is worse
// than refusing, and in containers or CI (where HOME is often unset) that
// directory is frequently a checked-out repository.
func dir() (string, error) {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "commercespine"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine your home directory; set XDG_CONFIG_HOME to choose where the Commerce Spine token is stored: %w", err)
	}
	return filepath.Join(home, ".config", "commercespine"), nil
}

// CredentialsPath returns the full path to credentials.json.
func CredentialsPath() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "credentials.json"), nil
}

// hardenPermissions restricts a file or directory to the current user.
//
//	POSIX  : chmod to the given mode (already applied at create time).
//	Windows: mode bits are ignored by NTFS, so reset inherited ACLs and grant
//	         full control to only the current user via icacls.
//
// Windows hardening is best effort: it is skipped when USERNAME is unset, and
// icacls failures are ignored. The README documents this asymmetry.
func hardenPermissions(path string, mode os.FileMode, isDir bool) {
	if runtime.GOOS == "windows" {
		user := os.Getenv("USERNAME")
		if user == "" {
			return
		}
		grant := user + ":F"
		if isDir {
			grant = user + ":(OI)(CI)F"
		}
		_ = exec.Command("icacls", path, "/inheritance:r", "/grant:r", grant, "/Q").Run()
		return
	}
	_ = os.Chmod(path, mode) // best effort — e.g. non-POSIX filesystem
}

// ReadCredentials loads the stored credentials. It returns (nil, nil) when no
// credentials file exists yet; an error means the location could not be resolved
// or the file could not be parsed.
func ReadCredentials() (*Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}
	var creds Credentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", path, err)
	}
	return &creds, nil
}

// WriteCredentials persists the credentials with owner-only permissions and
// returns the path written.
func WriteCredentials(creds Credentials) (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	hardenPermissions(d, 0o700, true)

	raw, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')

	path := filepath.Join(d, "credentials.json")
	// Create with restrictive permissions from the start on POSIX, then harden
	// for the current platform.
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", err
	}
	hardenPermissions(path, 0o600, false)
	return path, nil
}

// Token returns the stored bearer token. An empty string with a nil error means
// no token is stored yet — that is an authentication problem. A non-nil error
// means the token could not be looked up at all, which is not.
func Token() (string, error) {
	creds, err := ReadCredentials()
	if err != nil {
		return "", err
	}
	if creds == nil {
		return "", nil
	}
	return creds.Token, nil
}

// HasToken reports whether a token is stored.
func HasToken() (bool, error) {
	token, err := Token()
	if err != nil {
		return false, err
	}
	return token != "", nil
}
