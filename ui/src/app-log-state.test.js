import { describe, expect, test } from "bun:test"

import i18n from "./i18n"
import { runnerLogTextForView } from "./app-log-state"

describe("runnerLogTextForView", () => {
  test("uses the current language for the no-runner placeholder", () => {
    expect(runnerLogTextForView("", "", i18n.getFixedT("en"))).toBe("No runner selected")
    expect(runnerLogTextForView("", "", i18n.getFixedT("zh"))).toBe("尚未选择 Runner")
  })

  test("resolves semantic log placeholders with the current language", () => {
    const emptyLog = { kind: "message", key: "user.runnerLogEmpty" }

    expect(runnerLogTextForView("runner-1", emptyLog, i18n.getFixedT("en"))).toBe("Log is empty")
    expect(runnerLogTextForView("runner-1", emptyLog, i18n.getFixedT("zh"))).toBe("日志为空")
  })

  test("preserves raw log and server error text", () => {
    const rawLog = { kind: "text", text: "server-provided log" }

    expect(runnerLogTextForView("runner-1", rawLog, i18n.getFixedT("zh"))).toBe("server-provided log")
  })
})
