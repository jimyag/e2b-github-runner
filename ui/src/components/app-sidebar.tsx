import { Activity, ClipboardList, Github, ListTree, Route, ScrollText, Server, Settings2, Stethoscope, Terminal } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"

type AppSidebarProps = {
  section: string
  connected: boolean
  activeCount: number
  authLabel: string
  onSectionChange: (section: string) => void
  onSignOut: () => void
}

export function AppSidebar({
  section,
  connected,
  activeCount,
  authLabel,
  onSectionChange,
  onSignOut,
}: AppSidebarProps) {
  const items = [
    { id: "overview", label: "Overview", icon: Activity },
    { id: "runner_requests", label: "Runner Requests", icon: ListTree },
    { id: "runner_specs", label: "Runner Specs", icon: Settings2 },
    { id: "runner_groups", label: "Runner Groups", icon: Server },
    { id: "runner_policies", label: "Runner Policies", icon: Route },
    { id: "match", label: "Match Test", icon: ClipboardList },
    { id: "audit", label: "Audit", icon: ScrollText },
    { id: "diagnostics", label: "Diagnostics", icon: Stethoscope },
  ]

  return (
    <Sidebar collapsible="offcanvas">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild className="data-[slot=sidebar-menu-button]:!p-1.5">
              <a href="/" aria-label="Qiniu Runner home">
                <div className="rounded-lg bg-sidebar-primary p-1.5 shadow-sm">
                  <Terminal className="h-4 w-4 text-sidebar-primary-foreground" />
                </div>
                <div className="flex flex-col">
                  <span className="bg-gradient-to-r from-primary to-primary/70 bg-clip-text text-base font-semibold text-transparent">
                    Qiniu
                  </span>
                  <span className="text-[10px] font-medium leading-none text-muted-foreground">
                    Runner
                  </span>
                </div>
              </a>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {items.map((item) => {
                const Icon = item.icon
                return (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton
                      isActive={section === item.id}
                      className="data-[active=true]:bg-primary/10 data-[active=true]:text-primary data-[active=true]:shadow-sm"
                      onClick={() => onSectionChange(item.id)}
                    >
                      <Icon className="text-sidebar-primary" />
                      <span className="font-medium">{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <div className="rounded-lg border bg-card p-2 shadow-sm transition-colors hover:bg-muted/50">
          <div className="mb-2 flex items-center gap-2 px-1">
            <span className={connected ? "h-2 w-2 rounded-full bg-green-500" : "h-2 w-2 rounded-full bg-muted-foreground"} />
            <span className="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground">
              {connected ? "Connected" : authLabel}
            </span>
          </div>
          <div className="grid gap-2 rounded-md bg-background p-2 text-xs">
            <div className="flex items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1 text-muted-foreground">
                <Activity className="h-3.5 w-3.5" />
                Active
              </span>
              <span className="font-semibold">{activeCount}</span>
            </div>
            <div className="flex items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1 text-muted-foreground">
                <Server className="h-3.5 w-3.5" />
                Scope
              </span>
              <span className="font-semibold">repo/org</span>
            </div>
            <div className="flex items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1 text-muted-foreground">
                <Github className="h-3.5 w-3.5" />
                Admin
              </span>
              <span className="min-w-0 truncate font-semibold">{authLabel}</span>
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-2 w-full"
            onClick={onSignOut}
          >
            Sign out
          </Button>
        </div>
      </SidebarFooter>
    </Sidebar>
  )
}
