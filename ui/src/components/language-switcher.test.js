import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import i18n from "../i18n"
import { LanguageSwitcher } from "./language-switcher"
import { languageMenuOptions } from "./language-switcher-options"

describe("LanguageSwitcher", () => {
  test("exposes an accessible language control with the active language", () => {
    const html = renderToStaticMarkup(createElement(LanguageSwitcher))

    expect(html).toContain('data-language-switcher="true"')
    expect(html).toContain('aria-label="Language: English"')
    expect(html).toContain("lucide-languages")
  })

  test("provides the English and Chinese choices embedded in the account menu", () => {
    expect(languageMenuOptions(i18n.getFixedT("en"))).toEqual([
      { value: "en", label: "English" },
      { value: "zh", label: "中文" },
    ])
  })
})
