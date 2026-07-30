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
