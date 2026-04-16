import { QUEUE_DIR, RESULTS_DIR, ensureDirs } from "./config.js";
import {
  findScreenshot,
  listPendingTasks,
  readTask,
  updateTaskStatus,
  writeResult,
} from "./queue.js";
import { buildPrompt } from "./prompt.js";
import { spawnAgentCapture } from "./spawn.js";

function log(msg: string): void {
  const ts = new Date().toISOString();
  console.log(`[${ts}] ${msg}`);
}

function extractPrUrl(output: string): string | undefined {
  const match = output.match(/https:\/\/github\.com\/[^\s]+\/pull\/\d+/);
  return match?.[0];
}

function extractBranch(output: string): string | undefined {
  const match = output.match(/screenshot-fix\/\d+/);
  return match?.[0];
}

async function processTask(taskDir: string): Promise<void> {
  const taskId = taskDir.split("/").pop()!;
  log(`Processing task ${taskId}`);

  updateTaskStatus(taskDir, "processing");

  const task = readTask(taskDir);
  const screenshotPath = findScreenshot(taskDir);

  if (!screenshotPath) {
    log(`No screenshot found in ${taskDir}`);
    writeResult(RESULTS_DIR, taskId, {
      status: "error",
      summary: "No screenshot found in task directory",
      timestamp: Date.now(),
    });
    updateTaskStatus(taskDir, "error");
    return;
  }

  const prompt = buildPrompt({
    screenshotPath,
    repo: task.repo,
    message: task.message,
  });

  try {
    const result = await spawnAgentCapture({ prompt });
    const combined = result.stdout + "\n" + result.stderr;

    if (result.exitCode === 0) {
      const prUrl = extractPrUrl(combined);
      const branch = extractBranch(combined);

      log(`Task ${taskId} completed. PR: ${prUrl ?? "none detected"}`);

      writeResult(RESULTS_DIR, taskId, {
        status: "success",
        pr_url: prUrl,
        branch,
        summary: combined.slice(-2000), // last 2000 chars as summary
        timestamp: Date.now(),
      });
      updateTaskStatus(taskDir, "done");
    } else {
      log(`Task ${taskId} failed (exit ${result.exitCode})`);

      writeResult(RESULTS_DIR, taskId, {
        status: "error",
        summary: combined.slice(-2000),
        timestamp: Date.now(),
      });
      updateTaskStatus(taskDir, "error");
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    log(`Task ${taskId} threw: ${msg}`);

    writeResult(RESULTS_DIR, taskId, {
      status: "error",
      summary: msg,
      timestamp: Date.now(),
    });
    updateTaskStatus(taskDir, "error");
  }
}

async function pollLoop(intervalMs: number): Promise<void> {
  log(`Daemon started. Watching ${QUEUE_DIR} every ${intervalMs / 1000}s`);

  let processing = false;

  const tick = async () => {
    if (processing) return;
    const pending = listPendingTasks(QUEUE_DIR);
    if (pending.length === 0) return;

    processing = true;
    try {
      await processTask(pending[0]);
    } finally {
      processing = false;
    }
  };

  // Run immediately, then on interval
  await tick();
  const timer = setInterval(tick, intervalMs);

  // Graceful shutdown
  const shutdown = () => {
    log("Shutting down...");
    clearInterval(timer);
    // If processing, let it finish (the process will exit when it's done)
    if (!processing) process.exit(0);
  };

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

const intervalMs = parseInt(process.env.POLL_INTERVAL_MS ?? "5000", 10);
ensureDirs();
pollLoop(intervalMs);
