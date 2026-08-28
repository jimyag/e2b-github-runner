import { afterAll, afterEach, describe, expect, test } from "bun:test"
import { Window } from "happy-dom"
import { act, createElement } from "react"
import { createRoot } from "react-dom/client"
import { renderToStaticMarkup } from "react-dom/server"

import { RunnerTypeForm, RunnerTypeItem, UserRunnerTypesSection } from "./user-runner-types-section"
import { runnerTypeCatalogRegion, runnerTypeOverridesGlobal, runnerTypeWorkflowYAML } from "./user-runner-types-utils"

const window = new Window({ url: "http://localhost/" })
const domGlobals = { window, document: window.document, navigator: window.navigator, HTMLElement: window.HTMLElement, SVGElement: window.SVGElement, Node: window.Node, DocumentFragment: window.DocumentFragment, Event: window.Event, MouseEvent: window.MouseEvent, getComputedStyle: window.getComputedStyle.bind(window), requestAnimationFrame: window.requestAnimationFrame.bind(window), cancelAnimationFrame: window.cancelAnimationFrame.bind(window), IS_REACT_ACT_ENVIRONMENT: true }
const originalGlobalDescriptors = new Map(Object.keys(domGlobals).map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)]))
for (const [key, value] of Object.entries(domGlobals)) Object.defineProperty(globalThis, key, { configurable: true, writable: true, value })
const mountedRoots = []

afterEach(async () => {
  for (const { root, container } of mountedRoots.splice(0)) {
    await act(async () => root.unmount())
    container.remove()
  }
})

afterAll(() => {
  for (const [key, descriptor] of originalGlobalDescriptors) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor)
    else Reflect.deleteProperty(globalThis, key)
  }
  window.close()
})

function deferred() {
  let resolve
  const promise = new Promise((resolvePromise) => { resolve = resolvePromise })
  return { promise, resolve }
}

async function settle() {
  await act(async () => { await Promise.resolve(); await Promise.resolve() })
}

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
  test("uses the configured Sandbox region for the scoped catalog", () => {
    expect(runnerTypeCatalogRegion("cn-yangzhou-1")).toBe("cn-yangzhou-1")
    expect(runnerTypeCatalogRegion("unknown-region")).toBe("us-south-1")
  })

  test("quotes workflow labels in copied YAML", () => {
    expect(runnerTypeWorkflowYAML(["foo: bar", "a,b", "#gpu"])).toBe('runs-on: ["foo: bar", "a,b", "#gpu"]')
  })

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
      form: { name: "", labels: "", templateID: "gpu-template", runnerGroup: "org-runners", maxConcurrency: "0", enabled: true },
      templates: [{ template_id: "gpu-template", aliases: [], build_status: "ready" }],
      allowRunnerGroup: true,
      saving: false,
      onChange() {},
      onSubmit() {},
      onClose() {},
    }))

    expect(html).toContain("gpu-template")
    expect(html).toContain("org-runners")
  })

  test("keeps the form fields visible and disables submit while saving", () => {
    const html = renderToStaticMarkup(createElement(RunnerTypeForm, {
      mode: "create",
      form: { name: "gpu", labels: "qiniu,gpu", templateID: "gpu-template", runnerGroup: "", maxConcurrency: "2", enabled: true },
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

  test("detects only exact platform label overrides", () => {
    expect(runnerTypeOverridesGlobal(["linux", "qiniu"], [managed])).toBe(false)
    expect(runnerTypeOverridesGlobal(["ubuntu-24.04", "qiniu"], [managed])).toBe(true)
  })

  test("ignores a stale manual refresh after switching organization scope", async () => {
    const staleRefresh = deferred()
    let scopeOneCalls = 0
    const request = (url) => {
      if (url === "/user/runner-specs?installation_id=1") {
        scopeOneCalls++
        if (scopeOneCalls === 1) return Promise.resolve({ items: [{ ...custom, name: "scope-one-initial" }] })
        return staleRefresh.promise
      }
      if (url === "/user/runner-specs?installation_id=2") return Promise.resolve({ items: [{ ...custom, name: "scope-two" }] })
      throw new Error(`unexpected request: ${url}`)
    }
    const container = document.createElement("div")
    document.body.append(container)
    const root = createRoot(container)
    mountedRoots.push({ root, container })
    await act(async () => root.render(createElement(UserRunnerTypesSection, { request, installationID: 1 })))
    await settle()
    const refresh = [...container.querySelectorAll("button")].find((button) => button.textContent === "Refresh")
    await act(async () => refresh.dispatchEvent(new MouseEvent("click", { bubbles: true })))
    await act(async () => root.render(createElement(UserRunnerTypesSection, { request, installationID: 2 })))
    await settle()
    expect(container.textContent).toContain("scope-two")

    await act(async () => staleRefresh.resolve({ items: [{ ...custom, name: "scope-one-stale" }] }))
    await settle()
    expect(container.textContent).toContain("scope-two")
    expect(container.textContent).not.toContain("scope-one-stale")
  })
})
