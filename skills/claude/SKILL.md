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

2. **List available images** (machine-readable):

```bash
xmuggle list --json
```

Returns a JSON array of images sorted by date (newest first):

```json
[
  {
    "name": "Screenshot 2026-04-18 at 10.30.00 AM.png",
    "path": "/Users/you/.xmuggle/Screenshot 2026-04-18 at 10.30.00 AM.png",
    "status": "pending",
    "mod_time": "2026-04-18T10:30:00-07:00",
    "mod_time_unix": 1776451800
  }
]
```

3. **Send the fix** (non-interactive — use `--img` for specific images):

```bash
# Latest unprocessed screenshot (default)
xmuggle send --repo <repo> --msg "<message>"

# Specific image(s) by name (fuzzy match)
xmuggle send --repo <repo> --img "<name>" [--img "<name2>"] --msg "<message>"

# All unprocessed
xmuggle send --repo <repo> --all --msg "<message>"

# Forward to another Mac on the LAN
xmuggle send --repo <repo> --remote --msg "<message>"

# Forward to a specific host
xmuggle send --repo <repo> --remote --host mac.local --msg "<message>"

# Forward via encrypted git transport
xmuggle send --repo <repo> --remote --git --msg "<message>"
```

4. **Report the result** to the user — mention what was fixed and that they can `git pull` to get changes.

## Agent workflow (recommended)

When acting on behalf of the user, always use non-interactive commands:

1. `xmuggle list --json` — get available images as JSON
2. Pick the right image(s) based on user intent (latest, specific name, etc.)
3. `xmuggle send --repo <repo> --img "<name>" --msg "<context>"` — send with explicit `--img`

Never use `--screenshots` (that flag opens an interactive picker for human users).

## Flags reference

| Flag | Purpose |
|---|---|
| `--repo <repo>` | Target repo (required) |
| `--msg "<text>"` | Context for the agent |
| `--img "<name>"` | Select specific image (repeatable, fuzzy matches) |
| `--all` | Process all unprocessed images |
| `--screenshots` | Interactive picker (human use only, not for agents) |
| `--remote` | Forward to another Mac via SSH (discovers via Bonjour if no --host) |
| `--host <host>` | Specific remote hostname (with --remote) |
| `--user <user>` | SSH user on remote (with --remote) |
| `--git` | Forward via age-encrypted GitHub queue repo (with --remote) |
| `--to <host>` | Recipient hostname (with --remote --git) |
| `--scan` | Ingest ALL images from ~/Desktop |

## Prerequisites

- `xmuggle` CLI on PATH (install: `make install` in the xmuggle repo)
- `claude` and `gh` CLIs on PATH
- For `--remote` (SSH): SSH enabled on the target Mac, daemon running (`make daemon-install`)
- For `--remote --git`: both machines set up with roles:
  - receiver: `xmuggle init-recv <owner/repo>`  (installs + starts the daemon)
  - sender:   `xmuggle init-send <owner/repo>`  (then: `xmuggle add-recipient <receiver-host> --default`)

## Pairing with a peer (AI-assisted)

When a user asks you to set up xmuggle on a new machine, use the `--json` and `--peer` flags to discover and select a peer conversationally instead of relying on interactive stdin prompts.

### Step 1: run init with `--json` to fetch available peers

```bash
xmuggle init-send <owner/repo> --json   # lists receivers (what to SEND to)
# or
xmuggle init-recv <owner/repo> --json   # lists senders (for local pubkey cache)
```

The command performs all base setup (queue clone, age keypair, role marker, daemon install for init-recv) and then emits JSON like:

```json
{
  "action": "select-peer",
  "role": "receiver",
  "peers": ["joshs-macbook-pro", "work-mbp"],
  "queue_repo": "jschell12/xmuggle-queue",
  "hint": "Re-run with --peer <hostname> to proceed."
}
```

If `"action": "no-peers"`, stop — the base setup is complete and no peers exist yet. Tell the user.

### Step 2: ask the user conversationally

Parse `.peers[]` and present them to the user. For example:

> I've set up this machine. There are 2 registered receivers in your queue repo:
> 1. joshs-macbook-pro
> 2. work-mbp
>
> Which should be the default recipient for your sends?

### Step 3: re-run with `--peer <choice>`

```bash
xmuggle init-send <owner/repo> --peer joshs-macbook-pro
```

This caches the peer's pubkey locally. For `init-send`, it also sets `default_recipient`. Idempotent — safe to re-run.

If the user picks "none / skip", do nothing further. They can pair later with `xmuggle add-recipient <host> --default`.
