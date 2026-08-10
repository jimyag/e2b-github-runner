import type { TFunction } from "i18next"

import type { SupportedLanguage } from "@/i18n"

export function languageMenuOptions(t: TFunction): Array<{ value: SupportedLanguage; label: string }> {
  return [
    { value: "en", label: t("common.english") },
    { value: "zh", label: t("common.chinese") },
  ]
}
