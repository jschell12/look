---
name: xmuggle
description: >-
  Analyze screenshot(s) to identify bugs or UI issues and fix the code.
  Auto-detects new screenshots from ~/Desktop. Works locally
  or forwards to a discovered Mac on the LAN.
---

# /xmuggle

Analyze screenshot(s), identify the problem, and fix the code. New screenshots are auto-detected from your ~/Desktop via macOS Spotlight — just take a screenshot and invoke `/xmuggle`.

## When to trigger

- User invokes `/xmuggle`
- User says "look at this", "fix this screenshot", "check my screenshot"
- User provides a screenshot and mentions a repo that needs changes
- User drops an image and describes a bug or desired change

## Steps

1. **Gather information**:
   - **Repo** (required): GitHub repo (`owner/name` or URL) or local path
   - **Message** (optional): What's wrong, what to fix
   - **Image selection** (optional): Specific image name(s), `--all` for all unprocessed, or omit for latest

2. **See what's available**:

```bash
xmuggle --list
```

3. **Run the fix**:

```bash
# Process locally (default)
xmuggle --repo <repo> --msg "<message>"

# Specific / multiple images
xmuggle --repo <repo> --img "<name>" [--img "<name2>"] --msg "<message>"

# All unprocessed
xmuggle --repo <repo> --all --msg "<message>"

# Forward to another Mac on the LAN (interactive host discovery)
xmuggle --repo <repo> --remote --msg "<message>"

# Forward to a specific host
xmuggle --repo <repo> --remote --host mac.local --msg "<message>"
```

4. **Report the result** to the user — mention what was fixed and that they can `git pull` to get changes.

## Flags reference

| Flag | Purpose |
|---|---|
| `--repo <repo>` | Target repo (required) |
| `--msg "<text>"` | Context for the agent |
| `--img "<name>"` | Select specific image (repeatable, fuzzy matches) |
| `--all` | Process all unprocessed images |
| `--remote` | Forward to another Mac via SSH (discovers via Bonjour if no --host) |
| `--host <host>` | Specific remote hostname (with --remote) |
| `--user <user>` | SSH user on remote (with --remote) |
| `--git` | Forward via age-encrypted GitHub queue repo (with --remote) |
| `--to <host>` | Recipient hostname (with --remote --git) |
| `--list` | Show all images and status |
| `--scan` | Ingest ALL images from ~/Desktop |

## Prerequisites

- `xmuggle` CLI on PATH (install: `make install` in the xmuggle repo)
- `claude` and `gh` CLIs on PATH
- For `--remote`: SSH enabled on the target Mac, daemon running (`make daemon-install`)
