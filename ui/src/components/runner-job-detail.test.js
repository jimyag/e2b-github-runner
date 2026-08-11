import { describe, expect, test } from "bun:test"

import i18n from "../i18n"
import { runnerJobDetailRows } from "./runner-job-detail-rows"

describe("runnerJobDetailRows", () => {
  test("keeps failed time and failure reason as distinct stable fields", () => {
    const failedAt = "2026-08-10T08:09:10Z"
    const rows = runnerJobDetailRows(
      {
        id: "job-1",
        status: "failed",
        failed_at: failedAt,
        failure_reason: "runner exited",
      },
      i18n.getFixedT("en"),
      "en",
    )

    expect(new Set(rows.map((row) => row.id)).size).toBe(rows.length)
    expect(rows.find((row) => row.id === "failed_at")).toEqual({
      id: "failed_at",
      label: "Failed at",
      value: new Date(failedAt).toLocaleString("en"),
    })
    expect(rows.find((row) => row.id === "failure_reason")).toEqual({
      id: "failure_reason",
      label: "Failure",
      value: "runner exited",
    })
  })
})
