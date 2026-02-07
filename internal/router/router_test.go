package router

import (
	"strings"
	"testing"
)

func TestParse_BridgeCommands(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantCmd string
		wantTyp RouteType
	}{
		{"status", "/status", "status", RouteToBridge},
		{"cancel", "/cancel", "cancel", RouteToBridge},
		{"restart", "/restart", "restart", RouteToBridge},
		{"help", "/help", "help", RouteToBridge},
		{"status with args", "/status repo1", "status", RouteToBridge},
		{"uppercase normalized", "/STATUS", "status", RouteToBridge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := Parse(tt.input)
			if route.Type != tt.wantTyp {
				t.Errorf("Parse(%q).Type = %v, want %v", tt.input, route.Type, tt.wantTyp)
			}
			if route.Command != tt.wantCmd {
				t.Errorf("Parse(%q).Command = %q, want %q", tt.input, route.Command, tt.wantCmd)
			}
		})
	}
}

func TestParse_LLMCommands(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantRaw string
	}{
		{"unknown slash", "/commit", "/commit"},
		{"unknown slash with args", "/review-pr 123", "/review-pr 123"},
		{"select now routes to llm", "/select notification-hooks", "/select notification-hooks"},
		{"plain text", "refactor the auth module", "refactor the auth module"},
		{"whitespace trimmed", "  hello world  ", "hello world"},
		{"old ! prefix is plain text", "!commit", "!commit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := Parse(tt.input)
			if route.Type != RouteToLLM {
				t.Errorf("Parse(%q).Type = %v, want RouteToLLM", tt.input, route.Type)
			}
			if route.Raw != tt.wantRaw {
				t.Errorf("Parse(%q).Raw = %q, want %q", tt.input, route.Raw, tt.wantRaw)
			}
		})
	}
}

func TestParse_DoubleColonTranslation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantRaw string
	}{
		{"::commit to /commit", "::commit", "/commit"},
		{"::review-pr with args", "::review-pr 123", "/review-pr 123"},
		{"::help to /help", "::help", "/help"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := Parse(tt.input)
			if route.Type != RouteToLLM {
				t.Errorf("Parse(%q).Type = %v, want RouteToLLM", tt.input, route.Type)
			}
			if route.Raw != tt.wantRaw {
				t.Errorf("Parse(%q).Raw = %q, want %q", tt.input, route.Raw, tt.wantRaw)
			}
		})
	}
}

func TestParse_Worktrees(t *testing.T) {
	route := Parse("/worktrees")
	if route.Type != RouteToBridge {
		t.Errorf("route type = %v, want RouteToBridge", route.Type)
	}
	if route.Command != "worktrees" {
		t.Errorf("route command = %q, want %q", route.Command, "worktrees")
	}
}

func TestParse_NewRepoManagementCommands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCmd  string
		wantArgs string
		wantType RouteType
	}{
		{"list-repos", "/list-repos", "list-repos", "", RouteToBridge},
		{"remove-repo", "/remove-repo myrepo", "remove-repo", "myrepo", RouteToBridge},
		{"clone", "/clone https://github.com/user/repo myrepo", "clone", "https://github.com/user/repo myrepo", RouteToBridge},
		{"clone with channel", "/clone https://github.com/user/repo myrepo 12345", "clone", "https://github.com/user/repo myrepo 12345", RouteToBridge},
		{"add-worktree", "/add-worktree feature feature-branch", "add-worktree", "feature feature-branch", RouteToBridge},
		{"add-worktree with channel", "/add-worktree feature feature-branch 12345", "add-worktree", "feature feature-branch 12345", RouteToBridge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := Parse(tt.input)
			if route.Type != tt.wantType {
				t.Errorf("Parse(%q).Type = %v, want %v", tt.input, route.Type, tt.wantType)
			}
			if route.Command != tt.wantCmd {
				t.Errorf("Parse(%q).Command = %q, want %q", tt.input, route.Command, tt.wantCmd)
			}
			if route.Args != tt.wantArgs {
				t.Errorf("Parse(%q).Args = %q, want %q", tt.input, route.Args, tt.wantArgs)
			}
		})
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantArg string
	}{
		{"status", "status", ""},
		{"status repo1", "status", "repo1"},
		{"help repo1 extra args", "help", "repo1 extra args"},
		{"STATUS", "status", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, args := parseCommand(tt.input)
			if cmd != tt.wantCmd {
				t.Errorf("parseCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.wantCmd)
			}
			if args != tt.wantArg {
				t.Errorf("parseCommand(%q) args = %q, want %q", tt.input, args, tt.wantArg)
			}
		})
	}
}

func TestParse_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTyp RouteType
		wantRaw string
	}{
		{"empty string", "", RouteToLLM, ""},
		{"whitespace only", "   \t\n  ", RouteToLLM, ""},
		{"single slash", "/", RouteToLLM, "/"},
		{"double colon only", "::", RouteToLLM, "/"},
		{"unicode emoji", "🚀 deploy", RouteToLLM, "🚀 deploy"},
		{"unicode RTL", "مرحبا", RouteToLLM, "مرحبا"},
		{"null byte in middle", "hello\x00world", RouteToLLM, "hello\x00world"},
		{"very long input", strings.Repeat("a", 10240), RouteToLLM, strings.Repeat("a", 10240)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := Parse(tt.input)
			if route.Type != tt.wantTyp {
				t.Errorf("Parse(%q).Type = %v, want %v", tt.input, route.Type, tt.wantTyp)
			}
			if route.Raw != tt.wantRaw {
				t.Errorf("Parse(%q).Raw = %q, want %q", tt.input, route.Raw, tt.wantRaw)
			}
		})
	}
}
