import { expect, test, type Page } from "@playwright/test"

import type { CatalogMigrationReadiness, RunnerJobGroup, RunnerState } from "../src/admin-types"
import { getLocalAuthSessionRoute } from "./production-smoke-support"

const postRenderObservationMs = 1_000

test("boots the public landing page from the production bundle", async ({ page }) => {
  const diagnostics = observeBrowserDiagnostics(page)

  await routeLocalAnonymousSession(page)

  const response = await page.goto("/", { waitUntil: "networkidle" })

  expect(response?.ok()).toBe(true)
  await expect(
    page.getByRole("heading", { name: "GitHub Actions, powered by Qiniu Sandbox" }),
  ).toBeVisible()
  await expect(page.locator("#root")).not.toBeEmpty()
  await page.waitForTimeout(postRenderObservationMs)
  diagnostics.expectClean()
})

test("serves the hosted guide as a responsive public production route", async ({ page }) => {
  const diagnostics = observeBrowserDiagnostics(page)

  await routeLocalAnonymousSession(page)
  await page.setViewportSize({ width: 390, height: 844 })
  const response = await page.goto("/docs/getting-started/hosted", { waitUntil: "networkidle" })

  expect(response?.ok()).toBe(true)
  await expect(
    page.getByRole("heading", { name: "Get started with the hosted service", exact: true }),
  ).toBeVisible()
  await expect(page.getByRole("link", { name: "Copy a complete workflow" })).toHaveAttribute(
    "href",
    "/docs/guides/workflow",
  )
  await expect(page.locator("#root")).not.toBeEmpty()

  const viewport = await page.evaluate(() => ({
    documentWidth: document.documentElement.scrollWidth,
    viewportWidth: window.innerWidth,
  }))
  expect(viewport.documentWidth).toBeLessThanOrEqual(viewport.viewportWidth + 1)

  await page.waitForTimeout(postRenderObservationMs)
  diagnostics.expectClean()
})

test("serves the custom template guide from the production bundle", async ({ page }) => {
  const diagnostics = observeBrowserDiagnostics(page)

  await routeLocalAnonymousSession(page)
  await page.setViewportSize({ width: 390, height: 844 })
  const response = await page.goto("/docs/guides/custom-templates", { waitUntil: "networkidle" })

  expect(response?.ok()).toBe(true)
  await expect(
    page.getByRole("heading", { name: "Build and use a custom runner template", exact: true }),
  ).toBeVisible()
  await expect(page).toHaveTitle("Build and use a custom runner template · Qiniu CI Runner")
  await expect(page.locator('meta[name="description"]')).toHaveAttribute(
    "content",
    "Build a private Qiniu Sandbox template for your own tools, connect it to a custom Runner Spec, and select it from a GitHub Actions workflow.",
  )
  await expect(
    page.locator("pre code").filter({ hasText: "qshell sandbox template build --wait" }),
  ).toBeVisible()
  await expect(page.getByText("Status: ready", { exact: true }).first()).toBeVisible()

  const viewport = await page.evaluate(() => ({
    documentWidth: document.documentElement.scrollWidth,
    viewportWidth: window.innerWidth,
  }))
  expect(viewport.documentWidth).toBeLessThanOrEqual(viewport.viewportWidth + 1)

  await page.waitForTimeout(postRenderObservationMs)
  diagnostics.expectClean()
})

test("renders durable Release A evidence in Admin Diagnostics", async ({ page }) => {
  test.skip(Boolean(process.env.RUNNERD_UI_SMOKE_BASE_URL), "local fixture coverage only")

  const diagnostics = observeBrowserDiagnostics(page)
  await page.route("**/auth/session", async (route) => {
    await route.fulfill({
      json: { authenticated: true, oauth_enabled: true, login: "fixture-admin", role: "admin" },
    })
  })
  await page.route("**/diagnostics/pprof", async (route) => {
    await route.fulfill({
      json: {
        pprof: [],
        state: { backend: "sqlite", database: "/fixture/runnerd.db" },
        github: { auth_mode: "app", api_base_url: "https://api.github.com" },
        recent_failures: [],
      },
    })
  })
  await page.route("**/diagnostics/vars", async (route) => {
    await route.fulfill({ json: { e2b_runner_catalog_match_migration_total: { same: 10 } } })
  })
  await page.route("**/diagnostics/catalog-migration-readiness?window_hours=72", async (route) => {
    await route.fulfill({ json: readinessFixture() })
  })

  const response = await page.goto("/admin/diagnostics", { waitUntil: "networkidle" })
  expect(response?.ok()).toBe(true)
  const readiness = page.getByRole("region", { name: "Release A readiness" })
  await expect(readiness).toBeVisible()
  await expect(readiness.getByText("Automated evidence passed", { exact: true })).toBeVisible()
  await expect(readiness.getByText("Manual signoff still required", { exact: true })).toBeVisible()
  await expect(readiness.getByRole("cell", { name: "qiniu-ubuntu-24.04", exact: true })).toBeVisible()
  await expect(readiness.getByRole("link", { name: "fixture-request" })).toHaveAttribute(
    "href",
    "https://github.com/qiniu/ci-runner/actions/runs/1/job/42",
  )
  await readiness.getByRole("button", { name: "Inspect attempts for qiniu-ubuntu-24.04" }).click()
  await expect(readiness.getByText("fixture/repository-failed", { exact: true })).toBeVisible()
  await expect(readiness.getByText("sandbox_create · sandbox_capacity", { exact: true })).toBeVisible()
  await expect(readiness.getByRole("link", { name: "fixture-attempt" })).toHaveAttribute(
    "href",
    "https://github.com/fixture/repository-failed/actions/runs/2/job/84",
  )
  await page.waitForTimeout(postRenderObservationMs)
  diagnostics.expectClean()
})

