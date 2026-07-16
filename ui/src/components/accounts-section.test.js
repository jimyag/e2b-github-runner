import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import * as AccountsSectionModule from "./accounts-section"

const { AccountsSection } = AccountsSectionModule

function collectText(node) {
  if (typeof node === "string" || typeof node === "number") return String(node)
  if (Array.isArray(node)) return node.map(collectText).join("")
  if (!node || typeof node !== "object") return ""
  return collectText(node.props?.children)
}

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
    expect(AccountsSectionModule.AccountAvatar.toString()).not.toContain("currentTarget.hidden")

    const html = renderToStaticMarkup(
      createElement(AccountsSectionModule.AccountAvatar, {
        displayLogin: "miclle",
        identities: [{ oauth_provider: "github", oauth_login: "miclle" }],
      }),
    )

    expect(html).toContain("https://github.com/miclle.png?size=96")
    expect(html).toContain(">M<")
  })

  test("keeps role update errors in the dialog and clears them on close", () => {
    const AccountRoleChangeDialog = AccountsSectionModule.AccountRoleChangeDialog
    expect(typeof AccountRoleChangeDialog).toBe("function")
    if (typeof AccountRoleChangeDialog !== "function") return

    let closeCount = 0
    const dialog = AccountRoleChangeDialog({
      pendingRoleChange: {
        account: { id: 2, role: "user", oauth_identities: [] },
        role: "admin",
      },
      pendingLogin: "octocat",
      saving: false,
      error: "role update failed",
      onClose: () => {
        closeCount++
      },
      onConfirm: () => {},
    })

    expect(collectText(dialog)).toContain("role update failed")
    dialog.props.onOpenChange(false)
    expect(closeCount).toBe(1)
  })
})
