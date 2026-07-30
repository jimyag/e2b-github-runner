import { expect, test } from "@playwright/test"

const postRenderObservationMs = 1_000

test("boots the public landing page from the production bundle", async ({ page }) => {
  const consoleErrors: string[] = []
  const pageErrors: string[] = []
  const failedAssets: string[] = []

  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text())
    }
  })
  page.on("pageerror", (error) => {
    pageErrors.push(error.message)
  })
  page.on("requestfailed", (request) => {
    if (["script", "stylesheet"].includes(request.resourceType())) {
      failedAssets.push(`${request.method()} ${request.url()}: ${request.failure()?.errorText ?? "failed"}`)
    }
  })
  page.on("response", (response) => {
    if (
      response.status() >= 400 &&
      ["script", "stylesheet"].includes(response.request().resourceType())
    ) {
      failedAssets.push(`${response.status()} ${response.url()}`)
    }
  })

  const response = await page.goto("/", { waitUntil: "networkidle" })

  expect(response?.ok()).toBe(true)
  await expect(
    page.getByRole("heading", { name: "GitHub Actions, powered by Qiniu Sandbox" }),
  ).toBeVisible()
  await expect(page.locator("#root")).not.toBeEmpty()
  await page.waitForTimeout(postRenderObservationMs)
  expect(pageErrors, "page errors").toEqual([])
  expect(consoleErrors, "console errors").toEqual([])
  expect(failedAssets, "failed script or stylesheet requests").toEqual([])
})
