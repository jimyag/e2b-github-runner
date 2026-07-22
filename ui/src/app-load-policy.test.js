import { describe, expect, test } from "bun:test"

import {
  adminDataResources,
  adminPollingResources,
  shouldPollAdminSection,
  shouldPollUserRoute,
  userDataResources,
  userPollingResources,
  userRunnerRequestLimit,
  userRunnerRequestsPath,
} from "./app-load-policy"

describe("app load policy", () => {
  test.each([
    ["overview", ["runner_requests", "runner_specs", "runner_policies"]],
    ["runner_requests", ["runner_requests", "runner_specs"]],
    ["runner_specs", ["runner_specs", "runner_groups"]],
    ["runner_groups", ["runner_groups", "runner_specs"]],
    ["runner_policies", ["runner_policies", "runner_specs", "runner_groups"]],
    ["audit", ["audit_events"]],
    ["accounts", []],
    ["sandbox_service", []],
    ["match", []],
    ["diagnostics", []],
  ])("loads only data used by the %s admin section", (section, expected) => {
    expect(adminDataResources(section)).toEqual(expected)
  })

  test("polls only dynamic admin request surfaces", () => {
    expect(shouldPollAdminSection("overview")).toBe(true)
    expect(shouldPollAdminSection("runner_requests")).toBe(true)
    expect(shouldPollAdminSection("runner_specs")).toBe(false)
    expect(shouldPollAdminSection("audit")).toBe(false)
    expect(adminPollingResources("overview")).toEqual(["runner_requests"])
    expect(adminPollingResources("runner_requests")).toEqual(["runner_requests"])
    expect(adminPollingResources("runner_specs")).toEqual([])
  })

  test.each([
    ["/", ["github_app", "runner_requests"]],
    ["/github/pulls/octo/repo/12/jobs", ["github_app", "runner_requests"]],
    ["/github/runs/octo/repo/34/jobs", ["github_app", "runner_requests"]],
    ["/github/branches/octo/repo/deadbeef/jobs", ["github_app", "runner_requests"]],
    ["/jobs/manual/octo/repo/manual-1", ["github_app", "runner_requests"]],
    ["/repositories", ["github_app"]],
    ["/account/preferences", ["github_app", "preferences"]],
    ["/organizations/octo/sandbox-templates", ["github_app", "preferences"]],
    ["/jobs/job-1", []],
    ["/admin/", []],
  ])("loads only data used by user route %s", (path, expected) => {
    expect(userDataResources(path)).toEqual(expected)
  })

  test("polls only user job-list routes", () => {
    expect(shouldPollUserRoute("/")).toBe(true)
    expect(shouldPollUserRoute("/github/pulls/octo/repo/12/jobs")).toBe(true)
    expect(shouldPollUserRoute("/repositories")).toBe(false)
    expect(shouldPollUserRoute("/account/preferences")).toBe(false)
    expect(shouldPollUserRoute("/jobs/job-1")).toBe(false)
    expect(userPollingResources("/")).toEqual(["runner_requests"])
    expect(userPollingResources("/repositories")).toEqual([])
  })

  test("keeps the homepage light while making stable job routes resolve the bounded history", () => {
    expect(userRunnerRequestLimit("/", false)).toBe(100)
    expect(userRunnerRequestLimit("/github/pulls/octo/repo/12/jobs", false)).toBe(500)
    expect(userRunnerRequestLimit("/github/pulls/octo/repo/12/jobs", true)).toBe(100)
    expect(userRunnerRequestsPath(500, 0)).toBe("/user/runner_requests?limit=500&offset=0")
  })
})
