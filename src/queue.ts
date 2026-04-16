import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { basename, extname, join } from "node:path";

export interface TaskPayload {
  repo: string;
  message?: string;
  timestamp: number;
  status: "pending" | "processing" | "done" | "error";
}

export interface TaskResult {
  status: "success" | "error";
  pr_url?: string;
  branch?: string;
  summary: string;
  timestamp: number;
}

export function createTaskId(): string {
  const hex = Math.random().toString(16).slice(2, 6);
  return `${Date.now()}-${hex}`;
}

export function writeTask(
  baseDir: string,
  taskId: string,
  payload: TaskPayload,
  screenshotPath: string
): string {
  const taskDir = join(baseDir, taskId);
  mkdirSync(taskDir, { recursive: true });
  writeFileSync(
    join(taskDir, "task.json"),
    JSON.stringify(payload, null, 2) + "\n"
  );
  const ext = extname(screenshotPath) || ".png";
  const dest = join(taskDir, `screenshot${ext}`);
  copyFileSync(screenshotPath, dest);
  return taskDir;
}

export function readTask(taskDir: string): TaskPayload {
  return JSON.parse(readFileSync(join(taskDir, "task.json"), "utf-8"));
}

export function updateTaskStatus(
  taskDir: string,
  status: TaskPayload["status"]
): void {
  const task = readTask(taskDir);
  task.status = status;
  writeFileSync(
    join(taskDir, "task.json"),
    JSON.stringify(task, null, 2) + "\n"
  );
}

export function writeResult(
  resultsDir: string,
  taskId: string,
  result: TaskResult
): void {
  const resultDir = join(resultsDir, taskId);
  mkdirSync(resultDir, { recursive: true });
  writeFileSync(
    join(resultDir, "result.json"),
    JSON.stringify(result, null, 2) + "\n"
  );
}

export function readResult(resultDir: string): TaskResult {
  return JSON.parse(readFileSync(join(resultDir, "result.json"), "utf-8"));
}

export function findScreenshot(taskDir: string): string | null {
  const files = readdirSync(taskDir);
  const shot = files.find((f) => f.startsWith("screenshot."));
  return shot ? join(taskDir, shot) : null;
}

export function listPendingTasks(queueDir: string): string[] {
  if (!existsSync(queueDir)) return [];
  const entries = readdirSync(queueDir, { withFileTypes: true });
  const pending: { dir: string; ts: number }[] = [];

  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const taskDir = join(queueDir, entry.name);
    const taskFile = join(taskDir, "task.json");
    if (!existsSync(taskFile)) continue;
    try {
      const task: TaskPayload = JSON.parse(readFileSync(taskFile, "utf-8"));
      if (task.status === "pending") {
        pending.push({ dir: taskDir, ts: task.timestamp });
      }
    } catch {
      // skip malformed
    }
  }

  return pending.sort((a, b) => a.ts - b.ts).map((p) => p.dir);
}
