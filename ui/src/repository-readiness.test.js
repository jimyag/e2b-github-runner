import { describe, expect, test } from "bun:test"
import * as readiness from "./repository-readiness"

describe("repository readiness", () => {
  test.each([
    ["custom", true, "Own credentials"],
    ["inherited", true, "Inherited credentials"],
    ["admin_default", true, "Platform provided"],
    ["none", false, "Configuration required"],
  ])("maps %s Sandbox source to an actionable state", (source, ready, label) => {
    expect(
      readiness.sandboxServiceReadiness?.({
        sandbox: {
          mode: source === "inherited" ? "inherit" : "custom",
          resolved_source: source,
          api_url: "",
          api_key: { configured: source === "custom" },
        },
      }),
    ).toMatchObject({ ready, label })
  })

  test("treats missing preferences as loading instead of misreporting a blocker", () => {
    expect(readiness.sandboxServiceReadiness?.(null)).toEqual({
      ready: false,
      loading: true,
      source: "none",
      label: "Checking configuration",
      description: "Resolving the effective Sandbox service for this account.",
    })
  })

  test("uses one canonical repository route while preserving organization scope", () => {
    expect(readiness.repositoryPath?.("miclle", "miclle")).toBe("/repositories")
    expect(readiness.repositoryPath?.("qbox", "miclle")).toBe("/organizations/qbox/repositories")
    expect(readiness.repositoryAccountLogin?.("/repositories", "miclle")).toBe("miclle")
    expect(readiness.repositoryAccountLogin?.("/account/repositories", "miclle")).toBe("miclle")
    expect(readiness.repositoryAccountLogin?.("/organizations/qbox/repositories", "miclle")).toBe("qbox")
    expect(readiness.repositoryAccountLogin?.("/account/preferences", "miclle")).toBeNull()
  })

  test("keeps repository and Sandbox scope on the same installation", () => {
    const installations = [
      { id: 7, account_login: "qbox" },
      { id: 8, account_login: "octo" },
    ]

    expect(
      readiness.selectRepositoryInstallation?.(installations, "miclle", "miclle"),
    ).toEqual(installations[0])
    expect(
      readiness.selectRepositoryInstallation?.(installations, "octo", "miclle"),
    ).toEqual(installations[1])
    expect(
      readiness.selectRepositoryInstallation?.(installations, "missing", "miclle"),
    ).toBeUndefined()
    expect(readiness.repositoryPreferenceScope?.(installations[0], "miclle")).toBe(
      "github_installation:7",
    )
    expect(
      readiness.repositoryPreferenceScope?.(
        { id: 9, account_login: "miclle" },
        "miclle",
      ),
    ).toBe("account")
  })

  test("annotates authorized repositories with local job activity", () => {
    expect(
      readiness.repositoryRows?.(
        ["Qiniu/Runner", "Qiniu/Docs"],
        ["qiniu/runner"],
      ),
    ).toEqual([
      { name: "Qiniu/Docs", hasJobs: false },
      { name: "Qiniu/Runner", hasJobs: true },
    ])
  })
})
