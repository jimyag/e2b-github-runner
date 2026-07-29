import { describe, expect, mock, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

mock.module("xterm", () => ({
  Terminal: class Terminal {},
}))
mock.module("xterm-addon-fit", () => ({
  FitAddon: class FitAddon {},
}))

const { UserDashboard } = await import("./user-dashboard")

describe("Sandbox service Settings", () => {
  test("keeps the supported region selector in Settings without a custom endpoint control", () => {
    const html = renderToStaticMarkup(createElement(UserDashboard, {
      authSession: {
        authenticated: true,
        oauth_enabled: true,
        login: "miclle",
        role: "user",
      },
      githubApp: {
        setup_url: "/github-app/setup",
        installations: [{
          id: 1,
          account_id: 1,
          installation_id: 987,
          account_type: "user",
          account_login: "miclle",
          repositories: [],
          created_at: "2026-07-29T00:00:00Z",
          updated_at: "2026-07-29T00:00:00Z",
        }],
      },
      locationPath: "/account/preferences",
      productTourOnboarding: null,
      userPreferences: {
        sandbox: {
          mode: "custom",
          resolved_source: "none",
          api_url: "",
          api_key: { configured: false },
        },
      },
      userPreferencesScope: "account",
      runners: [],
      runnerTotal: 0,
      loadingRunnerHistory: false,
      selectedKey: "",
      selectedJobID: "",
      page: "settings",
      selectedRepositoryAccountLogin: "miclle",
      accountSettingsRoute: { tab: "preferences" },
      authorizedRepositories: { 1: ["miclle/qiniu-ci-runner"] },
      repositoryErrors: {},
      loadingRepositoriesFor: null,
      syncingGitHubInstallations: false,
      onLoadAuthorizedRepositories: () => {},
      onSyncGitHubInstallations: () => {},
      onNavigateProductTourStart: () => {},
      onSaveProductTourOnboarding: async () => {},
      onSaveSandboxConfig: async () => {},
      onDeleteSandboxAPIKey: async () => {},
      onNavigate: () => {},
      onNavigateRepositoryAccount: () => {},
      onNavigateAccountSettings: () => {},
      onOpenJob: () => {},
      onLoadJobGroup: async () => null,
      onLoadRunnerHistory: () => {},
      request: async () => [],
      onSelectKey: () => {},
      onSignOut: () => {},
    }))

    expect(html).toContain("Select Sandbox region")
    expect(html).not.toContain("Custom endpoint")
    expect(html).not.toContain("https://sandbox.example.test")
  })
})
