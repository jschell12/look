import { resolve } from "node:path";
import { existsSync } from "node:fs";
import { buildPrompt } from "./prompt.js";
import { spawnAgent } from "./spawn.js";
import { loadConfig } from "./config.js";
import { createTaskId, writeTask, type TaskPayload } from "./queue.js";
import { sendTask, pollForResult } from "./remote.js";
import { tmpdir } from "node:os";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { spawn } from "node:child_process";

const USAGE = `Usage: screenshot-agent <screenshot> --repo <repo> [--msg "context"] [--remote]

  <screenshot>   Path to a screenshot image (.png, .jpg, .webp, .gif)
  --repo <repo>  GitHub repo (owner/name or URL) or local path
  --msg  <msg>   Optional context to guide the agent
  --remote       Send to personal machine for processing (requires 'make setup')

Examples:
  screenshot-agent ./bug.png --repo jschell12/my-app
  screenshot-agent ./bug.png --repo jschell12/my-app --msg "fix the button alignment"
  screenshot-agent ./bug.png --repo jschell12/my-app --remote --msg "fix this error on login"`;

function parseArgs(argv: string[]) {
  const args = argv.slice(2);

  if (args.length === 0 || args.includes("--help") || args.includes("-h")) {
    console.log(USAGE);
    process.exit(0);
  }

  let screenshot: string | undefined;
  let repo: string | undefined;
  let message: string | undefined;
  let remote = false;

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === "--repo" && i + 1 < args.length) {
      repo = args[++i];
    } else if (arg === "--msg" && i + 1 < args.length) {
      message = args[++i];
    } else if (arg === "--remote") {
      remote = true;
    } else if (!arg.startsWith("--") && !screenshot) {
      screenshot = arg;
    }
  }

  if (!screenshot) {
    console.error("Error: screenshot path is required\n");
    console.log(USAGE);
    process.exit(1);
  }

  if (!repo) {
    console.error("Error: --repo is required\n");
    console.log(USAGE);
    process.exit(1);
  }

  return { screenshot, repo, message, remote };
}

function validateScreenshot(path: string): string {
  const abs = resolve(path);
  if (!existsSync(abs)) {
    console.error(`Error: screenshot not found: ${abs}`);
    process.exit(1);
  }
  const ext = abs.toLowerCase().split(".").pop();
  if (!ext || !["png", "jpg", "jpeg", "webp", "gif"].includes(ext)) {
    console.error(`Error: unsupported image format: .${ext}`);
    process.exit(1);
  }
  return abs;
}

async function runLocal(
  screenshotPath: string,
  repo: string,
  message?: string
): Promise<void> {
  const prompt = buildPrompt({ screenshotPath, repo, message });
  const exitCode = await spawnAgent({ prompt });
  process.exit(exitCode);
}

async function runRemote(
  screenshotPath: string,
  repo: string,
  message?: string
): Promise<void> {
  const config = loadConfig();
  const taskId = createTaskId();

  // Create task in a temp directory
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

  if (result.status === "success") {
    console.log("Fix applied successfully!");
    if (result.pr_url) console.log(`PR: ${result.pr_url}`);
    if (result.branch) console.log(`Branch: ${result.branch}`);

    // If repo is a local path, offer to pull
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
  const { screenshot, repo, message, remote } = parseArgs(process.argv);
  const screenshotPath = validateScreenshot(screenshot);

  console.log(`Screenshot: ${screenshotPath}`);
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
