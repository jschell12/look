#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const os = require('os');
const { execSync, spawn } = require('child_process');

const XMUGGLE_DIR = path.join(os.homedir(), '.xmuggle');
const CONFIG_FILE = path.join(XMUGGLE_DIR, 'daemon.json');
const PID_FILE = path.join(XMUGGLE_DIR, 'daemon.pid');
const LOG_FILE = path.join(XMUGGLE_DIR, 'daemon.log');
const INBOX_DIR = path.join(XMUGGLE_DIR, 'inbox');

// ── Config ──

const DEFAULT_CONFIG = {
  interval: 30,              // seconds between sync cycles
  repos: [],                 // [{ path: "/abs/path", pull: true, commands: ["make build"] }]
  syncRepo: '',              // git repo URL for image sync
  commands: [],              // global commands to run each cycle: ["echo hello"]
  onReceive: [],             // commands to run when new images arrive: ["notify-send 'new screenshot'"]
};

function loadConfig() {
  try {
    const raw = JSON.parse(fs.readFileSync(CONFIG_FILE, 'utf8'));
    return { ...DEFAULT_CONFIG, ...raw };
  } catch {
    return { ...DEFAULT_CONFIG };
  }
}

function saveConfig(config) {
  fs.mkdirSync(XMUGGLE_DIR, { recursive: true });
  fs.writeFileSync(CONFIG_FILE, JSON.stringify(config, null, 2) + '\n');
}

function ensureConfig() {
  if (!fs.existsSync(CONFIG_FILE)) {
    // Seed from existing sync-repo file if present
    const config = { ...DEFAULT_CONFIG };
    const syncRepoFile = path.join(XMUGGLE_DIR, 'sync-repo');
    try {
      config.syncRepo = fs.readFileSync(syncRepoFile, 'utf8').trim();
    } catch {}
    saveConfig(config);
  }
}

// ── Logging ──

function log(msg) {
  const ts = new Date().toISOString();
  const line = `[${ts}] ${msg}`;
  console.log(line);
  try {
    fs.appendFileSync(LOG_FILE, line + '\n');
  } catch {}
}

// ── Git env (reuse GH token if available) ──

function gitEnv() {
  const tokenFile = path.join(XMUGGLE_DIR, 'gh-token');
  let token = process.env.GH_TOKEN || process.env.GITHUB_TOKEN || '';
  if (!token) {
    try { token = fs.readFileSync(tokenFile, 'utf8').trim(); } catch {}
  }
  if (!token) return process.env;
  return {
    ...process.env,
    GH_TOKEN: token,
    GIT_ASKPASS: 'echo',
    GIT_TERMINAL_PROMPT: '0',
  };
}

// ── Sync repo (image relay) ──

function syncImages(config) {
  if (!config.syncRepo) return [];

  const syncDir = path.join(XMUGGLE_DIR, 'sync');
  const env = gitEnv();

  // Clone or pull
  if (!fs.existsSync(path.join(syncDir, '.git'))) {
    log(`Cloning sync repo: ${config.syncRepo}`);
    fs.mkdirSync(syncDir, { recursive: true });
    try {
      execSync(`git clone "${config.syncRepo}" "${syncDir}"`, { stdio: 'pipe', env });
    } catch (e) {
      log(`Sync clone failed: ${e.message}`);
      return [];
    }
  } else {
    try {
      execSync('git pull --ff-only', { cwd: syncDir, stdio: 'pipe', env });
    } catch (e) {
      log(`Sync pull failed: ${e.message}`);
      return [];
    }
  }

  // Import pending images from other hosts
  const pendingDir = path.join(syncDir, 'pending');
  if (!fs.existsSync(pendingDir)) return [];

  const hostname = os.hostname();
  const imported = [];

  for (const entry of fs.readdirSync(pendingDir)) {
    const dir = path.join(pendingDir, entry);
    const metaFile = path.join(dir, 'meta.json');
    if (!fs.existsSync(metaFile)) continue;

    try {
      const meta = JSON.parse(fs.readFileSync(metaFile, 'utf8'));
      if (meta.from === hostname) continue;

      const srcImage = path.join(dir, meta.filename);
      if (!fs.existsSync(srcImage)) continue;

      fs.mkdirSync(INBOX_DIR, { recursive: true });
      const destImage = path.join(INBOX_DIR, meta.filename);
      if (fs.existsSync(destImage)) continue;

      fs.copyFileSync(srcImage, destImage);
      if (meta.message) {
        fs.writeFileSync(destImage + '.msg', meta.message);
      }
      imported.push(meta.filename);
      log(`Imported: ${meta.filename} from ${meta.from}`);
    } catch {}
  }

  return imported;
}

// ── Repo sync (git pull + commands) ──

function syncRepos(config) {
  const env = gitEnv();

  for (const repo of config.repos) {
    if (!repo.path || !fs.existsSync(repo.path)) {
      log(`Repo not found: ${repo.path}`);
      continue;
    }

    if (repo.pull !== false) {
      try {
        log(`Pulling ${repo.path}...`);
        const output = execSync('git pull --ff-only', {
          cwd: repo.path,
          encoding: 'utf8',
          stdio: 'pipe',
          env,
        }).trim();
        if (output && !output.includes('Already up to date')) {
          log(`  ${output}`);
        }
      } catch (e) {
        log(`  Pull failed: ${e.message}`);
      }
    }

    if (repo.commands && repo.commands.length) {
      for (const cmd of repo.commands) {
        runCommand(cmd, repo.path);
      }
    }
  }
}

// ── Run a command ──

