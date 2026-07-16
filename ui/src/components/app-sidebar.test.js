import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { AppSidebar } from "./app-sidebar"
import { SidebarProvider } from "./ui/sidebar"

describe("AppSidebar", () => {
  test("keeps navigation without duplicating status and account controls", () => {
    const html = renderToStaticMarkup(
      createElement(
        SidebarProvider,
        null,
        createElement(AppSidebar, {
          section: "runner_specs",
          onSectionChange: () => {},
        }),
      ),
    )

    expect(html).toContain("Runner Specs")
    expect(html).toContain("Accounts")
    expect(html).not.toContain("Connected")
    expect(html).not.toContain("repo/org")
    expect(html).not.toContain("@miclle")
    expect(html).not.toContain("Sign out")
  })
})
