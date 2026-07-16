import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import * as AccountsSectionModule from "./accounts-section"

const { AccountsSection } = AccountsSectionModule

describe("AccountsSection", () => {
  test("renders account search, role filter, and pagination controls", () => {
    const html = renderToStaticMarkup(
      createElement(AccountsSection, { request: async () => ({}) }),
    )

    expect(html).toContain("Accounts")
    expect(html).toContain("Search accounts")
    expect(html).toContain("Filter by role")
    expect(html).toContain("Page 1 of 1")
  })

  test("reserves enough space for the accounts-per-page label", () => {
    const html = renderToStaticMarkup(
      createElement(AccountsSection, { request: async () => ({}) }),
    )

    expect(html).toContain("min-w-32 shrink-0")
  })

  test("renders contextual account statistics", () => {
    const html = renderToStaticMarkup(
      createElement(AccountsSection, { request: async () => ({}) }),
    )

    expect(html).toContain("Administrators")
    expect(html).toContain("Users")
    expect(html).toContain("Linked identities")
    expect(html).toContain("local access principals")
    expect(html).toContain("full management access")
    expect(html).toContain("standard account access")
    expect(html).toContain("OAuth provider bindings")
  })

  test("renders a GitHub avatar with an initial fallback", () => {
    expect(typeof AccountsSectionModule.AccountAvatar).toBe("function")

    const html = renderToStaticMarkup(
      createElement(AccountsSectionModule.AccountAvatar, {
        displayLogin: "miclle",
        identities: [{ oauth_provider: "github", oauth_login: "miclle" }],
      }),
    )

    expect(html).toContain("https://github.com/miclle.png?size=96")
    expect(html).toContain(">M<")
  })
})
