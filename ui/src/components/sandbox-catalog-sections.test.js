import { describe, expect, test } from "bun:test"

import * as catalogUtils from "./sandbox-catalog-utils"

const { formatOptionalTime } = catalogUtils

describe("formatOptionalTime", () => {
  test("renders invalid timestamps as unavailable", () => {
    expect(formatOptionalTime("not-a-date")).toBe("—")
  })

  test("renders empty and zero timestamps as unavailable", () => {
    expect(formatOptionalTime("")).toBe("—")
    expect(formatOptionalTime("0001-01-01T00:00:00Z")).toBe("—")
  })
})

describe("sandbox catalog loaders", () => {
  test("loads instances without fetching templates", async () => {
    const paths = []
    const request = async (path) => {
      paths.push(path)
      return []
    }

    expect(typeof catalogUtils.loadSandboxInstances).toBe("function")
    await catalogUtils.loadSandboxInstances(request, "us-south-1", 42, "template-1")

    expect(paths).toEqual([
      "/user/sandbox/instances?region=us-south-1&installation_id=42&template_id=template-1",
    ])
  })
})

describe("sandbox instances view state", () => {
  test("keeps the instances table available when only the template filter fails", () => {
    expect(
      catalogUtils.sandboxInstancesViewState({
        templatesLoading: false,
        instancesLoading: false,
        templatesError: "template catalog unavailable",
        instancesError: "",
      }),
    ).toEqual({
      loading: false,
      error: "",
      filterDisabled: true,
    })
  })
})
