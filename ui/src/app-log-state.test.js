import { describe, expect, test } from "bun:test"

import i18n from "./i18n"
import { runnerLogTextForView } from "./app-log-state"

describe("runnerLogTextForView", () => {
  test("uses the current language for the no-runner placeholder", () => {
    expect(runnerLogTextForView("", "", i18n.getFixedT("en"))).toBe("No runner selected")
    expect(runnerLogTextForView("", "", i18n.getFixedT("zh"))).toBe("尚未选择 Runner")
  })
})
