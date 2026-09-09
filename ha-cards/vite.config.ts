import { resolve } from "node:path";
import { gzipSync } from "node:zlib";
import { defineConfig } from "vite";

export default defineConfig({
  // HA can install the module in any www subdirectory. Keep emitted assets
  // relative to that module instead of resolving them from the HA host root.
  base: "./",
  plugins: [{
    name: "initial-bundle-budget",
    generateBundle(_options, bundle) {
      const pending = Object.values(bundle).filter((item) => item.type === "chunk" && item.isEntry).map((item) => item.fileName);
      const visited = new Set<string>();
      let gzipBytes = 0;
      while (pending.length) {
        const name = pending.pop()!;
        if (visited.has(name)) continue;
        visited.add(name);
        const chunk = bundle[name];
        if (chunk?.type !== "chunk") continue;
        gzipBytes += gzipSync(chunk.code).length;
        // Count the complete static import graph; optional dynamic chunks load later.
        pending.push(...chunk.imports);
        if (Object.keys(chunk.modules).some((id) => /node_modules[\\/](?:hls\.js|dashjs)[\\/]/.test(id))) {
          this.error("Optional player engines must not enter the initial card bundle.");
        }
      }
      if (gzipBytes > 175_000) this.error(`Initial card bundle exceeds 175 kB gzip: ${gzipBytes} bytes.`);
      this.info(`Initial card bundle: ${gzipBytes} bytes gzip (budget 175000).`);
    },
  }],
  build: {
    emptyOutDir: true,
    lib: {
      entry: resolve(import.meta.dirname, "src/cards/index.ts"),
      formats: ["es"],
      fileName: () => "dahuabridge-surveillance-panel.js",
    },
    rolldownOptions: {
      output: {
        chunkFileNames: "chunks/[name]-[hash].js",
      },
    },
    sourcemap: true,
    target: ["es2020", "chrome58", "edge88", "firefox72", "node12", "safari15"],
  },
});
