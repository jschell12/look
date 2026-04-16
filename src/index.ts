import { resolve } from "node:path";
import { existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";
import { buildPrompt } from "./prompt.js";
import { spawnAgent } from "./spawn.js";
import { loadConfig } from "./config.js";
import { createTaskId, writeTask, type TaskPayload } from "./queue.js";
import { sendTask, pollForResult } from "./remote.js";
import {
  findLatestImage,
  ingestFromScanDirs,
  markProcessed,
  listUnprocessed,
  listAllImages,
} from "./images.js";

const USAGE = `Usage: screenshot-agent --repo <repo> [--msg "context"] [--remote] [--list] [--scan]

  --repo <repo>  GitHub repo (owner/name or URL) or local path
  --msg  <msg>   Optional context to guide the agent
  --remote       Send to remote machine for processing (requires 'make setup')
  --list         List all images in ~/.screenshot-agent/ and their status
  --scan         Scan ~/Desktop and ~/Downloads, move images to ~/.screenshot-agent/

Image discovery:
  Images are stored in ~/.screenshot-agent/. The latest unprocessed image
  is automatically selected. Processed images are tracked in ~/.screenshot-agent/.tracked.

  To add images, either:
    - Run --scan to ingest from ~/Desktop and ~/Downloads
    - Manually move/copy images into ~/.screenshot-agent/

Examples:
  screenshot-agent --scan                                        # ingest images from Desktop/Downloads
  screenshot-agent --repo jschell12/my-app                       # fix latest unprocessed image
  screenshot-agent --repo jschell12/my-app --msg "fix the btn"   # with context
  screenshot-agent --list                                        # see all images + status`;

function parseArgs(argv: string[]) {
  const args = argv.slice(2);

  if (args.includes("--help") || args.includes("-h")) {
    console.log(USAGE);
    process.exit(0);
  }

  let repo: string | undefined;
  let message: string | undefined;
  let remote = false;
  let list = false;
  let scan = false;

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === "--repo" && i + 1 < args.length) {
      repo = args[++i];
    } else if (arg === "--msg" && i + 1 < args.length) {
      message = args[++i];
    } else if (arg === "--remote") {
      remote = true;
    } else if (arg === "--list") {
      list = true;
    } else if (arg === "--scan") {
      scan = true;
    }
  }

  return { repo, message, remote, list, scan };
}

async function runLocal(
  screenshotPath: string,
  repo: string,
  message?: string
): Promise<void> {
  const prompt = buildPrompt({ screenshotPath, repo, message });
  const exitCode = await spawnAgent({ prompt });
  markProcessed(screenshotPath);
  process.exit(exitCode);
}

async function runRemote(
  screenshotPath: string,
  repo: string,
  message?: string
): Promise<void> {
  const config = loadConfig();
  const taskId = createTaskId();

  const tmpBase = join(tmpdir(), "screenshot-agent-tasks");
  mkdirSync(tmpBase, { recursive: true });

  const payload: TaskPayload = {
    repo,
    message,
    timestamp: Date.now(),
    status: "pending",
  };

  const taskDir = writeTask(tmpBase, taskId, payload, screenshotPath);
  console.log(`Task ${taskId} created`);
  console.log(`Sending to ${config.sshHost}...`);

  await sendTask(config, taskDir, taskId);
  console.log("Task sent. Waiting for result...");

  const result = await pollForResult(config, taskId);
  console.log("\n---");

  markProcessed(screenshotPath);

  if (result.status === "success") {
    console.log("Fix applied successfully!");
    if (result.pr_url) console.log(`PR: ${result.pr_url}`);
    if (result.branch) console.log(`Branch: ${result.branch}`);

    if (existsSync(resolve(repo))) {
      console.log(`\nPulling latest in ${repo}...`);
      const pull = spawn("git", ["pull"], {
        cwd: resolve(repo),
        stdio: "inherit",
      });
      await new Promise<void>((res) => pull.on("close", () => res()));
    }
  } else {
    console.error("Agent reported an error:");
    console.error(result.summary.slice(-500));
    process.exit(1);
  }
}

async function main() {
  const { repo, message, remote, list, scan } = parseArgs(process.argv);

  // --scan: ingest images from Desktop/Downloads into ~/.screenshot-agent/
  if (scan) {
    const count = ingestFromScanDirs();
    console.log(`Ingested ${count} image(s) into ~/.screenshot-agent/`);
    if (!repo) process.exit(0);
  }

  // --list: show all images and their status
  if (list) {
    const images = listAllImages();
    if (images.length === 0) {
      console.log("No images in ~/.screenshot-agent/");
      console.log("Run --scan to ingest from ~/Desktop and ~/Downloads.");
    } else {
      const unprocessed = images.filter((i) => !i.isProcessed).length;
      console.log(`${images.length} image(s) in ~/.screenshot-agent/ (${unprocessed} unprocessed):\n`);
      for (const img of images) {
        const status = img.isProcessed ? "done" : "pending";
        console.log(`  [${status}] ${img.name}`);
      }
    }
    process.exit(0);
  }

  if (!repo) {
    console.error("Error: --repo is required\n");
    console.log(USAGE);
    process.exit(1);
  }

  // Find latest unprocessed image
  const found = findLatestImage();
  if (!found) {
    console.error("No unprocessed images in ~/.screenshot-agent/");
    console.error("Run: screenshot-agent --scan   to ingest from Desktop/Downloads");
    process.exit(1);
  }

  const screenshotPath = found.path;

  console.log(`Screenshot: ${found.name}`);
  console.log(`Target repo: ${repo}`);
  if (message) console.log(`Context: ${message}`);
  console.log(`Mode: ${remote ? "remote" : "local"}`);
  console.log("---");

  if (remote) {
    await runRemote(screenshotPath, repo, message);
  } else {
    await runLocal(screenshotPath, repo, message);
  }
}

main().catch((err) => {
  console.error(err.message || err);
  process.exit(1);
});
