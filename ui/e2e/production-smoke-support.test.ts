import { expect, test } from "bun:test"

import { getLocalAuthSessionRoute } from "./production-smoke-support"

test("provides an anonymous auth session when the smoke owns the local preview", () => {
  expect(getLocalAuthSessionRoute(undefined)).toEqual({
    pattern: "**/auth/session",
    json: {
      authenticated: false,
      oauth_enabled: false,
    },
  })
})

test("does not replace the deployed origin auth session", () => {
  expect(getLocalAuthSessionRoute("https://runner.example.com")).toBeNull()
})
