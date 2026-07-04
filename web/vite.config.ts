import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Backend API base for dev proxy.
// Prefer explicit env override, fall back to the local dev gateway port.
const devApiTarget =
  process.env.LOGTAP_DEV_API_BASE ||
  process.env.VITE_API_BASE ||
  "http://127.0.0.1:18080";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
  },
  server: {
    port: 5173,
    fs: {
      allow: [".."],
    },
    proxy: {
      "/api": {
        target: devApiTarget,
        changeOrigin: true,
      },
    },
  },
});
