import { BookOpen } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { AuthSession } from "@/admin-types"
import { AccountMenu } from "@/components/account-menu"
import { Separator } from "@/components/ui/separator"
import { SidebarTrigger } from "@/components/ui/sidebar"

export function SiteHeader({ authSession, onSignOut }: { authSession: AuthSession; onSignOut: () => void }) {
  const { t } = useTranslation()

  return (
    <header className="sticky top-0 z-50 flex h-(--header-height) shrink-0 items-center gap-2 border-b bg-background/95 backdrop-blur transition-[width,height] ease-linear supports-[backdrop-filter]:bg-background/60">
      <div className="flex w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
        <SidebarTrigger className="-ml-1" />
        <Separator
          orientation="vertical"
          className="mx-2 data-[orientation=vertical]:h-4"
        />
        <span className="text-sm font-semibold text-foreground">Qiniu Runner</span>
        <div className="ml-auto flex items-center gap-2">
          <a
            href="/docs"
            aria-label={t("common.documentation", { defaultValue: "Documentation" })}
            className="inline-flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <BookOpen className="h-4 w-4" />
          </a>
          <AccountMenu authSession={authSession} onSignOut={onSignOut} />
        </div>
      </div>
    </header>
  )
}
