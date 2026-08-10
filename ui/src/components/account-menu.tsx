import { CircleHelp, LogOut, Monitor, Moon, Settings, ShieldCheck, Sun } from "lucide-react"
import { useTheme } from "next-themes"
import { useTranslation } from "react-i18next"

import type { AuthSession } from "@/admin-types"
import { LanguageSwitcher } from "@/components/language-switcher"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

export function AccountMenu({
  authSession,
  onReplayProductTour,
  onSignOut,
}: {
  authSession: AuthSession
  onReplayProductTour?: () => void
  onSignOut: () => void
}) {
  const { setTheme, theme } = useTheme()
  const { t } = useTranslation()
  const avatarURL = userAvatarURL(authSession)
  const login = authSession.login || "github"

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="rounded-full"
          aria-label={t("common.accountMenu")}
          data-onboarding="account-menu"
        >
          {avatarURL ? (
            <img
              src={avatarURL}
              alt=""
              className="h-8 w-8 rounded-full border bg-muted object-cover"
              referrerPolicy="no-referrer"
            />
          ) : (
            <span className="flex h-8 w-8 items-center justify-center rounded-full border bg-muted text-xs font-semibold">
              {userInitials(login)}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="truncate">{login}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <a href="/account/preferences">
            <Settings className="h-4 w-4" />
            {t("common.settings")}
          </a>
        </DropdownMenuItem>
        {onReplayProductTour ? (
          <DropdownMenuItem onClick={onReplayProductTour}>
            <CircleHelp className="h-4 w-4" />
            {t("common.replayProductTour")}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuSeparator />
        <LanguageSwitcher variant="menu" />
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-xs font-medium text-muted-foreground">{t("common.theme")}</DropdownMenuLabel>
        <DropdownMenuRadioGroup value={theme || "system"} onValueChange={setTheme}>
          <DropdownMenuRadioItem value="light">
            <Sun className="h-4 w-4" />
            {t("common.light")}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">
            <Moon className="h-4 w-4" />
            {t("common.dark")}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system">
            <Monitor className="h-4 w-4" />
            {t("common.system")}
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        {authSession.role === "admin" ? (
          <>
            <DropdownMenuItem asChild>
              <a href="/admin/">
                <ShieldCheck className="h-4 w-4" />
                {t("common.admin")}
              </a>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        ) : null}
        <DropdownMenuItem onClick={onSignOut}>
          <LogOut className="h-4 w-4" />
          {t("common.signOut")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function userAvatarURL(authSession: AuthSession) {
  if (authSession.avatar_url) return authSession.avatar_url
  if (!authSession.login) return ""
  return `https://github.com/${encodeURIComponent(authSession.login)}.png?size=96`
}

function userInitials(login: string) {
  return (
    login
      .split(/[-_\s]+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase())
      .join("") || "GH"
  )
}
