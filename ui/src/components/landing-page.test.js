import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { LandingPage } from "./landing-page"

describe("LandingPage", () => {
  test("uses the same Qiniu CI Runner logo as the admin interface", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('data-brand-logo="qiniu-runner"')
    expect(html).toContain('aria-label="Qiniu CI Runner home"')
    expect(html).toContain(">CI Runner</span>")
    expect(html).toContain("lucide-terminal")
  })

  test("gives the shared brand mark enough presence in the primary navigation", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain("h-[18px] w-[18px]")
    expect(html).toContain("text-[17px]")
    expect(html).toContain("text-[11px]")
  })

  test("keeps the primary navigation legible beside the enlarged brand mark", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('gap-8 text-[15px] text-white/70')
  })

  test("links the Open source navigation item to the product boundary section", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    const productLink = html.indexOf('href="#capabilities"')
    const openSourceLink = html.indexOf('href="#open-source"')
    const howItWorksLink = html.indexOf('href="#how-it-works"')

    expect(openSourceLink).toBeGreaterThan(productLink)
    expect(howItWorksLink).toBeGreaterThan(openSourceLink)
    expect(html).toContain('id="open-source"')
    expect(html).toContain(">Open source</a>")
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

  test("distinguishes the open-source runner from the Qiniu cloud sandbox", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain("Open-source orchestration")
    expect(html).toContain("Qiniu CI Runner is open source")
    expect(html).toContain("View the source")
    expect(html).toContain("Qiniu Sandbox is a Qiniu-provided cloud service")
    expect(html).toContain('href="https://github.com/qiniu/ci-runner"')
  })

  test("links the open-source claim to the Apache-2.0 license", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain("Apache-2.0 licensed")
    expect(html).toContain(
      'href="https://github.com/qiniu/ci-runner/blob/main/LICENSE"',
    )
  })

  test("offers hosted and deployment paths without blurring the Sandbox boundary", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain("Use hosted service")
    expect(html).toContain("Deploy runnerd")
    expect(html).toContain('href="/jobs"')
    expect(html).toContain(
      'href="https://github.com/qiniu/ci-runner#quick-start"',
    )
    expect(html).toContain("Both run workflow jobs on Qiniu Sandbox")
    expect(html).toContain("cloud service operated by Qiniu")
    expect(html).toContain("GitHub.com")
    expect(html).toContain("Qiniu Sandbox credentials")
    expect(html).toContain("Self-hosted runner label")
  })

  test("keeps the source card content above its decorative grid", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).not.toContain('<article class="brand-grid')
    expect(html).toContain(
      'aria-hidden="true" class="brand-grid pointer-events-none absolute inset-0 opacity-40"',
    )
  })

  test("presents product value before ownership boundaries and implementation details", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    const productValue = html.indexOf("Capacity appears for the job")
    const ownershipBoundary = html.indexOf("Open where you orchestrate")
    const implementationDetails = html.indexOf("From webhook to clean slate")

    expect(productValue).toBeGreaterThan(-1)
    expect(ownershipBoundary).toBeGreaterThan(productValue)
    expect(implementationDetails).toBeGreaterThan(ownershipBoundary)
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

  test("keeps the compact access action accessible when its text is hidden", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('aria-label="Get started"')
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

  test("keeps the public CTA neutral and independent from the visitor session", () => {
    const html = renderToStaticMarkup(
      createElement(LandingPage),
    )

    expect(html).toContain('href="/jobs"')
    expect(html).toContain("Get started")
    expect(html).not.toContain("Open Jobs")
    expect(html).not.toContain("Sign-in unavailable")
    expect(html).not.toContain("GitHub OAuth is required but is not configured")
    expect(html).not.toContain('href="/auth/github/login"')
  })
})
