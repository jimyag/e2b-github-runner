import { Languages } from "lucide-react"
import { useTranslation } from "react-i18next"

import { setLanguage, type SupportedLanguage } from "@/i18n"
import { Button } from "@/components/ui/button"
import { languageMenuOptions } from "@/components/language-switcher-options"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

export function LanguageSwitcher({
  className,
  variant = "button",
}: {
  className?: string
  variant?: "button" | "menu"
} = {}) {
  const { i18n, t } = useTranslation()
  const language = (i18n.resolvedLanguage === "zh" ? "zh" : "en") as SupportedLanguage
  const options = languageMenuOptions(t)
  const onLanguageChange = (value: string) => {
    if (value === "en" || value === "zh") void setLanguage(value)
  }

  if (variant === "menu") {
    return (
      <>
        <DropdownMenuLabel className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
          <Languages className="h-4 w-4" />
          {t("common.language")}
        </DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={language}
          onValueChange={onLanguageChange}
          data-language-menu="true"
        >
          {options.map((option) => (
            <DropdownMenuRadioItem key={option.value} value={option.value}>{option.label}</DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={className}
          aria-label={`${t("common.language")}: ${t(`common.${language === "zh" ? "chinese" : "english"}`)}`}
          data-language-switcher="true"
        >
          <Languages className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        <DropdownMenuLabel>{t("common.language")}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuRadioGroup
          value={language}
          onValueChange={onLanguageChange}
        >
          {options.map((option) => (
            <DropdownMenuRadioItem key={option.value} value={option.value}>{option.label}</DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
