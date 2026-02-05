# Security Design Decisions

## Authorization

llm-bridge does not implement user authorization for Discord commands like `/clone`, `/add-worktree`, and `/remove-repo`. This is intentional:

- **Internal tool**: llm-bridge is designed for small teams where all Discord channel members are trusted
- **Channel-based isolation**: Each repo is bound to a specific channel; users only interact with repos in channels they have access to
- **Discord handles access control**: Channel permissions in Discord control who can send messages

If you need finer-grained access control, implement it at the Discord channel permission level.

## Git URL Schemes

llm-bridge allows these URL schemes for `/clone`:
- `https://` - HTTPS transport
- `git://` - Git protocol (read-only, no auth)
- `ssh://` - SSH transport
- `git@` - SSH shorthand (e.g., `git@github.com:user/repo`)

### Why git@ is allowed

The `git@` scheme is required for cloning private repositories using SSH keys. Without it, users would need to configure personal access tokens in URLs, which is less secure and more cumbersome.

### SSRF Considerations

The `git@` scheme could theoretically be used for SSRF attacks against internal services. This risk is accepted because:

1. The attack surface is limited to git operations (not arbitrary network requests)
2. Users with `/clone` access are already trusted (see Authorization above)
3. Blocking `git@` would break the primary use case of cloning private repos

If your threat model includes malicious insiders, deploy llm-bridge in a network segment without access to sensitive internal services.

## File Permissions

- Config files are written with mode `0600` (owner read/write only)
- Working directories are controlled by the user who runs llm-bridge