function runCommand(cmd, cwd) {
  log(`Running: ${cmd}` + (cwd ? ` (in ${cwd})` : ''));
  try {
    const output = execSync(cmd, {
      cwd: cwd || process.cwd(),
      encoding: 'utf8',
      stdio: 'pipe',
      env: gitEnv(),
      timeout: 120_000,
    }).trim();
    if (output) log(`  ${output}`);
  } catch (e) {
    const errMsg = e.stderr ? e.stderr.trim() : e.message;
    log(`  Error: ${errMsg}`);
  }
}

// ── Cycle ──

function runCycle() {
  const config = loadConfig();

  // Sync images from git
  const imported = syncImages(config);

  // Pull repos and run their commands
  syncRepos(config);

  // Run global commands
  for (const cmd of config.commands) {
    runCommand(cmd);
  }

  // Run onReceive commands if new images arrived
  if (imported.length > 0 && config.onReceive.length > 0) {
    log(`${imported.length} new image(s), running onReceive commands`);
    for (const cmd of config.onReceive) {
      // Substitute $FILES with the imported filenames
      const expanded = cmd.replace('$FILES', imported.join(' '));
      runCommand(expanded);
    }
  }
}

// ── CLI ──

const args = process.argv.slice(2);
const command = args[0] || 'start';

switch (command) {
  case 'start': {
    ensureConfig();
    const config = loadConfig();

    // Write PID
    fs.writeFileSync(PID_FILE, String(process.pid));
    log(`Daemon starting (pid ${process.pid}, interval ${config.interval}s)`);
    log(`Config: ${CONFIG_FILE}`);
    log(`Log: ${LOG_FILE}`);

    // Run immediately, then on interval
    runCycle();
    setInterval(runCycle, config.interval * 1000);
    break;
  }

  case 'run': {
    // Run a single cycle and exit
    ensureConfig();
    log('Running single cycle');
    runCycle();
    log('Done');
    break;
  }

  case 'stop': {
    try {
      const pid = parseInt(fs.readFileSync(PID_FILE, 'utf8').trim());
      process.kill(pid, 'SIGTERM');
      fs.unlinkSync(PID_FILE);
      console.log(`Stopped daemon (pid ${pid})`);
    } catch {
      console.log('No daemon running');
    }
    break;
  }

  case 'status': {
    try {
      const pid = parseInt(fs.readFileSync(PID_FILE, 'utf8').trim());
      process.kill(pid, 0); // Check if alive
      console.log(`Daemon running (pid ${pid})`);
    } catch {
      console.log('Daemon not running');
    }
    const config = loadConfig();
    console.log(`Config: ${CONFIG_FILE}`);
    console.log(`Interval: ${config.interval}s`);
    console.log(`Sync repo: ${config.syncRepo || '(none)'}`);
    console.log(`Repos: ${config.repos.length}`);
    console.log(`Commands: ${config.commands.length}`);
    console.log(`onReceive: ${config.onReceive.length}`);
    break;
  }

  case 'config': {
    ensureConfig();
    console.log(fs.readFileSync(CONFIG_FILE, 'utf8'));
    break;
  }

  case 'edit': {
    ensureConfig();
    const editor = process.env.EDITOR || 'vi';
    const child = spawn(editor, [CONFIG_FILE], { stdio: 'inherit' });
    child.on('exit', () => {
      console.log('Config saved:', CONFIG_FILE);
    });
    break;
  }

  case 'add-repo': {
    ensureConfig();
    const repoPath = args[1];
    if (!repoPath) { console.error('Usage: daemon.js add-repo <path> [command...]'); process.exit(1); }
    const config = loadConfig();
    const abs = path.resolve(repoPath);
    const commands = args.slice(2);
    const existing = config.repos.find(r => r.path === abs);
    if (existing) {
      if (commands.length) existing.commands = commands;
      console.log(`Updated repo: ${abs}`);
    } else {
      config.repos.push({ path: abs, pull: true, commands });
      console.log(`Added repo: ${abs}`);
    }
    saveConfig(config);
    break;
  }

  case 'add-command': {
    ensureConfig();
    const cmd = args.slice(1).join(' ');
    if (!cmd) { console.error('Usage: daemon.js add-command <command>'); process.exit(1); }
    const config = loadConfig();
    config.commands.push(cmd);
    saveConfig(config);
    console.log(`Added command: ${cmd}`);
    break;
  }

  case 'on-receive': {
    ensureConfig();
    const cmd = args.slice(1).join(' ');
    if (!cmd) { console.error('Usage: daemon.js on-receive <command>'); process.exit(1); }
    const config = loadConfig();
    config.onReceive.push(cmd);
    saveConfig(config);
    console.log(`Added onReceive: ${cmd}`);
    break;
  }

  case 'log': {
    try {
      const lines = fs.readFileSync(LOG_FILE, 'utf8').split('\n');
      const n = parseInt(args[1]) || 20;
      console.log(lines.slice(-n).join('\n'));
    } catch {
      console.log('No log file');
    }
    break;
  }

  default:
    console.log(`xmuggle daemon

Usage:
  node daemon.js start          Start the daemon (foreground)
  node daemon.js run            Run a single sync cycle
  node daemon.js stop           Stop the running daemon
  node daemon.js status         Show daemon status and config summary
  node daemon.js config         Print current config
  node daemon.js edit           Open config in $EDITOR
  node daemon.js log [n]        Show last n log lines (default 20)
  node daemon.js add-repo <path> [cmd...]   Add a repo to sync
  node daemon.js add-command <cmd>          Add a global command
  node daemon.js on-receive <cmd>           Add a command to run on new images

Config: ~/.xmuggle/daemon.json
`);
}
