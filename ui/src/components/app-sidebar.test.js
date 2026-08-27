import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { AppSidebar } from "./app-sidebar"
import { SidebarProvider } from "./ui/sidebar"
import i18n from "../i18n"

describe("AppSidebar", () => {
  test("renders administrator navigation in Chinese", async () => {
    await i18n.changeLanguage("zh")
    try {
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

      expect(html).toContain("Runner 规格")
      expect(html).toContain("Sandbox 服务")
      expect(html).toContain("诊断")
    } finally {
      await i18n.changeLanguage("en")
    }
  })

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
		expect(html).not.toContain("Runner Groups")
		expect(html).not.toContain("Runner Policies")
    expect(html).toContain('data-size="lg"')
    expect(html).toContain("h-12 text-sm")
    expect(html).not.toContain("Connected")
    expect(html).not.toContain("repo/org")
    expect(html).not.toContain("@miclle")
    expect(html).not.toContain("Sign out")
  })
})
