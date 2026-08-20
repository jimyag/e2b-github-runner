import { afterAll, afterEach, describe, expect, test } from "bun:test"
import { Window } from "happy-dom"
import { act, createElement } from "react"
import { createRoot } from "react-dom/client"

const window = new Window({ url: "http://localhost/admin/diagnostics" })
const domGlobals = {
  window,
  document: window.document,
  navigator: window.navigator,
  HTMLElement: window.HTMLElement,
  SVGElement: window.SVGElement,
  Node: window.Node,
  DocumentFragment: window.DocumentFragment,
  Event: window.Event,
  MouseEvent: window.MouseEvent,
  getComputedStyle: window.getComputedStyle.bind(window),
  requestAnimationFrame: window.requestAnimationFrame.bind(window),
  cancelAnimationFrame: window.cancelAnimationFrame.bind(window),
  IS_REACT_ACT_ENVIRONMENT: true,
}
const originalGlobalDescriptors = new Map(Object.keys(domGlobals).map((key) => [
  key,
  Object.getOwnPropertyDescriptor(globalThis, key),
]))
for (const [key, value] of Object.entries(domGlobals)) {
  Object.defineProperty(globalThis, key, { configurable: true, writable: true, value })
}

const { ReleaseReadinessSection } = await import("./release-readiness-section")
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

function readinessFixture(specName = "qiniu-ubuntu-24.04") {
  return {
    window_start: "2026-08-17T00:00:00Z",
    window_end: "2026-08-20T00:00:00Z",
    replay: {
      request_count: 12,
      distinct_input_count: 3,
      same: 12,
      legacy_only: 0,
      enabled_only: 0,
      different_profile: 0,
      errors: 0,
      truncated: false,
    },
    replay_samples: [],
    specs: [{
      name: specName,
      workflow_labels: ["qiniu", "ubuntu-24.04"],
      request_count: 2,
      registered_requests: 2,
      completed_requests: 1,
      cleanup_finalized_requests: 1,
      latest: {
        request_id: "request-24",
        repository_full_name: "owner/repo",
        workflow_job_id: 42,
        github_job_url: "https://github.com/owner/repo/actions/runs/1/job/42",
        requested_labels: ["qiniu", "ubuntu-24.04"],
        registered_at: "2026-08-19T01:00:00Z",
        completed_at: "2026-08-19T01:10:00Z",
        cleanup_finalized_at: "2026-08-19T01:10:00Z",
      },
    }],
    catalog_changes: [],
    catalog_changes_truncated: false,
    automated_gates_passed: true,
    gates: [
      { code: "window_at_least_72_hours", passed: true },
      { code: "catalog_unchanged", passed: true },
      { code: "matcher_parity", passed: true },
      { code: "all_enabled_specs_full_lifecycle", passed: true },
    ],
    manual_requirements: [
      "backup_restore_verified",
      "continuous_service_observation",
      "workflow_labels_unchanged",
    ],
    current_process: {
      started_at: "2026-08-20T09:17:21Z",
      catalog_match_counts: {
        same: 4,
        legacy_only: 0,
        enabled_only: 0,
        different_profile: 0,
      },
    },
  }
}

async function mountReadiness(request) {
  const container = document.createElement("div")
  document.body.append(container)
  const root = createRoot(container)
  mountedRoots.push({ root, container })
  await act(async () => {
    root.render(createElement(ReleaseReadinessSection, { request }))
  })
  return { container }
}

async function settle() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

async function click(element) {
  await act(async () => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }))
  })
}

