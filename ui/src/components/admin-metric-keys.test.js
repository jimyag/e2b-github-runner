import { describe, expect, test } from "bun:test"

import * as AdminFormatModule from "../admin-format"
import i18n from "../i18n"

describe("translated admin metrics", () => {
  test("keep stable IDs while labels change with language", () => {
    expect(typeof AdminFormatModule.runnerMetrics).toBe("function")
    expect(typeof AdminFormatModule.accountMetrics).toBe("function")
    if (
      typeof AdminFormatModule.runnerMetrics !== "function"
      || typeof AdminFormatModule.accountMetrics !== "function"
    ) return

    const en = i18n.getFixedT("en")
    const zh = i18n.getFixedT("zh")
    const stats = {
      total_accounts: 4,
      admin_accounts: 1,
      user_accounts: 3,
      oauth_identities: 4,
    }

    const englishAccountMetrics = AdminFormatModule.accountMetrics(stats, en)
    const chineseAccountMetrics = AdminFormatModule.accountMetrics(stats, zh)
    expect(englishAccountMetrics.map((metric) => metric.id)).toEqual([
      "accounts",
      "administrators",
      "users",
      "identities",
    ])
    expect(chineseAccountMetrics.map((metric) => metric.id)).toEqual(
      englishAccountMetrics.map((metric) => metric.id),
    )
    expect(chineseAccountMetrics.map((metric) => metric.label)).not.toEqual(
      englishAccountMetrics.map((metric) => metric.label),
    )

    const englishRunnerMetrics = AdminFormatModule.runnerMetrics([], 2, en)
    const chineseRunnerMetrics = AdminFormatModule.runnerMetrics([], 2, zh)
    expect(englishRunnerMetrics.map((metric) => metric.id)).toEqual([
      "active",
      "completed",
      "failed",
      "runner-specs",
    ])
    expect(chineseRunnerMetrics.map((metric) => metric.id)).toEqual(
      englishRunnerMetrics.map((metric) => metric.id),
    )
    expect(chineseRunnerMetrics.map((metric) => metric.label)).not.toEqual(
      englishRunnerMetrics.map((metric) => metric.label),
    )
  })
})
