import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { LandingPage } from "./landing-page"

describe("LandingPage", () => {
  test("uses the same Qiniu Runner logo as the admin interface", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('data-brand-logo="qiniu-runner"')
    expect(html).toContain('aria-label="Qiniu Runner home"')
    expect(html).toContain("lucide-terminal")
  })

  test("presents the runner product before asking visitors to sign in", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain("Qiniu CI Runner")
    expect(html).toContain("GitHub Actions, powered by Qiniu Sandbox")
    expect(html).toContain("One sandbox per job")
    expect(html).toContain("queues the request until worker capacity is available")
    expect(html).toContain("How it works")
  })

  test("keeps product, documentation, and Jobs destinations explicit", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('href="#capabilities"')
    expect(html).toContain('href="#how-it-works"')
    expect(html).toContain('href="https://github.com/qiniu/ci-runner#documentation"')
    expect(html).toContain('href="/jobs"')
    expect(html).toContain('aria-label="Documentation"')
  })

  test("keeps the compact Jobs action accessible when its text is hidden", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('aria-label="Open Jobs"')
  })

  test("uses top-level page landmarks and gives the skip link a focus target", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    const header = html.indexOf("<header")
    const main = html.indexOf('<main id="main-content"')
    const footer = html.indexOf("<footer")

    expect(header).toBeGreaterThan(-1)
    expect(main).toBeGreaterThan(header)
    expect(footer).toBeGreaterThan(main)
    expect(html).toContain('tabindex="-1"')
  })

  test("renders accessible text contrast tokens for small technical labels", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).not.toContain("text-white/35")
    expect(html).not.toContain("text-[#073149]/65")
    expect(html).toContain("text-[#006b91]")
  })

  test("centers the workflow connector behind the step icons", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain("left-[41px]")
    expect(html).toContain("sm:left-[57px]")
    expect(html).toContain("-translate-x-1/2")
  })

  test("keeps the public landing page focused on the product when OAuth is unavailable", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('href="/jobs"')
    expect(html).toContain("Open Jobs")
    expect(html).not.toContain("Sign-in unavailable")
    expect(html).not.toContain("GitHub OAuth is required but is not configured")
    expect(html).not.toContain('href="/auth/github/login"')
  })

  test("keeps the landing page public for signed-in visitors", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain("Qiniu CI Runner")
    expect(html).toContain('href="/jobs"')
    expect(html).toContain("Open Jobs")
    expect(html).not.toContain("does not have access")
    expect(html).not.toContain("Sign out")
  })
})
