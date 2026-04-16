# xmuggle

Screenshot-driven code fixes. Take a screenshot, run `xmuggle`, and a Claude Code agent analyzes the image, finds the relevant code, fixes it, opens a PR, and merges it.

Written in Go. Single static binary. age encryption for git-based remote transport.

## Install

```bash
git clone git@github.com:jschell12/xmuggle.git
cd xmuggle
make install    # builds, installs to /usr/local/bin, adds /xmuggle skill
```

## Usage

```bash
# Fix the latest unprocessed screenshot (auto-detected from ~/Desktop)
xmuggle --repo jschell12/my-app

# With context
xmuggle --repo jschell12/my-app --msg "the submit button overlaps the footer"

# Specific image (fuzzy name match)
xmuggle --repo jschell12/my-app --img "Screenshot 2026-04-14"

# Multiple images as one task
xmuggle --repo jschell12/my-app --img bug1 --img bug2 --msg "same issue, different pages"

# Process all unprocessed screenshots
xmuggle --repo jschell12/my-app --all

# List images + status
xmuggle --list
```

## Remote processing

Forward a task to another Mac when the local machine can't run the agent.

### SSH/rsync (same LAN)

```bash
# Interactive: Bonjour discovers Macs advertising SSH
xmuggle --repo jschell12/my-app --remote

# Or specify directly
xmuggle --repo jschell12/my-app --remote --host macmini.local
```

Target Mac runs the daemon: `make daemon-install`.

### Encrypted git queue (works through VPN/firewall)

Uses age encryption and a private GitHub repo as the transport.

**One-time setup (both laptops):**
```bash
xmuggle queue-init jschell12/xmuggle-queue   # register private queue repo
xmuggle init-keys                          # generate age keypair, publish pubkey
```

**On the sender:** once the receiver has published their pubkey:
```bash
xmuggle add-recipient home-mbp --default   # fetches pubkey from queue repo
```

**Send:**
```bash
xmuggle --repo jschell12/my-app --remote --git --msg "fix this"
```

The task is age-encrypted to the receiver's pubkey, committed to the queue repo, and the receiver's daemon picks it up, processes it, and encrypts the result back.

## Image detection

Screenshots are auto-detected from `~/Desktop` via macOS Spotlight (`kMDItemIsScreenCapture`) and copied into `~/.xmuggle/` on every run.

- `~/.xmuggle/.tracked` — processed filenames
- `~/.xmuggle/.seen` — source paths we've ingested (prevents re-copying)
- `--scan` — ingest ALL images (not just screenshots)

## Architecture

```
xmuggle (CLI)                      xmuggled (daemon)
────────────                    ────────────
Local mode:                     Watches ~/.xmuggle/queue/ every 5s
  spawn claude + prompt         Enqueues tasks to agent-queue
                                Spawns workers (up to MAX_WORKERS)
Remote (SSH):                   Workers: claim → fix → agent-merge → complete
  rsync task → polling
                                Git sync (if configured):
Remote (git):                     Pulls queue repo every N seconds
  age-encrypt + commit + push     Decrypts new tasks addressed to us
  poll for encrypted result       Encrypts + pushes results back
```

## Commands

| Command | Purpose |
|---|---|
| `make install` | Build, install binaries, install /xmuggle skill |
| `make daemon-install` | Install queue-processing daemon (launchd) |
| `make daemon-start / -stop / -logs` | Control the daemon |
| `make daemon-uninstall` | Remove the daemon |
| `make link` | Interactive LAN discovery (mac-link.sh) |

## Requirements

- macOS (uses `mdfind`, `dns-sd`)
- Go 1.26+ (to build)
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code)
- [GitHub CLI](https://cli.github.com/) (`gh`) authenticated
- [agent-queue](https://github.com/jschell12/agent-queue) (for daemon)
- `git`, `rsync`, `ssh` (stdlib on macOS)

No dependency on `age` CLI — the age protocol is embedded in the binary via `filippo.io/age`.
