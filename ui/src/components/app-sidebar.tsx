import { Activity, ClipboardList, CloudCog, ListTree, Route, ScrollText, Server, Settings2, Stethoscope, Terminal, UsersRound } from "lucide-react"

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"

type AppSidebarProps = {
  section: string
  onSectionChange: (section: string) => void
}

export function AppSidebar({
  section,
  onSectionChange,
}: AppSidebarProps) {
  const items = [
    { id: "overview", label: "Overview", icon: Activity },
    { id: "accounts", label: "Accounts", icon: UsersRound },
    { id: "runner_requests", label: "Runner Requests", icon: ListTree },
    { id: "runner_specs", label: "Runner Specs", icon: Settings2 },
    { id: "runner_groups", label: "Runner Groups", icon: Server },
    { id: "runner_policies", label: "Runner Policies", icon: Route },
    { id: "sandbox_service", label: "Sandbox Service", icon: CloudCog },
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
    </Sidebar>
  )
}
