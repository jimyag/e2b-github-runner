import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { AccessDeniedPage, NotFoundPage, SessionLoadingPage, SignInPage } from "./auth-pages"

describe("authentication route pages", () => {
  test("preserves the protected destination through GitHub sign-in", () => {
    const html = renderToStaticMarkup(
      createElement(SignInPage, {
        oauthEnabled: true,
        returnTo: "/github/pulls/octo/repo/12/jobs?job=runner-1",
      }),
    )

    expect(html).toContain("Sign in to continue")
    expect(html).toContain("Pull request jobs")
    expect(html).not.toContain("lucide-lock-keyhole")
    expect(html).toContain(
      'href="/auth/github/login?return_to=%2Fgithub%2Fpulls%2Focto%2Frepo%2F12%2Fjobs%3Fjob%3Drunner-1"',
    )
    expect(html).toContain('href="/"')
  })

  test("explains when sign-in is unavailable", () => {
    const html = renderToStaticMarkup(
      createElement(SignInPage, {
        oauthEnabled: false,
        returnTo: "/jobs",
      }),
    )

    expect(html).toContain("Sign-in unavailable")
    expect(html).toContain("GitHub OAuth is not configured")
    expect(html).not.toContain("/auth/github/login?")
  })

  test("gives authenticated users a way out of an unauthorized admin route", () => {
    const html = renderToStaticMarkup(
      createElement(AccessDeniedPage, {
        login: "octocat",
        onSignOut: () => {},
      }),
    )

    expect(html).toContain("Access denied")
    expect(html).toContain("octocat")
    expect(html).toContain("Open Jobs")
    expect(html).toContain("Sign out")
  })

  test("renders distinct loading and not-found states", () => {
    expect(renderToStaticMarkup(createElement(SessionLoadingPage))).toContain("Checking your session")
    expect(renderToStaticMarkup(createElement(NotFoundPage))).toContain("Page not found")
  })
})
