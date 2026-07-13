import { describe, expect, test } from "bun:test"

import { formatOptionalTime } from "./sandbox-catalog-utils"

describe("formatOptionalTime", () => {
  test("renders invalid timestamps as unavailable", () => {
    expect(formatOptionalTime("not-a-date")).toBe("—")
  })

  test("renders empty and zero timestamps as unavailable", () => {
    expect(formatOptionalTime("")).toBe("—")
    expect(formatOptionalTime("0001-01-01T00:00:00Z")).toBe("—")
  })
})
