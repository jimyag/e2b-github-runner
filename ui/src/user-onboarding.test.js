import { describe, expect, test } from "bun:test"
import * as onboardingPolicy from "./user-onboarding"

import {
  productTourSteps,
  sandboxSetupCompletesProductTour,
  shouldStartProductTour,
} from "./user-onboarding"

describe("product tour onboarding", () => {
  test("starts automatically only from the Jobs home for a pending account", () => {
    expect(shouldStartProductTour("/jobs", { version: 1, status: "pending", tour_seen: false }, false)).toBe(true)
    expect(shouldStartProductTour("/jobs", { version: 1, status: "pending", tour_seen: true }, false)).toBe(false)
    expect(shouldStartProductTour("/repositories", { version: 1, status: "pending" }, false)).toBe(false)
    expect(shouldStartProductTour("/account/preferences", { version: 1, status: "pending" }, false)).toBe(false)
    expect(shouldStartProductTour("/jobs", { version: 1, status: "completed" }, false)).toBe(false)
    expect(shouldStartProductTour("/jobs", { version: 1, status: "skipped" }, false)).toBe(false)
    expect(shouldStartProductTour("/jobs", null, false)).toBe(false)
  })

  test("allows an explicit replay without resetting the persisted state", () => {
    expect(shouldStartProductTour("/jobs", { version: 1, status: "completed" }, true)).toBe(true)
    expect(shouldStartProductTour("/jobs", { version: 1, status: "skipped" }, true)).toBe(true)
  })

  test("records a finished first-run tour without marking Sandbox setup complete", () => {
    expect(
      onboardingPolicy.productTourStateAfterEnd?.(
        { version: 1, status: "pending", tour_seen: false },
        false,
        "finished",
      ),
    ).toEqual({ version: 1, status: "pending", tour_seen: true })
    expect(
      onboardingPolicy.productTourStateAfterEnd?.(
        { version: 1, status: "pending", tour_seen: false },
        false,
        "skipped",
      ),
    ).toEqual({ version: 1, status: "skipped", tour_seen: true })
    expect(
      onboardingPolicy.productTourStateAfterEnd?.(
        { version: 1, status: "completed", tour_seen: true },
        true,
        "finished",
      ),
    ).toBeNull()
  })

  test("describes the cross-route path from Jobs to Sandbox service settings", () => {
    expect(productTourSteps.map(({ id, route, target }) => ({ id, route, target }))).toEqual([
      { id: "welcome", route: "/jobs", target: "product-shell" },
      { id: "jobs", route: "/jobs", target: "jobs-nav" },
      { id: "repositories", route: "/jobs", target: "repositories-nav" },
      { id: "account-menu", route: "/jobs", target: "account-menu" },
      { id: "settings", route: "/account/preferences", target: "settings-tabs" },
      { id: "sandbox-service", route: "/account/preferences", target: "sandbox-service" },
    ])
  })

  test("completes only after this account has saved its own Sandbox API Key", () => {
    expect(
      sandboxSetupCompletesProductTour({
        sandbox: {
          mode: "custom",
          resolved_source: "custom",
          api_url: "https://sandbox.qiniu.com",
          api_key: { configured: true },
        },
      }),
    ).toBe(true)
    expect(
      sandboxSetupCompletesProductTour({
        sandbox: {
          mode: "custom",
          resolved_source: "admin_default",
          api_url: "",
          api_key: { configured: false },
        },
      }),
    ).toBe(false)
    expect(sandboxSetupCompletesProductTour(null)).toBe(false)
  })
})
