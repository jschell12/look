import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts", "src/daemon.ts"],
  format: ["esm"],
  target: "node22",
  dts: false,
  clean: true,
  banner: { js: "#!/usr/bin/env node" },
});
