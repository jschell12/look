---
name: xmuggle
description: Analyze screenshot(s) to identify bugs or UI issues and fix the code
---

# /xmuggle (Cursor Command)

Screenshots are auto-detected from ~/Desktop.

## Usage

```bash
# Latest screenshot, local
xmuggle --repo <repo> --msg "<message>"

# Specific images
xmuggle --repo <repo> --img "<name>" --msg "<message>"

# All unprocessed
xmuggle --repo <repo> --all --msg "<message>"

# Forward to another Mac on the LAN
xmuggle --repo <repo> --remote --msg "<message>"
```