test("keeps the Jobs list independently scrollable beside the Web Console", async ({ page }) => {
  test.skip(Boolean(process.env.RUNNERD_UI_SMOKE_BASE_URL), "local fixture coverage only")

  const diagnostics = observeBrowserDiagnostics(page)
  const runners = fixtureRunners(40)
  const selected = runners[0]
  const selectedGroup: RunnerJobGroup = {
    key: `branch:${selected.repository_full_name}:${selected.head_branch}:${selected.head_sha}`,
    group: "branch",
    repository: selected.repository_full_name || "fixture/repository-0",
    title: selected.head_branch || "fixture-branch-0",
    subtitle: selected.head_sha || "0".repeat(40),
    updated_at: selected.updated_at,
    jobs: [selected],
    current_jobs: [selected],
    previous_jobs: [],
    workflow_run_ids: [selected.workflow_run_id || 1],
    head_sha: selected.head_sha,
    head_branch: selected.head_branch,
  }

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.route("**/auth/session", async (route) => {
    await route.fulfill({
      json: {
        authenticated: true,
        oauth_enabled: true,
        login: "fixture-user",
        role: "user",
      },
    })
  })
  await page.route("**/user/**", async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname === "/user/github-app") {
      await route.fulfill({
        json: {
          setup_url: "/github-app/setup",
          installations: [{
            id: 1,
            account_id: 1,
            installation_id: 101,
            account_login: "fixture-user",
            repositories: runners.map((runner) => runner.repository_full_name),
            created_at: "2026-08-12T00:00:00Z",
            updated_at: "2026-08-12T00:00:00Z",
          }],
        },
      })
      return
    }
    if (url.pathname === "/user/runner_requests") {
      await route.fulfill({
        headers: { "X-Total-Count": String(runners.length) },
        json: runners,
      })
      return
    }
    if (url.pathname === "/user/onboarding/product-tour") {
      await route.fulfill({
        json: { version: 1, status: "completed", tour_seen: true },
      })
      return
    }
    if (url.pathname.startsWith("/user/github/branches/")) {
      await route.fulfill({ json: selectedGroup })
      return
    }
    if (url.pathname.endsWith("/github-log")) {
      await route.fulfill({
        contentType: "text/plain",
        body: Array.from({ length: 200 }, (_, index) => `fixture log line ${index + 1}`).join("\n"),
      })
      return
    }
    if (url.pathname.includes("/logs/")) {
      await route.fulfill({ contentType: "text/plain", body: "fixture runner log\n" })
      return
    }
    await route.fulfill({ status: 404, body: "fixture route not found" })
  })

  const response = await page.goto("/jobs", { waitUntil: "networkidle" })
  expect(response?.ok()).toBe(true)

  const jobList = page.locator("main aside .overflow-y-auto")
  const webConsoleTab = page.getByRole("tab", { name: /Web Console|Web 控制台/ })
  await expect(jobList).toBeVisible()
  await expect(webConsoleTab).toBeVisible()
  await webConsoleTab.click()

  const webConsole = page.getByRole("tabpanel", { name: /Web Console|Web 控制台/ })
  await expect(webConsole).toBeVisible()
  const consoleBefore = await webConsole.boundingBox()
  expect(consoleBefore).not.toBeNull()

  const layoutBefore = await page.evaluate(() => ({
    documentHeight: document.documentElement.scrollHeight,
    viewportHeight: window.innerHeight,
  }))
  expect(layoutBefore.documentHeight).toBeLessThanOrEqual(layoutBefore.viewportHeight + 1)

  const listBefore = await jobList.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }))
  expect(listBefore.scrollHeight).toBeGreaterThan(listBefore.clientHeight)

  await jobList.hover()
  await page.mouse.wheel(0, 600)
  await expect.poll(() => jobList.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  expect(await page.evaluate(() => window.scrollY)).toBe(0)

  const consoleAfter = await webConsole.boundingBox()
  expect(consoleAfter).not.toBeNull()
  expect(consoleAfter?.y).toBeCloseTo(consoleBefore?.y || 0, 0)

  await page.setViewportSize({ width: 1024, height: 700 })
  const narrowLayout = await page.evaluate(() => {
    const main = document.querySelector("main")
    return {
      documentHeight: document.documentElement.scrollHeight,
      mainOverflowY: main ? getComputedStyle(main).overflowY : "missing",
      viewportHeight: window.innerHeight,
    }
  })
  expect(narrowLayout.documentHeight).toBeGreaterThan(narrowLayout.viewportHeight)
  expect(narrowLayout.mainOverflowY).not.toBe("hidden")

  await page.waitForTimeout(postRenderObservationMs)
  diagnostics.expectClean()
})

