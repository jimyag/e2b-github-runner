import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import i18n from "../i18n"
import { RunnerRequestsSection } from "./runner-requests-section"

function renderSelectedRunner() {
  return renderToStaticMarkup(
    createElement(RunnerRequestsSection, {
      hasAccess: false,
      loading: false,
      runners: [],
      filteredRunners: [],
      selected: {
        id: "runner-1",
        status: "completed",
        completed_at: "2026-08-10T08:09:10Z",
      },
      selectedID: "runner-1",
      selectedLog: "control.log",
      logText: "",
      createID: "",
      createRepository: "",
      createRunnerSpec: "",
      createLabels: "",
      createRunnerOpen: false,
      runnerStatusFilter: "all",
      runnerRepositoryFilter: "all",
      runnerSpecFilter: "all",
      runnerRepositories: [],
      runnerSpecNames: [],
      onRefresh: () => {},
      onResetCreateRunnerForm: () => {},
      onCreateRunnerOpenChange: () => {},
      onCreateRunnerSubmit: () => {},
      onCreateIDChange: () => {},
      onCreateRepositoryChange: () => {},
      onCreateRunnerSpecChange: () => {},
      onCreateLabelsChange: () => {},
      onStatusFilterChange: () => {},
      onRepositoryFilterChange: () => {},
      onRunnerSpecFilterChange: () => {},
      onSelectRunner: () => {},
      onRetryRunner: () => {},
      onStopRunner: () => {},
      onCopySelectedID: () => {},
      onLoadLog: () => {},
      onSelectedLogChange: () => {},
    }),
  )
}

describe("RunnerRequestsSection", () => {
  test("labels completed_at as a completed time", async () => {
    await i18n.changeLanguage("zh")
    try {
      const html = renderSelectedRunner()
      expect(html).toContain("完成时间")
      expect(html).not.toContain("已完成2026")
    } finally {
      await i18n.changeLanguage("en")
    }
  })
})
