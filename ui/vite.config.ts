import path from "path"
import react from "@vitejs/plugin-react-swc"
import { defineConfig } from "vite"
import tailwindcss from "@tailwindcss/vite"

export default defineConfig({
  base: "/",
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/healthz": "http://127.0.0.1:25500",
      "/runner_requests": "http://127.0.0.1:25500",
      "/runner_specs": "http://127.0.0.1:25500",
      "/runner_policies": "http://127.0.0.1:25500",
      "/audit-events": "http://127.0.0.1:25500",
      "/diagnostics": "http://127.0.0.1:25500",
      "/auth": "http://127.0.0.1:25500",
      "/user/github-app": "http://127.0.0.1:25500",
      "/user/runner_requests": "http://127.0.0.1:25500",
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    target: "esnext",
    outDir: "../internal/server/ui",
    emptyOutDir: true,
    modulePreload: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (
            id.includes("node_modules/react") ||
            id.includes("node_modules/react-dom") ||
            id.includes("node_modules/scheduler")
          ) {
            return "vendor-react"
          }
          if (id.includes("node_modules/@radix-ui") || id.includes("node_modules/cmdk")) {
            return "vendor-radix"
          }
          if (id.includes("node_modules/lucide-react")) {
            return "vendor-icons"
          }
        },
      },
    },
  },
  optimizeDeps: {
    esbuildOptions: {
      target: "esnext",
      supported: {
        "top-level-await": true,
      },
    },
  },
})
