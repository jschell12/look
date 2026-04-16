import { spawn } from "node:child_process";

export interface SpawnOptions {
  prompt: string;
  cwd?: string;
}

export interface CapturedResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

function claudeArgs(prompt: string): string[] {
  return ["-p", prompt, "--dangerously-skip-permissions"];
}

function handleSpawnError(err: Error): never {
  if ((err as NodeJS.ErrnoException).code === "ENOENT") {
    throw new Error(
      "claude CLI not found on PATH. Install it: npm i -g @anthropic-ai/claude-code"
    );
  }
  throw err;
}

/** Interactive mode — inherits stdio so user sees output */
export function spawnAgent(opts: SpawnOptions): Promise<number> {
  return new Promise((resolve, reject) => {
    const child = spawn("claude", claudeArgs(opts.prompt), {
      cwd: opts.cwd,
      stdio: "inherit",
      env: { ...process.env },
    });

    child.on("error", (err) => reject(handleSpawnError(err)));
    child.on("close", (code) => resolve(code ?? 0));
  });
}

/** Capture mode — collects stdout/stderr for daemon use */
export function spawnAgentCapture(opts: SpawnOptions): Promise<CapturedResult> {
  return new Promise((resolve, reject) => {
    const child = spawn("claude", claudeArgs(opts.prompt), {
      cwd: opts.cwd,
      stdio: ["ignore", "pipe", "pipe"],
      env: { ...process.env },
    });

    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];

    child.stdout!.on("data", (chunk) => stdout.push(chunk));
    child.stderr!.on("data", (chunk) => stderr.push(chunk));

    child.on("error", (err) => reject(handleSpawnError(err)));
    child.on("close", (code) => {
      resolve({
        exitCode: code ?? 0,
        stdout: Buffer.concat(stdout).toString("utf-8"),
        stderr: Buffer.concat(stderr).toString("utf-8"),
      });
    });
  });
}
