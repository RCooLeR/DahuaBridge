import { resolve } from "node:path";
import { defineConfig } from "vite";

export default defineConfig({
  build: {
    emptyOutDir: true,
    lib: {
      entry: resolve(__dirname, "src/cards/index.ts"),
      formats: ["es"],
      fileName: () => "dahuabridge-surveillance-panel.js",
    },
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
      },
    },
    sourcemap: true,
    target: ["es2020","chrome58","edge88","firefox72","node12","safari15"],
  },
});
