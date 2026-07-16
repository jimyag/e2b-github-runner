import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { SiteHeader } from "./site-header"
import { SidebarProvider } from "./ui/sidebar"

describe("SiteHeader", () => {
  test("uses the signed-in account menu instead of a standalone theme toggle", () => {
    const html = renderToStaticMarkup(
      createElement(
        SidebarProvider,
        null,
        createElement(SiteHeader, {
          authSession: {
            authenticated: true,
            oauth_enabled: true,
            login: "miclle",
            role: "admin",
            avatar_url: "https://avatars.example.test/miclle.png",
          },
          onSignOut: () => {},
        }),
      ),
    )

    expect(html).toContain('aria-label="Account menu"')
    expect(html).toContain('src="https://avatars.example.test/miclle.png"')
    expect(html).not.toContain("Toggle theme")
  })
})
