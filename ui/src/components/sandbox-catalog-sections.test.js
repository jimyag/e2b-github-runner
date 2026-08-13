import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import * as catalogUtils from "./sandbox-catalog-utils"
import { SandboxTemplateCatalog } from "./sandbox-catalog-sections"

const { formatOptionalTime } = catalogUtils

describe("sandbox regions", () => {
  test("shares overseas-first region metadata", () => {
    expect(catalogUtils.sandboxRegions).toEqual([
      {
        id: "us-south-1",
        label: "United States · Dallas 1",
        apiURL: "https://us-south-1-sandbox.qiniuapi.com",
      },
      {
        id: "cn-yangzhou-1",
        label: "China · Yangzhou 1",
        apiURL: "https://cn-yangzhou-1-sandbox.qiniuapi.com",
      },
    ])
  })
})

describe("formatOptionalTime", () => {
  test("formats timestamps with the selected application locale", () => {
    const timestamp = "2026-08-10T08:09:10Z"
    const date = new Date(timestamp)

    expect(formatOptionalTime(timestamp, "en")).toBe(date.toLocaleString("en"))
    expect(formatOptionalTime(timestamp, "zh")).toBe(date.toLocaleString("zh"))
  })

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

describe("sandbox template catalog", () => {
  test("keeps public managed templates visible when the scoped provider catalog fails", () => {
    const html = renderToStaticMarkup(createElement(SandboxTemplateCatalog, {
      publicTemplates: [{
        default_template_name: "github-runner-ubuntu-24-04",
        runner_spec_names: ["qiniu-ubuntu-24.04", "qiniu-ubuntu-latest"],
        workflow_labels: [["qiniu", "ubuntu-24.04"], ["qiniu", "ubuntu-latest"]],
      }],
      publicLoading: false,
      publicError: "",
      scopedTemplates: [],
      scopedLoading: false,
      scopedError: "Sandbox credentials are not configured",
    }))

    expect(html).toContain("Managed runner templates")
    expect(html).toContain("github-runner-ubuntu-24-04")
    expect(html).toContain("qiniu-ubuntu-latest")
    expect(html).toContain("qiniu, ubuntu-latest")
    expect(html).toContain("Provider templates")
    expect(html).toContain("Sandbox credentials are not configured")
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
