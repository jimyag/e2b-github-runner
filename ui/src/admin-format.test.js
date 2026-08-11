import { describe, expect, test } from "bun:test"

import { formatTime } from "./admin-format"

describe("formatTime", () => {
  test("formats timestamps with the selected application locale", () => {
    const timestamp = "2026-08-10T08:09:10Z"
    const date = new Date(timestamp)

    expect(formatTime(timestamp, "en")).toBe(date.toLocaleString("en"))
    expect(formatTime(timestamp, "zh")).toBe(date.toLocaleString("zh"))
  })
})
