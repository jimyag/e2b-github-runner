import { describe, expect, test } from "bun:test"

import viteConfig from "../vite.config"

describe("Vite manual chunks", () => {
  test("keeps React ecosystem packages out of the React core chunk", () => {
    const manualChunks = viteConfig.build?.rollupOptions?.output?.manualChunks

    expect(typeof manualChunks).toBe("function")
    expect(manualChunks("/app/node_modules/react/index.js")).toBe("vendor-react")
    expect(manualChunks("/app/node_modules/react-dom/client.js")).toBe("vendor-react")
    expect(manualChunks("/app/node_modules/scheduler/index.js")).toBe("vendor-react")
    expect(manualChunks("/app/node_modules/react-remove-scroll/dist/es2015/index.js")).toBeUndefined()
    expect(manualChunks("/app/node_modules/react-redux/dist/react-redux.mjs")).toBeUndefined()
    expect(manualChunks("/app/node_modules/react-joyride/dist/index.mjs")).toBeUndefined()
  })
})

describe("Vite build warnings", () => {
  test("fails the build for unsafe cross-chunk cycles", () => {
    const onLog = viteConfig.build?.rollupOptions?.onLog
    const dangerousCodes = ["CIRCULAR_CHUNK", "CYCLIC_CROSS_CHUNK_REEXPORT"]

    expect(typeof onLog).toBe("function")

    for (const code of dangerousCodes) {
      expect(() =>
        onLog(
          "warn",
          { code, message: `unsafe ${code}` },
          (level, log) => {
            if (level === "error") {
              throw new Error(log.message)
            }
          },
        ),
      ).toThrow(`unsafe ${code}`)
    }
  })

  test("preserves non-blocking Rollup warnings", () => {
    const onLog = viteConfig.build?.rollupOptions?.onLog
    const forwardedLevels = []

    expect(typeof onLog).toBe("function")

    onLog(
      "warn",
      { code: "CHUNK_SIZE_LIMIT", message: "large chunk" },
      (level) => {
        forwardedLevels.push(level)
      },
    )

    expect(forwardedLevels).toEqual(["warn"])
  })
})
