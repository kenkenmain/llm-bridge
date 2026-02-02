package router

import (
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
		{"select", "/select", "select", RouteToBridge},
		{"sessions", "/sessions", "sessions", RouteToBridge},
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
		{"plain text", "refactor the auth module", "refactor the auth module"},
		{"whitespace trimmed", "  hello world  ", "hello world"},
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

func TestParse_ExclamationTranslation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantRaw string
	}{
		{"!commit to /commit", "!commit", "/commit"},
		{"!review-pr with args", "!review-pr 123", "/review-pr 123"},
		{"!help to /help", "!help", "/help"},
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

func TestParse_BridgeCommandArgs(t *testing.T) {
	route := Parse("/select notification-hooks")
	if route.Type != RouteToBridge {
		t.Errorf("Type = %v, want RouteToBridge", route.Type)
	}
	if route.Command != "select" {
		t.Errorf("Command = %q, want %q", route.Command, "select")
	}
	if route.Args != "notification-hooks" {
		t.Errorf("Args = %q, want %q", route.Args, "notification-hooks")
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
		{"select repo1 extra args", "select", "repo1 extra args"},
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
