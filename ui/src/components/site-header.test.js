import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { SiteHeader } from "./site-header"
import { SidebarProvider } from "./ui/sidebar"

describe("SiteHeader", () => {
  test("keeps signed-in preferences inside the account menu", () => {
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
    expect(html).toContain('href="/docs"')
    expect(html).toContain('aria-label="Documentation"')
    expect(html).toContain('src="https://avatars.example.test/miclle.png"')
    expect(html).not.toContain("Toggle theme")
    expect(html).not.toContain('data-language-switcher="true"')
  })
})
