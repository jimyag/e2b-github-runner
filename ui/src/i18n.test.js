import { afterEach, describe, expect, test } from "bun:test"

import i18n, {
  detectLanguage,
  languageStorageKey,
  normalizeLanguage,
  setLanguage,
} from "./i18n"

const originalDocument = globalThis.document
const originalLocalStorage = globalThis.localStorage

afterEach(async () => {
  Object.defineProperty(globalThis, "document", { configurable: true, value: originalDocument })
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: originalLocalStorage })
  await i18n.changeLanguage("en")
})

describe("i18n language selection", () => {
  test("normalizes supported regional language tags", () => {
    expect(normalizeLanguage("en-US")).toBe("en")
    expect(normalizeLanguage("zh-CN")).toBe("zh")
    expect(normalizeLanguage("zh-Hans")).toBe("zh")
    expect(normalizeLanguage("fr-FR")).toBeNull()
  })

  test("prefers a saved language and otherwise follows the browser locale", () => {
    expect(detectLanguage({ getItem: () => "zh" }, "en-US")).toBe("zh")
    expect(detectLanguage({ getItem: () => "de" }, "zh-TW")).toBe("zh")
    expect(detectLanguage(undefined, "fr-FR")).toBe("en")
  })

  test("resolves English and Chinese resources", () => {
    expect(i18n.t("common.language", { lng: "en" })).toBe("Language")
    expect(i18n.t("common.language", { lng: "zh" })).toBe("语言")
  })

  test("persists an explicit choice and synchronizes the document language", async () => {
    const values = new Map()
    const documentElement = { lang: "en" }
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      value: {
        getItem: (key) => values.get(key) ?? null,
        setItem: (key, value) => values.set(key, value),
      },
    })
    Object.defineProperty(globalThis, "document", { configurable: true, value: { documentElement } })

    await setLanguage("zh")

    expect(i18n.language).toBe("zh")
    expect(values.get(languageStorageKey)).toBe("zh")
    expect(documentElement.lang).toBe("zh")
  })
})
