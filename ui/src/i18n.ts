import i18n from "i18next"
import { initReactI18next } from "react-i18next"

import { en } from "./locales/en"
import { zh } from "./locales/zh"

export const supportedLanguages = ["en", "zh"] as const
export type SupportedLanguage = (typeof supportedLanguages)[number]
export const languageStorageKey = "runnerd:language"

type LanguageStorage = Pick<Storage, "getItem">

export function normalizeLanguage(language?: string | null): SupportedLanguage | null {
  const normalized = language?.trim().toLowerCase()
  if (normalized === "en" || normalized?.startsWith("en-")) return "en"
  if (normalized === "zh" || normalized?.startsWith("zh-")) return "zh"
  return null
}

export function detectLanguage(
  storage?: LanguageStorage,
  browserLanguage?: string,
): SupportedLanguage {
  let savedLanguage: string | null = null
  try {
    savedLanguage = storage?.getItem(languageStorageKey) ?? null
  } catch {
    // Browsers can deny storage access; language detection must still work.
  }
  return normalizeLanguage(savedLanguage) ?? normalizeLanguage(browserLanguage) ?? "en"
}

function browserStorage(): Storage | undefined {
  try {
    return typeof localStorage === "undefined" ? undefined : localStorage
  } catch {
    return undefined
  }
}

function syncDocumentLanguage(language: SupportedLanguage) {
  if (typeof document !== "undefined") {
    document.documentElement.lang = language
  }
}

const initialLanguage = detectLanguage(
  browserStorage(),
  typeof navigator === "undefined" ? undefined : navigator.language,
)

void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    zh: { translation: zh },
  },
  lng: initialLanguage,
  fallbackLng: "en",
  supportedLngs: supportedLanguages,
  load: "languageOnly",
  interpolation: {
    escapeValue: false,
  },
})

syncDocumentLanguage(initialLanguage)

export async function setLanguage(language: SupportedLanguage) {
  try {
    browserStorage()?.setItem(languageStorageKey, language)
  } catch {
    // The active language still changes when persistence is unavailable.
  }
  syncDocumentLanguage(language)
  await i18n.changeLanguage(language)
}

export default i18n
