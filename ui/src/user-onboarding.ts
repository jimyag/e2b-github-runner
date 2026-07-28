import type { ProductTourOnboarding, UserPreferences } from "@/admin-types"

export const productTourVersion = 1

export const productTourSteps = [
  { id: "welcome", route: "/jobs", target: "product-shell" },
  { id: "jobs", route: "/jobs", target: "jobs-nav" },
  { id: "repositories", route: "/jobs", target: "repositories-nav" },
  { id: "account-menu", route: "/jobs", target: "account-menu" },
  { id: "settings", route: "/account/preferences", target: "settings-tabs" },
  { id: "sandbox-service", route: "/account/preferences", target: "sandbox-service" },
] as const

export function shouldStartProductTour(
  path: string,
  onboarding: ProductTourOnboarding | null,
  replayRequested: boolean,
): boolean {
  if (path !== "/jobs" || !onboarding) return false
  return replayRequested || (onboarding.status === "pending" && !onboarding.tour_seen)
}

export function sandboxSetupCompletesProductTour(preferences: UserPreferences | null): boolean {
  return Boolean(
    preferences?.sandbox.api_key.configured &&
      preferences.sandbox.resolved_source === "custom" &&
      !preferences.sandbox.inherited,
  )
}

export function productTourStateAfterEnd(
  onboarding: ProductTourOnboarding | null,
  replay: boolean,
  status: "finished" | "skipped",
): ProductTourOnboarding | null {
  if (replay || onboarding?.status !== "pending") return null
  return {
    version: productTourVersion,
    status: status === "skipped" ? "skipped" : "pending",
    tour_seen: true,
  }
}

export function shouldShowSandboxSetupTask(
  onboarding: ProductTourOnboarding | null,
): boolean {
  return Boolean(onboarding && onboarding.status !== "completed")
}

export function shouldCompleteProductTour(
  onboarding: ProductTourOnboarding | null,
  preferences: UserPreferences | null,
): boolean {
  return shouldShowSandboxSetupTask(onboarding) &&
    sandboxSetupCompletesProductTour(preferences)
}
