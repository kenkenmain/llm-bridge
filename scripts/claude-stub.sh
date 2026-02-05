#!/bin/bash
# Claude CLI stub for testing
# Echoes input back with a prefix, simulating LLM response
# Requires tmux to be available (for session-based testing)

set -e

# Check for tmux if running in session mode
if [[ "$1" == "--resume" ]] && ! command -v tmux &> /dev/null; then
    echo "Error: tmux is required for session mode" >&2
    exit 1
fi

# Simple echo-based stub: read input, echo response
while IFS= read -r line; do
    # Skip empty lines
    [[ -z "$line" ]] && continue
    
    # Simulate thinking delay
    sleep 0.1
    
    # Echo back with stub prefix
    echo "[STUB] Received: $line"
    echo "[STUB] This is a test response from claude-stub"
done
