import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

export interface Config {
  sshHost: string;
  sshUser?: string;
  sshPort?: number;
  remoteQueueDir?: string;
  remoteResultsDir?: string;
  defaultRepo?: string;
}

export const CONFIG_DIR = join(homedir(), ".screenshot-agent");
export const CONFIG_PATH = join(CONFIG_DIR, "config.json");
export const QUEUE_DIR = join(CONFIG_DIR, "queue");
export const RESULTS_DIR = join(CONFIG_DIR, "results");
export const LOGS_DIR = join(CONFIG_DIR, "logs");

export function loadConfig(): Config {
  if (!existsSync(CONFIG_PATH)) {
    throw new Error(
      `Config not found at ${CONFIG_PATH}\nRun 'make setup' to configure SSH connection.`
    );
  }
  return JSON.parse(readFileSync(CONFIG_PATH, "utf-8"));
}

export function saveConfig(config: Config): void {
  mkdirSync(CONFIG_DIR, { recursive: true });
  writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2) + "\n");
}

export function ensureDirs(): void {
  for (const dir of [CONFIG_DIR, QUEUE_DIR, RESULTS_DIR, LOGS_DIR]) {
    mkdirSync(dir, { recursive: true });
  }
}

export function sshTarget(config: Config): string {
  const user = config.sshUser ? `${config.sshUser}@` : "";
  return `${user}${config.sshHost}`;
}

export function sshArgs(config: Config): string[] {
  const args: string[] = [];
  if (config.sshPort && config.sshPort !== 22) {
    args.push("-p", String(config.sshPort));
  }
  return args;
}

export function remoteQueueDir(config: Config): string {
  return config.remoteQueueDir || "~/.screenshot-agent/queue";
}

export function remoteResultsDir(config: Config): string {
  return config.remoteResultsDir || "~/.screenshot-agent/results";
}
