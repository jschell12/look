#!/usr/bin/env bash
# Tail daemon log and auto-tail new claude session logs with filtering.
# Usage: scripts/tail-all.sh

XMUGGLE_DIR="$HOME/.xmuggle"
DAEMON_LOG="$XMUGGLE_DIR/daemon.log"
FILTER="$(dirname "$0")/claude-log-filter.py"
TAILING_PIDS=()
SEEN_LOGS=()

cleanup() {
  for pid in "${TAILING_PIDS[@]}"; do
    kill "$pid" 2>/dev/null
  done
  exit 0
}
trap cleanup INT TERM

tail_claude_log() {
  local logfile="$1"
  local taskid
  taskid=$(basename "$logfile" .log | sed 's/^claude-//')
  echo -e "\033[36m── claude [$taskid] ──\033[0m"
  tail -f "$logfile" | python3 "$FILTER" | sed "s/^/  [$taskid] /" &
  TAILING_PIDS+=($!)
}

# Tail any existing claude logs that are still being written to
for f in "$XMUGGLE_DIR"/claude-*.log; do
  [ -f "$f" ] || continue
  # Only tail if modified in the last 60 seconds (likely active)
  if [ "$(find "$f" -mmin -1 2>/dev/null)" ]; then
    SEEN_LOGS+=("$f")
    tail_claude_log "$f"
  fi
done

# Watch daemon log and auto-tail new claude logs as they appear
tail -f "$DAEMON_LOG" 2>/dev/null | while IFS= read -r line; do
  echo "$line"

  # Detect "Tail live:" lines that reference new claude logs
  if [[ "$line" == *"Tail live: tail -f"* ]]; then
    logpath=$(echo "$line" | grep -o '/[^ ]*claude-[^ ]*\.log')
    if [ -n "$logpath" ] && [ -f "$logpath" ]; then
      # Check if we're already tailing this one
      already=false
      for seen in "${SEEN_LOGS[@]}"; do
        if [ "$seen" = "$logpath" ]; then
          already=true
          break
        fi
      done
      if ! $already; then
        SEEN_LOGS+=("$logpath")
        tail_claude_log "$logpath"
      fi
    fi
  fi
done
