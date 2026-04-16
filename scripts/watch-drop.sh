#!/usr/bin/env bash
#
# Triggered by launchd WatchPaths when files land in the drop directories.
# Scans ~/Desktop/<ip>/ and ~/Downloads/<ip>/ for new images,
# packages each as a task, and rsyncs to the personal machine.
#
set -euo pipefail

CONFIG_FILE="$HOME/.screenshot-agent/config.json"
SENT_DIR="$HOME/.screenshot-agent/sent"
LOG_FILE="$HOME/.screenshot-agent/logs/watcher.log"

mkdir -p "$SENT_DIR" "$(dirname "$LOG_FILE")"

log() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*" >> "$LOG_FILE"; }

if [[ ! -f "$CONFIG_FILE" ]]; then
  log "ERROR: No config found at $CONFIG_FILE"
  exit 1
fi

SSH_HOST=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c['sshHost'])")
SSH_USER=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('sshUser',''))")
SSH_PORT=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('sshPort',22))")
DEFAULT_REPO=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('defaultRepo',''))")
REMOTE_QUEUE=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('remoteQueueDir','~/.screenshot-agent/queue'))")

TARGET=""
if [[ -n "$SSH_USER" ]]; then
  TARGET="${SSH_USER}@${SSH_HOST}"
else
  TARGET="$SSH_HOST"
fi

SSH_OPTS=(-p "$SSH_PORT" -o ConnectTimeout=10)

# Directories to watch
DROP_DIRS=(
  "$HOME/Desktop/$SSH_HOST"
  "$HOME/Downloads/$SSH_HOST"
)

IMAGE_EXTS="png jpg jpeg webp gif"

is_image() {
  local ext="${1##*.}"
  ext=$(echo "$ext" | tr '[:upper:]' '[:lower:]')
  for e in $IMAGE_EXTS; do
    [[ "$ext" == "$e" ]] && return 0
  done
  return 1
}

# Find repo/message from sidecar JSON or subdirectory structure
resolve_context() {
  local img_path="$1"
  local drop_dir="$2"
  local repo="$DEFAULT_REPO"
  local msg=""

  # Check for sidecar JSON: same name but .json extension
  local base="${img_path%.*}"
  if [[ -f "${base}.json" ]]; then
    repo=$(python3 -c "import json; c=json.load(open('${base}.json')); print(c.get('repo','$DEFAULT_REPO'))")
    msg=$(python3 -c "import json; c=json.load(open('${base}.json')); print(c.get('msg',''))")
    log "Found sidecar: ${base}.json (repo=$repo, msg=$msg)"
    # Clean up sidecar after reading
    rm -f "${base}.json"
  else
    # Check subdirectory convention: <drop_dir>/<owner>/<repo>/image.png
    local rel="${img_path#$drop_dir/}"
    local parts
    IFS='/' read -ra parts <<< "$rel"
    if [[ ${#parts[@]} -ge 3 ]]; then
      repo="${parts[0]}/${parts[1]}"
      log "Derived repo from path: $repo"
    fi
  fi

  echo "$repo"
  echo "$msg"
}

process_image() {
  local img_path="$1"
  local drop_dir="$2"
  local filename
  filename=$(basename "$img_path")

  # Skip if already sent (dedup by filename+mtime)
  local mtime
  mtime=$(stat -f %m "$img_path" 2>/dev/null || stat -c %Y "$img_path" 2>/dev/null)
  local dedup_key="${filename}_${mtime}"
  if [[ -f "$SENT_DIR/$dedup_key" ]]; then
    return 0
  fi

  log "New image: $img_path"

  # Resolve repo and message
  local context
  context=$(resolve_context "$img_path" "$drop_dir")
  local repo msg
  repo=$(echo "$context" | head -1)
  msg=$(echo "$context" | tail -1)

  if [[ -z "$repo" ]]; then
    log "WARNING: No repo specified and no defaultRepo in config. Skipping $filename"
    return 0
  fi

  # Create task
  local task_id
  task_id="$(date +%s)-$(openssl rand -hex 2)"
  local task_dir="/tmp/screenshot-agent-tasks/$task_id"
  mkdir -p "$task_dir"

  local ext="${img_path##*.}"
  cp "$img_path" "$task_dir/screenshot.${ext}"

  cat > "$task_dir/task.json" <<TASK
{
  "repo": "$repo",
$(if [[ -n "$msg" ]]; then echo "  \"message\": \"$msg\","; fi)
  "timestamp": $(date +%s)000,
  "status": "pending"
}
TASK

  # Send to personal machine
  log "Sending task $task_id to $TARGET (repo=$repo)"
  ssh "${SSH_OPTS[@]}" "$TARGET" "mkdir -p $REMOTE_QUEUE/$task_id"

  local rsh_flag="ssh -p $SSH_PORT"
  if rsync -az --rsh "$rsh_flag" "$task_dir/" "$TARGET:$REMOTE_QUEUE/$task_id/" 2>> "$LOG_FILE"; then
    log "Task $task_id sent successfully"
    # Mark as sent
    touch "$SENT_DIR/$dedup_key"
    # Move image to sent dir to keep drop folder clean
    mv "$img_path" "$SENT_DIR/$filename" 2>/dev/null || true
  else
    log "ERROR: Failed to send task $task_id"
  fi

  rm -rf "$task_dir"
}

# Scan all drop directories
for drop_dir in "${DROP_DIRS[@]}"; do
  [[ ! -d "$drop_dir" ]] && continue

  # Process images at top level
  for img in "$drop_dir"/*; do
    [[ ! -f "$img" ]] && continue
    is_image "$img" && process_image "$img" "$drop_dir"
  done

  # Process images in subdirectories (owner/repo/image.png convention)
  find "$drop_dir" -mindepth 2 -type f 2>/dev/null | while read -r img; do
    is_image "$img" && process_image "$img" "$drop_dir"
  done
done

log "Scan complete."
