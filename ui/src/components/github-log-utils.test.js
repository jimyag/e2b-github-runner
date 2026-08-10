import { describe, expect, test } from "bun:test"

import i18n from "../i18n"
import { githubLogFailureMessage } from "./github-log-utils"

describe("githubLogFailureMessage", () => {
  test("uses the GitHub-specific fallback for non-Error failures", () => {
    expect(githubLogFailureMessage({ status: 500 }, i18n.getFixedT("en"))).toBe("Failed to load GitHub log")
    expect(githubLogFailureMessage({ status: 500 }, i18n.getFixedT("zh"))).toBe("无法加载 GitHub 日志")
  })
})
