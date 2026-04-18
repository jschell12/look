---
name: xmuggle
description: Analyze screenshot(s) to identify bugs or UI issues and fix the code
---

# /xmuggle (Cursor Command)

Screenshots are auto-detected from ~/Desktop.

## Agent workflow

1. List available images (JSON):

```bash
xmuggle list --json
```

Returns a JSON array sorted by date (newest first) with `name`, `path`, `status`, `mod_time`, `mod_time_unix` fields.

2. Send a fix:

```bash
# Latest screenshot, local
xmuggle send --repo <repo> --msg "<message>"

# Specific images by name (fuzzy match, repeatable)
xmuggle send --repo <repo> --img "<name>" [--img "<name2>"] --msg "<message>"

# All unprocessed
xmuggle send --repo <repo> --all --msg "<message>"

# Forward to another Mac on the LAN
xmuggle send --repo <repo> --remote --msg "<message>"

# Forward via encrypted git transport
xmuggle send --repo <repo> --remote --git --msg "<message>"
```

Always use `--img` for explicit image selection. Never use `--screenshots` (interactive picker, human-only).

## First-time pairing (AI-assisted)

When setting up `--remote --git` on a new machine, use `--json` to fetch available peers and ask the user conversationally which to pair with, then re-run with `--peer`:

```bash
# Step 1: base setup + list peers (JSON — no prompt)
xmuggle init-send <owner/repo> --json
# or for the receiver side:
xmuggle init-recv <owner/repo> --json

# Step 2: after user picks one
xmuggle init-send <owner/repo> --peer <chosen-host>
```
