import { Github, Play, ShieldCheck } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { cn } from "@/lib/utils"

export function LoginPage({
  oauthEnabled,
  currentLogin,
  currentRole,
  onSignOut,
}: {
  oauthEnabled: boolean
  currentLogin?: string
  currentRole?: string
  onSignOut: () => void
}) {
  return (
    <main className="min-h-screen bg-background text-foreground">
      <div className="min-h-screen">
        <section className="relative flex min-h-screen overflow-hidden bg-[radial-gradient(circle_at_30%_20%,oklch(0.92_0.08_195),transparent_34%),linear-gradient(135deg,oklch(0.98_0.02_180),oklch(0.94_0.03_250))] p-6 sm:p-10">
          <div className="absolute inset-x-10 top-1/2 h-px bg-foreground/10" />
          <div className="absolute bottom-28 left-10 right-20 grid grid-cols-6 gap-2 opacity-70">
            {Array.from({ length: 36 }).map((_, index) => (
              <span
                key={index}
                className={cn(
                  "h-2 rounded-full",
                  index % 7 === 0 ? "bg-primary" : index % 5 === 0 ? "bg-emerald-500" : "bg-foreground/15"
                )}
              />
            ))}
          </div>
          <div className="relative z-10 flex h-fit items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-foreground text-background shadow-sm">
              <Play className="h-5 w-5" />
            </div>
            <div>
              <div className="text-sm font-semibold tracking-wide">Qiniu Runner</div>
              <div className="text-xs text-muted-foreground">Ephemeral Actions control plane</div>
            </div>
          </div>

          <div className="absolute inset-0 z-20 flex items-center justify-center px-5">
            <Card className="rounded-lg shadow-sm">
              <CardHeader className="gap-2">
                <div className="flex h-11 w-11 items-center justify-center rounded-lg border bg-muted">
                  <ShieldCheck className="h-5 w-5 text-primary" />
                </div>
                <CardTitle className="text-2xl">Sign in</CardTitle>
                <CardDescription>Use your allowed GitHub account to access runnerd.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-5">
                {currentLogin ? (
                  <div className="space-y-3">
                    <div className="rounded-lg border bg-muted/50 p-3 text-sm text-muted-foreground">
                      {currentLogin} is signed in as {currentRole || "user"} and does not have admin access.
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="lg"
                      className="w-full justify-center"
                      onClick={onSignOut}
                    >
                      Sign out
                    </Button>
                  </div>
                ) : oauthEnabled ? (
                  <Button
                    type="button"
                    size="lg"
                    className="w-full justify-center"
                    onClick={() => {
                      window.location.href = "/auth/github/login"
                    }}
                  >
                    <Github className="h-4 w-4" />
                    Continue with GitHub
                  </Button>
                ) : (
                  <div className="rounded-lg border bg-muted/50 p-3 text-sm text-muted-foreground">
                    GitHub OAuth is required but not configured on this runnerd instance.
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          <div className="absolute bottom-10 left-6 right-6 z-10 max-w-2xl sm:left-10 sm:right-auto">
            <h1 className="max-w-lg text-3xl font-semibold leading-tight text-foreground sm:text-4xl">
              Sign in before touching live runner capacity.
            </h1>
            <div className="mt-5 grid max-w-xl gap-3 sm:grid-cols-3">
              {[
                ["Isolated", "E2B sandboxes per job"],
                ["Scoped", "GitHub allowlists and policies"],
                ["Audited", "Admin actions are traceable"],
              ].map(([label, detail]) => (
                <div key={label} className="rounded-lg border bg-background/70 p-3 shadow-sm backdrop-blur">
                  <div className="text-sm font-semibold">{label}</div>
                  <div className="mt-1 text-xs leading-relaxed text-muted-foreground">{detail}</div>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </main>
  )
}
