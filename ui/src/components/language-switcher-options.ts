import type { AppTFunction, SupportedLanguage } from "@/i18n"

export function languageMenuOptions(t: AppTFunction): Array<{ value: SupportedLanguage; label: string }> {
  return [
    { value: "en", label: t("common.english") },
    { value: "zh", label: t("common.chinese") },
  ]
}
