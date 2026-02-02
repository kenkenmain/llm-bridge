package llm

import (
	"regexp"
	"strings"
)

// SessionPrefix is the prefix added to all tmux session names managed by llm-bridge.
const SessionPrefix = "llm-bridge-"

// nonAlphanumericOrHyphen matches any character that is not alphanumeric or a hyphen.
var nonAlphanumericOrHyphen = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// consecutiveHyphens matches two or more consecutive hyphens.
var consecutiveHyphens = regexp.MustCompile(`-{2,}`)

// SanitizeSessionName converts a repo name into a valid tmux session name.
// It replaces non-alphanumeric characters (except hyphens) with hyphens,
// adds the "llm-bridge-" prefix, collapses consecutive hyphens, and trims
// trailing hyphens.
func SanitizeSessionName(repoName string) string {
	if repoName == "" {
		return SessionPrefix
	}

	// Replace non-alphanumeric characters (except hyphens) with hyphens
	sanitized := nonAlphanumericOrHyphen.ReplaceAllString(repoName, "-")

	// Collapse consecutive hyphens
	sanitized = consecutiveHyphens.ReplaceAllString(sanitized, "-")

	// Trim leading and trailing hyphens from the repo part
	sanitized = strings.Trim(sanitized, "-")

	return SessionPrefix + sanitized
}

// ParseSessionName strips the "llm-bridge-" prefix from a session name and
// returns the original repo name portion. Returns an empty string if the
// prefix is not found.
func ParseSessionName(sessionName string) string {
	if !strings.HasPrefix(sessionName, SessionPrefix) {
		return ""
	}
	return strings.TrimPrefix(sessionName, SessionPrefix)
}
