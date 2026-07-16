import { LogOut, Monitor, Moon, Settings, ShieldCheck, Sun } from "lucide-react"
import { useTheme } from "next-themes"

import type { AuthSession } from "@/admin-types"
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

export function AccountMenu({ authSession, onSignOut }: { authSession: AuthSession; onSignOut: () => void }) {
  const { setTheme, theme } = useTheme()
  const avatarURL = userAvatarURL(authSession)
  const login = authSession.login || "github"

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" className="rounded-full" aria-label="Account menu">
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
          <a href="/account/repositories">
            <Settings className="h-4 w-4" />
            Settings
          </a>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-xs font-medium text-muted-foreground">Theme</DropdownMenuLabel>
        <DropdownMenuRadioGroup value={theme || "system"} onValueChange={setTheme}>
          <DropdownMenuRadioItem value="light">
            <Sun className="h-4 w-4" />
            Light
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">
            <Moon className="h-4 w-4" />
            Dark
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system">
            <Monitor className="h-4 w-4" />
            System
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        {authSession.role === "admin" ? (
          <>
            <DropdownMenuItem asChild>
              <a href="/admin/">
                <ShieldCheck className="h-4 w-4" />
                Admin
              </a>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        ) : null}
        <DropdownMenuItem onClick={onSignOut}>
          <LogOut className="h-4 w-4" />
          Sign out
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