async function routeLocalAnonymousSession(page: Page) {
  const authSessionRoute = getLocalAuthSessionRoute(process.env.RUNNERD_UI_SMOKE_BASE_URL)
  if (!authSessionRoute) return

  await page.route(authSessionRoute.pattern, async (route) => {
    await route.fulfill({ json: authSessionRoute.json })
  })
}

function observeBrowserDiagnostics(page: Page) {
  const consoleErrors: string[] = []
  const pageErrors: string[] = []
  const failedAssets: string[] = []

  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text())
    }
  })
  page.on("pageerror", (error) => {
    pageErrors.push(error.message)
  })
  page.on("requestfailed", (request) => {
    if (["script", "stylesheet"].includes(request.resourceType())) {
      failedAssets.push(`${request.method()} ${request.url()}: ${request.failure()?.errorText ?? "failed"}`)
    }
  })
  page.on("response", (response) => {
    if (
      response.status() >= 400 &&
      ["script", "stylesheet"].includes(response.request().resourceType())
    ) {
      failedAssets.push(`${response.status()} ${response.url()}`)
    }
  })

  return {
    expectClean() {
      expect(pageErrors, "page errors").toEqual([])
      expect(consoleErrors, "console errors").toEqual([])
      expect(failedAssets, "failed script or stylesheet requests").toEqual([])
    },
  }
}

function fixtureRunners(count: number): RunnerState[] {
  return Array.from({ length: count }, (_, index) => {
    const createdAt = new Date(Date.UTC(2026, 7, 12, 0, 0, count - index)).toISOString()
    const id = `fixture-job-${index}`
    return {
      id,
      status: index === 0 ? "running" : "completed",
      repository_full_name: `fixture/repository-${index}`,
      requested_labels: ["self-hosted", "e2b"],
      runner_spec_name: "fixture-spec",
      runner_name: `fixture-runner-${index}`,
      sandbox_id: index === 0 ? "fixture-sandbox" : undefined,
      workflow_job_id: 10_000 + index,
      workflow_run_id: 20_000 + index,
      workflow_name: "Fixture workflow",
      head_branch: `fixture-branch-${index}`,
      head_sha: index.toString(16).padStart(40, "0"),
      assigned_job_name: `Fixture job ${index}`,
      github_job_url: `https://github.com/fixture/repository-${index}/actions/runs/${20_000 + index}/job/${10_000 + index}`,
      created_at: createdAt,
      updated_at: createdAt,
      running_at: index === 0 ? createdAt : undefined,
      completed_at: index === 0 ? undefined : createdAt,
    }
  })
}

function readinessFixture(): CatalogMigrationReadiness {
  return {
    window_start: "2026-08-17T00:00:00Z",
    window_end: "2026-08-20T00:00:00Z",
    replay: {
      request_count: 10,
      distinct_input_count: 2,
      same: 10,
      legacy_only: 0,
      enabled_only: 0,
      different_profile: 0,
      errors: 0,
      truncated: false,
    },
    replay_samples: [],
    specs: [{
      name: "qiniu-ubuntu-24.04",
      workflow_labels: ["qiniu", "ubuntu-24.04"],
      request_count: 1,
      registered_requests: 1,
      completed_requests: 1,
      cleanup_finalized_requests: 1,
      latest: {
        request_id: "fixture-request",
        repository_full_name: "qiniu/ci-runner",
        workflow_job_id: 42,
        github_job_url: "https://github.com/qiniu/ci-runner/actions/runs/1/job/42",
        requested_labels: ["qiniu", "ubuntu-24.04"],
        registered_at: "2026-08-19T00:00:00Z",
        completed_at: "2026-08-19T00:10:00Z",
        cleanup_finalized_at: "2026-08-19T00:10:00Z",
      },
      recent_attempts: [{
        request_id: "fixture-attempt",
        repository_full_name: "fixture/repository-failed",
        status: "failed",
        workflow_job_id: 84,
        github_job_url: "https://github.com/fixture/repository-failed/actions/runs/2/job/84",
        requested_labels: ["qiniu", "ubuntu-24.04"],
        failure_stage: "sandbox_create",
        failure_reason: "sandbox_capacity",
        queued_at: "2026-08-19T02:00:00Z",
      }],
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
      started_at: "2026-08-20T00:00:00Z",
      catalog_match_counts: {
        same: 10,
        legacy_only: 0,
        enabled_only: 0,
        different_profile: 0,
      },
    },
  }
}
