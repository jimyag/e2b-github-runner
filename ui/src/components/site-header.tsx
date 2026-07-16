import type { AuthSession } from "@/admin-types"
import { AccountMenu } from "@/components/account-menu"
import { Separator } from "@/components/ui/separator"
import { SidebarTrigger } from "@/components/ui/sidebar"

export function SiteHeader({ authSession, onSignOut }: { authSession: AuthSession; onSignOut: () => void }) {
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
          <AccountMenu authSession={authSession} onSignOut={onSignOut} />
        </div>
      </div>
    </header>
  )
}
