---
name: screenshot-fix
description: Send a screenshot to your personal machine for AI-driven code fixing
---

# Screenshot Fix (Cursor Command)

Submit a screenshot-driven code fix to your personal machine.

## Usage

When invoked, gather:
1. **Screenshot**: path to the image file
2. **Repo**: GitHub repo (owner/name) or local path
3. **Message** (optional): what to fix

Run in the terminal:

```bash
screenshot-agent <screenshot> --repo <repo> --remote --msg "<message>"
```

The agent on your personal machine will analyze the screenshot, fix the code, create a PR, and merge it. Pull latest when done.
