import { describe, expect, test } from "bun:test"

import { localizedLogTextForView } from "../app-log-state"
import i18n from "../i18n"
import { githubLogFailureState } from "./github-log-utils"

describe("githubLogFailureState", () => {
  test("keeps the GitHub-specific fallback tied to the active language", () => {
    const failure = githubLogFailureState({ status: 500 })

    expect(localizedLogTextForView(failure, i18n.getFixedT("en"))).toBe("Failed to load GitHub log")
    expect(localizedLogTextForView(failure, i18n.getFixedT("zh"))).toBe("无法加载 GitHub 日志")
  })

  test("preserves Error messages as raw text", () => {
    const failure = githubLogFailureState(new Error("upstream log failed"))

    expect(localizedLogTextForView(failure, i18n.getFixedT("zh"))).toBe("upstream log failed")
  })
})
