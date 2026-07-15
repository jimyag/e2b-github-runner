import { describe, expect, test } from "bun:test"

import {
  availableSandboxAudienceAccounts,
  normalizeSandboxAudienceLogin,
  sandboxAudienceIdentityKey,
  sandboxAudienceSummary,
  sandboxConfigSourceLabel,
  sandboxServiceDefaultAPIURL,
  sandboxServiceDefaultStatus,
} from "./sandbox-service-default-utils"

describe("sandboxServiceDefaultStatus", () => {
  test("distinguishes enabled, disabled, and incomplete settings", () => {
    expect(sandboxServiceDefaultStatus({ enabled: true, configured: true })).toBe("Enabled")
    expect(sandboxServiceDefaultStatus({ enabled: false, configured: true })).toBe("Disabled")
    expect(sandboxServiceDefaultStatus({ enabled: true, configured: false })).toBe("Incomplete")
    expect(sandboxServiceDefaultStatus({ enabled: false, configured: false })).toBe("Incomplete")
  })
})

describe("sandboxServiceDefaultAPIURL", () => {
  const regions = [{ apiURL: "https://us-south-1-sandbox.qiniuapi.com" }]

  test("canonicalizes catalog regions without dropping saved endpoints", () => {
    expect(sandboxServiceDefaultAPIURL(" HTTPS://US-SOUTH-1-SANDBOX.QINIUAPI.COM/ ", regions)).toBe(
      "https://us-south-1-sandbox.qiniuapi.com",
    )
    expect(sandboxServiceDefaultAPIURL(" https://sandbox.example.test/ ", regions)).toBe(
      "https://sandbox.example.test/",
    )
    expect(sandboxServiceDefaultAPIURL("   ", regions)).toBe("")
  })
})

describe("sandboxConfigSourceLabel", () => {
  test("uses operator-facing source labels", () => {
    expect(sandboxConfigSourceLabel("installation")).toBe("GitHub installation")
    expect(sandboxConfigSourceLabel("account")).toBe("Account")
    expect(sandboxConfigSourceLabel("inherited_account")).toBe("Inherited account")
    expect(sandboxConfigSourceLabel("admin_default")).toBe("Admin default")
    expect(sandboxConfigSourceLabel("request_snapshot")).toBe("Saved request snapshot")
    expect(sandboxConfigSourceLabel("")).toBe("—")
  })
})

describe("sandbox audience helpers", () => {
  test("normalizes manually entered GitHub logins", () => {
    expect(normalizeSandboxAudienceLogin("  @Octo-Org ")).toBe("Octo-Org")
    expect(normalizeSandboxAudienceLogin("octocat")).toBe("octocat")
  })

  test("keys and filters accounts by stable GitHub identity", () => {
    const available = [
      { github_account_id: 100, account_type: "user", account_login: "alice" },
      { github_account_id: 9001, account_type: "organization", account_login: "octo-org" },
    ]
    const selected = [
      { id: 1, github_account_id: 9001, account_type: "organization", account_login: "renamed-org" },
    ]

    expect(sandboxAudienceIdentityKey(available[1])).toBe("organization:9001")
    expect(availableSandboxAudienceAccounts(available, selected)).toEqual([available[0]])
  })

  test("summarizes all, selected, and selected-empty modes", () => {
    expect(sandboxAudienceSummary("all", 0)).toBe("Available to all GitHub accounts")
    expect(sandboxAudienceSummary("selected", 0)).toBe("No GitHub accounts selected")
    expect(sandboxAudienceSummary("selected", 2)).toBe("Available to 2 selected accounts")
  })
})
