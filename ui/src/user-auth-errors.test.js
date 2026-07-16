import { describe, expect, test } from "bun:test"

import {
  createGitHubReauthenticationGate,
  requiresGitHubReauthentication,
} from "./user-auth-errors"

describe("GitHub reauthentication errors", () => {
  test("recognizes the structured server error code", () => {
    const error = Object.assign(new Error("sign in again"), { code: "REAUTH_REQUIRED" })

    expect(requiresGitHubReauthentication(error)).toBe(true)
    expect(requiresGitHubReauthentication(new Error("temporary failure"))).toBe(false)
  })

  test("starts at most one browser redirect while polling continues", () => {
    const beginReauthentication = createGitHubReauthenticationGate()

    expect(beginReauthentication()).toBe(true)
    expect(beginReauthentication()).toBe(false)
  })
})
