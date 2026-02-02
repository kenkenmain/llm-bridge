package router

import (
	"strings"
)

type RouteType int

const (
	RouteToLLM RouteType = iota
	RouteToBridge
)

const LLMCommandPrefix = "::"

type Route struct {
	Type    RouteType
	Command string
	Args    string
	Raw     string
}

var BridgeCommands = map[string]bool{
	"status":  true,
	"cancel":  true,
	"restart": true,
	"help":    true,
	"select":  true,
}

func Parse(content string) Route {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "/") {
		cmd, args := parseCommand(content[1:])
		if BridgeCommands[cmd] {
			return Route{
				Type:    RouteToBridge,
				Command: cmd,
				Args:    args,
				Raw:     content,
			}
		}
		return Route{
			Type: RouteToLLM,
			Raw:  content,
		}
	}

	if strings.HasPrefix(content, LLMCommandPrefix) {
		translated := "/" + strings.TrimPrefix(content, LLMCommandPrefix)
		return Route{
			Type: RouteToLLM,
			Raw:  translated,
		}
	}

	return Route{
		Type: RouteToLLM,
		Raw:  content,
	}
}

func parseCommand(s string) (cmd, args string) {
	parts := strings.SplitN(s, " ", 2)
	cmd = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = parts[1]
	}
	return
}
