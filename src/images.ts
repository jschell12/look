import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  statSync,
  writeFileSync,
  appendFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { basename, extname, join } from "node:path";

const IMAGE_EXTS = new Set([".png", ".jpg", ".jpeg", ".webp", ".gif"]);

/** Central image store — images are moved here from Desktop/Downloads */
const IMAGE_DIR = join(homedir(), ".screenshot-agent");
const TRACKED_FILE = join(IMAGE_DIR, ".tracked");

function isImage(filename: string): boolean {
  return IMAGE_EXTS.has(extname(filename).toLowerCase());
}

/** Ensure ~/.screenshot-agent/ and .tracked exist */
function ensureImageDir(): void {
  mkdirSync(IMAGE_DIR, { recursive: true });
  if (!existsSync(TRACKED_FILE)) {
    writeFileSync(
      TRACKED_FILE,
      "# Tracked processed images — one filename per line\n"
    );
  }
}

/** Get the set of already-processed filenames (basenames in ~/.screenshot-agent/) */
function loadTracked(): Set<string> {
  ensureImageDir();
  const lines = readFileSync(TRACKED_FILE, "utf-8").split("\n");
  const set = new Set<string>();
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed && !trimmed.startsWith("#")) set.add(trimmed);
  }
  return set;
}

/** Mark an image as processed by filename */
export function markProcessed(absPath: string): void {
  ensureImageDir();
  appendFileSync(TRACKED_FILE, basename(absPath) + "\n");
}

/** List images in a directory, sorted by mtime descending (newest first) */
function listImages(dir: string): { path: string; name: string; mtime: number }[] {
  if (!existsSync(dir)) return [];
  const entries = readdirSync(dir, { withFileTypes: true });
  const images: { path: string; name: string; mtime: number }[] = [];

  for (const entry of entries) {
    if (!entry.isFile()) continue;
    if (entry.name.startsWith(".")) continue;
    if (!isImage(entry.name)) continue;
    const fullPath = join(dir, entry.name);
    const stat = statSync(fullPath);
    images.push({ path: fullPath, name: entry.name, mtime: stat.mtimeMs });
  }

  return images.sort((a, b) => b.mtime - a.mtime);
}

/**
 * Move an image from Desktop/Downloads into ~/.screenshot-agent/.
 * Returns the new path. If a file with the same name exists, dedup with timestamp.
 */
function ingestImage(srcPath: string): string {
  ensureImageDir();
  let destName = basename(srcPath);
  let destPath = join(IMAGE_DIR, destName);

  // Dedup if name collision
  if (existsSync(destPath)) {
    const ext = extname(destName);
    const stem = destName.slice(0, -ext.length);
    destName = `${stem}-${Date.now()}${ext}`;
    destPath = join(IMAGE_DIR, destName);
  }

  renameSync(srcPath, destPath);
  return destPath;
}

/**
 * Scan ~/Desktop and ~/Downloads for new images.
 * Move any found into ~/.screenshot-agent/.
 * Returns the count of newly ingested images.
 */
export function ingestFromScanDirs(): number {
  const home = homedir();
  const scanDirs = [
    join(home, "Desktop"),
    join(home, "Downloads"),
  ];

  let count = 0;
  for (const dir of scanDirs) {
    for (const img of listImages(dir)) {
      ingestImage(img.path);
      count++;
    }
  }
  return count;
}

export interface DiscoveredImage {
  path: string;
  name: string;
  isProcessed: boolean;
}

/**
 * Find the latest unprocessed image in ~/.screenshot-agent/.
 * Returns null if none found.
 */
export function findLatestImage(): DiscoveredImage | null {
  ensureImageDir();
  const tracked = loadTracked();
  const images = listImages(IMAGE_DIR);

  for (const img of images) {
    if (!tracked.has(img.name)) {
      return { path: img.path, name: img.name, isProcessed: false };
    }
  }

  return null;
}

/**
 * List all images in ~/.screenshot-agent/ with their processed status.
 */
export function listAllImages(): DiscoveredImage[] {
  ensureImageDir();
  const tracked = loadTracked();
  const images = listImages(IMAGE_DIR);

  return images.map((img) => ({
    path: img.path,
    name: img.name,
    isProcessed: tracked.has(img.name),
  }));
}

/**
 * List unprocessed images only.
 */
export function listUnprocessed(): DiscoveredImage[] {
  return listAllImages().filter((img) => !img.isProcessed);
}
