#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

is_on_path() {
  case ":$PATH:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

# Position of $1 within colon-separated PATH, or 99 if missing.
path_index() {
  local i=0
  local IFS=:
  for d in $PATH; do
    if [[ "$d" == "$1" ]]; then
      echo "$i"
      return
    fi
    i=$((i + 1))
  done
  echo 99
}

pick_prefix() {
  # Explicit override wins.
  if [[ -n "${INSTALL_PREFIX:-}" ]]; then
    echo "$INSTALL_PREFIX"
    return
  fi

  # Candidates in preference order. We only pick one that's ALREADY on PATH
  # AND writable — this prevents the binary from being shadowed by macOS's
  # built-in /usr/bin/look (found after /usr/bin on PATH).
  local candidates=(
    "$HOME/.local/bin"
    "$HOME/bin"
    "/opt/homebrew/bin"
    "/usr/local/bin"
  )

  local usr_bin_idx
  usr_bin_idx=$(path_index "/usr/bin")

  for dir in "${candidates[@]}"; do
    if ! is_on_path "$dir"; then
      continue
    fi
    # Must come BEFORE /usr/bin so we win over the system `look` utility.
    local idx
    idx=$(path_index "$dir")
    if (( idx >= usr_bin_idx )); then
      continue
    fi
    if [[ -d "$dir" && -w "$dir" ]] || \
       [[ ! -e "$dir" && -w "$(dirname "$dir")" ]]; then
      mkdir -p "$dir"
      echo "$dir"
      return
    fi
  done

  # Nothing on PATH is suitable. Pick ~/.local/bin and warn later.
  mkdir -p "$HOME/.local/bin"
  echo "$HOME/.local/bin"
}

BIN_DIR="$(pick_prefix)"

echo "=== Installing /look skill + CLI ==="
echo "Install dir: $BIN_DIR"
echo ""

# Build
echo "Building..."
(cd "$REPO_DIR" && make build)

# Install binaries
for bin in look lookd; do
  src="$REPO_DIR/bin/$bin"
  dst="$BIN_DIR/$bin"
  if [[ -w "$BIN_DIR" ]]; then
    install -m 0755 "$src" "$dst"
  else
    echo "Installing $bin (needs sudo)..."
    sudo install -m 0755 "$src" "$dst"
  fi
  echo "  $dst"
done

# PATH diagnostics
if ! is_on_path "$BIN_DIR"; then
  echo ""
  echo "WARNING: $BIN_DIR is not on your PATH."
  echo "Add this to your shell rc (~/.zshrc, ~/.bashrc, etc.):"
  echo ""
  echo "  export PATH=\"$BIN_DIR:\$PATH\""
  echo ""
  echo "Then restart your shell or run: source ~/.zshrc"
fi

# Shadow-by-/usr/bin/look check
ACTUAL="$(command -v look || true)"
if [[ "$ACTUAL" != "$BIN_DIR/look" ]]; then
  echo ""
  echo "WARNING: 'look' resolves to $ACTUAL (not $BIN_DIR/look)"
  if [[ "$ACTUAL" == "/usr/bin/look" ]]; then
    echo "That's the macOS built-in 'look' utility, shadowing ours."
    echo "Fix: put $BIN_DIR earlier on PATH than /usr/bin."
  fi
fi

# Claude Code skill
if [[ -d "$HOME/.claude" ]]; then
  mkdir -p "$HOME/.claude/skills/look"
  cp "$REPO_DIR/skills/claude/SKILL.md" "$HOME/.claude/skills/look/SKILL.md"
  echo "  ~/.claude/skills/look/SKILL.md"
fi

# Cursor command
if [[ -d "$HOME/.cursor" ]]; then
  mkdir -p "$HOME/.cursor/commands"
  cp "$REPO_DIR/skills/cursor/command.md" "$HOME/.cursor/commands/look.md"
  echo "  ~/.cursor/commands/look.md"
fi

echo ""
echo "Done! Use /look in Claude Code or Cursor."
echo ""
echo "Quick start:"
echo "  look --list"
echo "  look --repo jschell12/my-app"
