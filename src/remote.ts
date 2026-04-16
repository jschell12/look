import { spawn } from "node:child_process";
import {
  type Config,
  remoteQueueDir,
  remoteResultsDir,
  sshArgs,
  sshTarget,
} from "./config.js";
import type { TaskResult } from "./queue.js";

function exec(
  cmd: string,
  args: string[],
  timeoutMs = 30_000
): Promise<{ stdout: string; stderr: string; code: number }> {
  return new Promise((resolve, reject) => {
    const child = spawn(cmd, args, {
      stdio: ["ignore", "pipe", "pipe"],
      timeout: timeoutMs,
    });

    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];

    child.stdout!.on("data", (chunk) => stdout.push(chunk));
    child.stderr!.on("data", (chunk) => stderr.push(chunk));

    child.on("error", reject);
    child.on("close", (code) => {
      resolve({
        stdout: Buffer.concat(stdout).toString("utf-8"),
        stderr: Buffer.concat(stderr).toString("utf-8"),
        code: code ?? 1,
      });
    });
  });
}

/** rsync a local task directory to the remote queue */
export async function sendTask(
  config: Config,
  taskDir: string,
  taskId: string
): Promise<void> {
  const target = sshTarget(config);
  const remotePath = `${remoteQueueDir(config)}/${taskId}/`;

  // Ensure remote queue dir exists
  await exec("ssh", [
    ...sshArgs(config),
    target,
    `mkdir -p ${remoteQueueDir(config)}/${taskId}`,
  ]);

  const rshFlag =
    config.sshPort && config.sshPort !== 22
      ? `ssh -p ${config.sshPort}`
      : "ssh";

  const { code, stderr } = await exec("rsync", [
    "-az",
    "--rsh",
    rshFlag,
    `${taskDir}/`,
    `${target}:${remotePath}`,
  ]);

  if (code !== 0) {
    throw new Error(`rsync failed (exit ${code}): ${stderr}`);
  }
}

/** Poll the remote machine for a result file */
export async function pollForResult(
  config: Config,
  taskId: string,
  opts?: { timeoutMs?: number; pollIntervalMs?: number }
): Promise<TaskResult> {
  const timeoutMs = opts?.timeoutMs ?? 600_000; // 10 minutes
  const pollMs = opts?.pollIntervalMs ?? 5_000;
  const target = sshTarget(config);
  const resultPath = `${remoteResultsDir(config)}/${taskId}/result.json`;
  const start = Date.now();

  while (Date.now() - start < timeoutMs) {
    const { stdout, code } = await exec("ssh", [
      ...sshArgs(config),
      target,
      `cat ${resultPath} 2>/dev/null`,
    ]);

    if (code === 0 && stdout.trim()) {
      try {
        return JSON.parse(stdout.trim());
      } catch {
        // not valid JSON yet, keep polling
      }
    }

    process.stderr.write(".");
    await new Promise((r) => setTimeout(r, pollMs));
  }

  throw new Error(
    `Timed out waiting for result after ${timeoutMs / 1000}s. Check the daemon on your personal machine.`
  );
}

/** Test SSH connectivity */
export async function testConnection(config: Config): Promise<boolean> {
  try {
    const { stdout, code } = await exec(
      "ssh",
      [...sshArgs(config), sshTarget(config), "echo ok"],
      10_000
    );
    return code === 0 && stdout.trim() === "ok";
  } catch {
    return false;
  }
}