function button(container, name) {
  const found = [...container.querySelectorAll("button")].find((element) => element.textContent.trim() === name)
  if (!found) throw new Error(`button not found: ${name}`)
  return found
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe("ReleaseReadinessSection", () => {
  test("loads 72-hour durable evidence and separates automated gates from manual signoff", async () => {
    const paths = []
    const { container } = await mountReadiness(async (path) => {
      paths.push(path)
      return readinessFixture()
    })
    await settle()

    expect(paths).toEqual(["/diagnostics/catalog-migration-readiness?window_hours=72"])
    expect(container.textContent).toContain("Release A readiness")
    expect(container.textContent).toContain("Automated evidence passed")
    expect(container.textContent).toContain("Manual signoff still required")
    expect(container.textContent).toContain("Historical replay")
    expect(container.textContent).toContain("Current process")
    expect(container.textContent).toContain("qiniu-ubuntu-24.04")
    expect(container.textContent).toContain("qiniu, ubuntu-24.04")
    expect(container.textContent).toContain("Cleanup finalized")
    expect(container.querySelector('a[href="https://github.com/owner/repo/actions/runs/1/job/42"]')).not.toBeNull()
  })

  test("switches to the seven-day evidence window without reloading unrelated diagnostics", async () => {
    const paths = []
    const { container } = await mountReadiness(async (path) => {
      paths.push(path)
      return readinessFixture()
    })
    await settle()
    await click(button(container, "7 days"))
    await settle()

    expect(paths).toEqual([
      "/diagnostics/catalog-migration-readiness?window_hours=72",
      "/diagnostics/catalog-migration-readiness?window_hours=168",
    ])
  })

  test("shows a scoped error and retries the readiness request", async () => {
    let attempts = 0
    const { container } = await mountReadiness(async () => {
      attempts++
      if (attempts === 1) throw new Error("readiness unavailable")
      return readinessFixture()
    })
    await settle()

    expect(container.textContent).toContain("readiness unavailable")
    await click(button(container, "Retry"))
    await settle()
    expect(attempts).toBe(2)
    expect(container.textContent).toContain("qiniu-ubuntu-24.04")
    expect(container.textContent).not.toContain("readiness unavailable")
  })

  test("does not let an older window response overwrite the latest selection", async () => {
    const first = deferred()
    const second = deferred()
    let calls = 0
    const { container } = await mountReadiness(() => {
      calls++
      return calls === 1 ? first.promise : second.promise
    })
    await click(button(container, "7 days"))
    second.resolve(readinessFixture("newer-window-spec"))
    await settle()
    first.resolve(readinessFixture("stale-window-spec"))
    await settle()

    expect(container.textContent).toContain("newer-window-spec")
    expect(container.textContent).not.toContain("stale-window-spec")
  })

  test("shows bounded mismatch samples and catalog truncation as actionable evidence", async () => {
    const fixture = readinessFixture()
    fixture.automated_gates_passed = false
    fixture.gates[1].passed = false
    fixture.gates[2].passed = false
    fixture.replay.same = 11
    fixture.replay.different_profile = 1
    fixture.replay_samples = [{
      repository_full_name: "owner/mismatched-repo",
      labels: ["qiniu", "ubuntu-24.04"],
      request_count: 1,
      first_seen_at: "2026-08-19T01:00:00Z",
      last_seen_at: "2026-08-19T01:00:00Z",
      result: "different_profile",
      legacy_profile: "legacy-spec",
      enabled_profile: "enabled-spec",
    }]
    fixture.catalog_changes_truncated = true
    fixture.catalog_changes = [{
      id: 101,
      action: "profile.update",
      resource_type: "runner_profile",
      resource_id: "qiniu-ubuntu-24.04",
      created_at: "2026-08-19T02:00:00Z",
    }]
    const { container } = await mountReadiness(async () => fixture)
    await settle()

    expect(container.textContent).toContain("Automated evidence blocked")
    expect(container.textContent).toContain("Mismatch samples")
    expect(container.textContent).toContain("owner/mismatched-repo")
    expect(container.textContent).toContain("legacy-spec")
    expect(container.textContent).toContain("enabled-spec")
    expect(container.textContent).toContain("Only the newest 100 configuration changes are shown")
  })
})
