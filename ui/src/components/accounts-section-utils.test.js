import { describe, expect, test } from "bun:test"

import { accountAvatarURL, accountListQuery, accountPageMeta } from "./accounts-section-utils"
import * as AccountsSectionUtils from "./accounts-section-utils"

describe("accountAvatarURL", () => {
  test("uses the linked GitHub identity even when it is not first", () => {
    expect(
      accountAvatarURL([
        { oauth_provider: "gitlab", oauth_login: "miclle-lab" },
        { oauth_provider: " GitHub ", oauth_login: " miclle " },
      ]),
    ).toBe("https://github.com/miclle.png?size=96")
  })

  test("keeps the initial fallback for accounts without a GitHub identity", () => {
    expect(accountAvatarURL([{ oauth_provider: "gitlab", oauth_login: "miclle-lab" }])).toBe("")
  })

  test("keeps the initial fallback when identities are missing", () => {
    expect(accountAvatarURL(undefined)).toBe("")
  })

  test("only suppresses the avatar URL that failed", () => {
    const accountAvatarImageURL = AccountsSectionUtils.accountAvatarImageURL
    expect(typeof accountAvatarImageURL).toBe("function")
    if (typeof accountAvatarImageURL !== "function") return

    const firstIdentities = [{ oauth_provider: "github", oauth_login: "miclle" }]
    const firstURL = accountAvatarURL(firstIdentities)
    expect(accountAvatarImageURL(firstIdentities, firstURL)).toBe("")

    const renamedIdentities = [{ oauth_provider: "github", oauth_login: "miclle-renamed" }]
    expect(accountAvatarImageURL(renamedIdentities, firstURL)).toBe(
      "https://github.com/miclle-renamed.png?size=96",
    )
  })
})

describe("accountListQuery", () => {
  test("builds trimmed search, role, and pagination parameters", () => {
    expect(accountListQuery({ query: " Octo ", role: "admin", limit: 20, offset: 40 })).toBe(
      "q=Octo&role=admin&limit=20&offset=40",
    )
  })

  test("omits inactive filters", () => {
    expect(accountListQuery({ query: "  ", role: "all", limit: 10, offset: 0 })).toBe(
      "limit=10&offset=0",
    )
  })
})

describe("accountPageMeta", () => {
  test("calculates the last page and available directions", () => {
    expect(accountPageMeta(41, 20, 40)).toEqual({
      page: 3,
      pages: 3,
      canPrevious: true,
      canNext: false,
    })
  })

  test("keeps an empty result on page one", () => {
    expect(accountPageMeta(0, 20, 0)).toEqual({
      page: 1,
      pages: 1,
      canPrevious: false,
      canNext: false,
    })
  })
})
