import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { RunnerTypeForm, RunnerTypeItem } from "./user-runner-types-section"

const managed = {
  name: "ubuntu-24.04",
  source: "managed",
  workflow_labels: ["qiniu", "ubuntu-24.04"],
  template_id: "managed-template",
  default_template_name: "ubuntu-24.04-x64",
  enabled: true,
  global_max_concurrency: 8,
  scope_max_concurrency: 3,
  effective_max_concurrency: 3,
  overrides_global: false,
  editable: true,
  scope_control_configured: true,
  updated_at: "2026-08-28T00:00:00Z",
}

const custom = {
  ...managed,
  name: "gpu",
  source: "scoped_custom",
  workflow_labels: ["qiniu", "gpu"],
  template_id: "gpu-template",
  default_template_name: "",
  scope_max_concurrency: 0,
  effective_max_concurrency: 0,
  overrides_global: true,
}

describe("UserRunnerTypesSection", () => {
  test("renders controls and effective details for managed and custom runner types", () => {
    const html = renderToStaticMarkup(createElement("div", null,
      createElement(RunnerTypeItem, { item: managed, copying: false, onCopy() {}, onEdit() {}, onDelete() {}, onControl() {}, onReset() {} }),
      createElement(RunnerTypeItem, { item: custom, copying: false, onCopy() {}, onEdit() {}, onDelete() {}, onControl() {}, onReset() {} }),
    ))

    expect(html).toContain("ubuntu-24.04-x64")
    expect(html).toContain("Global max concurrency")
    expect(html).toContain("Scope max concurrency")
    expect(html).toContain(`aria-label="Edit ${custom.name}"`)
    expect(html).toContain(`aria-label="Delete ${custom.name}"`)
    expect(html).toContain(`aria-label="Edit platform control ${managed.name}"`)
  })

  test("loads scope templates when creating a custom runner type", () => {
    const html = renderToStaticMarkup(createElement(RunnerTypeForm, {
      mode: "create",
      form: { name: "", labels: "", templateID: "gpu-template", maxConcurrency: "0", enabled: true },
      templates: [{ template_id: "gpu-template", aliases: [], build_status: "ready" }],
      saving: false,
      onChange() {},
      onSubmit() {},
      onClose() {},
    }))

    expect(html).toContain("gpu-template")
  })

  test("keeps the form fields visible and disables submit while saving", () => {
    const html = renderToStaticMarkup(createElement(RunnerTypeForm, {
      mode: "create",
      form: { name: "gpu", labels: "qiniu,gpu", templateID: "gpu-template", maxConcurrency: "2", enabled: true },
      templates: [],
      saving: true,
      onChange() {},
      onSubmit() {},
      onClose() {},
    }))

    expect(html).toContain('value="gpu"')
    expect(html).toContain('value="qiniu,gpu"')
    expect(html).toContain('value="gpu-template"')
    expect(html).toMatch(/<button[^>]*type="submit"[^>]*disabled/)
  })
})
