// Command commercespine provides the Commerce Spine Claude plugin's local commands.
//
// It is a single binary with three subcommands. The bin/ shims invoke it as
// `commercespine login`, `commercespine gql`, and `commercespine config`.
//
// Exit codes:
//
//	0  success
//	2  authentication problem (no token, or one the API rejected) — and only
//	   this: exit 2 is the signal that signing in again will help
//	1  any other failure, including one that merely prevented verification
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/WeBuildAgents/commercespine-claude-plugin/internal/config"
	"github.com/WeBuildAgents/commercespine-claude-plugin/internal/graphql"
	"github.com/WeBuildAgents/commercespine-claude-plugin/internal/login"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: commercespine <login|gql|config|version> [options]")
		os.Exit(1)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "login":
		os.Exit(login.Run(args))
	case "gql":
		os.Exit(runGQL(args))
	case "config":
		os.Exit(runConfig(args))
	case "version":
		fmt.Println(version)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Usage: commercespine <login|gql|config|version> [options]\n")
		os.Exit(1)
	}
}

// version is stamped at build time with -ldflags "-X main.version=…".
var version = "dev"

func runGQL(args []string) int {
	var query, variablesRaw string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--query", "-q":
			if i+1 < len(args) {
				i++
				query = args[i]
			}
		case "--variables", "-v":
			if i+1 < len(args) {
				i++
				variablesRaw = args[i]
			}
		case "--help", "-h":
			fmt.Println("Usage: commercespine-gql --query '<gql>' [--variables '<json>']")
			fmt.Println("       echo '<gql>' | commercespine-gql")
			return 0
		}
	}

	if strings.TrimSpace(query) == "" {
		// No --query: read the query from stdin when it is piped.
		if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			raw, err := io.ReadAll(os.Stdin)
			if err == nil {
				query = strings.TrimSpace(string(raw))
			}
		}
	}
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(os.Stderr, "Error: no query provided (use --query or stdin).")
		return 1
	}

	variables := map[string]any{}
	if variablesRaw != "" {
		if err := json.Unmarshal([]byte(variablesRaw), &variables); err != nil {
			fmt.Fprintf(os.Stderr, "Error: --variables is not valid JSON: %v\n", err)
			return 1
		}
	}

	data, err := graphql.Execute(query, variables)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		var authErr *graphql.AuthError
		if errors.As(err, &authErr) {
			return 2
		}
		return 1
	}

	var pretty any
	if err := json.Unmarshal(data, &pretty); err != nil {
		fmt.Println(string(data))
		return 0
	}
	out, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		fmt.Println(string(data))
		return 0
	}
	fmt.Println(string(out))
	return 0
}

func maskToken(creds *config.Credentials) string {
	if creds == nil || creds.Token == "" {
		return ""
	}
	t := creds.Token
	if len(t) <= 8 {
		return "****"
	}
	return fmt.Sprintf("%s…%s (%d chars)", t[:4], t[len(t)-4:], len(t))
}

func runConfig(args []string) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
		if a == "--help" || a == "-h" {
			fmt.Println("Usage: commercespine-config [--json]")
			return 0
		}
	}

	credsPath, err := config.CredentialsPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	creds, err := config.ReadCredentials()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	hasToken := creds != nil && creds.Token != ""
	obtainedAt := ""
	if creds != nil {
		obtainedAt = creds.ObtainedAt
	}

	if asJSON {
		status := map[string]any{
			"frontendUrl":     config.FrontendURL(),
			"apiUrl":          config.APIURL(),
			"credentialsPath": credsPath,
			"tokenPresent":    hasToken,
			"tokenPreview":    nilIfEmpty(maskToken(creds)),
			"obtainedAt":      nilIfEmpty(obtainedAt),
		}
		out, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not encode status: %v\n", err)
			return 1
		}
		fmt.Println(string(out))
		return 0
	}

	lines := []string{
		"Commerce Spine plugin configuration",
		"-----------------------------------",
		"Frontend URL:      " + config.FrontendURL(),
		"API URL:           " + config.APIURL(),
		"Credentials file:  " + credsPath,
	}
	if hasToken {
		lines = append(lines,
			"Token present:     yes",
			"Token:             "+maskToken(creds),
			"Obtained at:       "+orUnknown(obtainedAt),
		)
	} else {
		lines = append(lines,
			"Token present:     no",
			"Next step:         run /commercespine:login to authenticate.",
		)
	}
	fmt.Println(strings.Join(lines, "\n"))
	return 0
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
