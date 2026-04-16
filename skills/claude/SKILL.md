---
name: screenshot-fix
description: >-
  Submit a screenshot-driven code fix to a remote machine for processing.
  Use when the user wants to fix a bug or UI issue shown in a screenshot
  by sending it to their personal machine's agent queue.
---

# Screenshot Fix

Send a screenshot to your personal machine where a Claude Code agent will analyze it, fix the code, push a branch, create a PR, and merge it.

## When to trigger

- User says "fix this screenshot", "screenshot fix", "send this to my machine"
- User provides a screenshot and mentions a repo that needs changes
- User invokes `/screenshot-fix`

## Steps

1. Gather the required information:
   - **Screenshot path**: The image file to analyze. If the user references a screenshot on their Desktop, find it (e.g., the latest `Screenshot*.png` on `~/Desktop`). If they pasted an image, save it to `/tmp/screenshot-fix-<timestamp>.png` first.
   - **Repo**: GitHub repo (`owner/name` or URL) or local path. Ask if not provided.
   - **Message** (optional): Additional context about what to fix.

2. Run the CLI:

```bash
screenshot-agent <screenshot-path> --repo <repo> --remote --msg "<message>"
```

3. The tool will:
   - Send the screenshot + task to your personal machine via SSH
   - Wait for the agent to process it (this may take a few minutes)
   - Report back with the PR URL and status
   - Auto-pull if the repo is a local path

4. Report the result to the user. If successful, mention the PR URL and that they can `git pull` to get the changes.

## Prerequisites

- `screenshot-agent` must be installed (`make i-wm` from the screenshot-agent repo)
- SSH connection to personal machine must be configured (`make setup`)
- Daemon must be running on personal machine (`make daemon-start`)
