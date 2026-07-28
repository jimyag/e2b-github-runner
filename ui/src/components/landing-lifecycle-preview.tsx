import { BookOpen, CheckCircle2, Clock3, Code2, Github, Layers3, ShieldCheck } from "lucide-react"

export function RunnerLifecyclePreview() {
  return (
    <div className="brand-reveal brand-reveal-delay relative mx-auto w-full max-w-[720px]">
      <div className="absolute -inset-10 -z-10 rounded-full bg-[#00aae7]/10 blur-3xl" />
      <div className="overflow-hidden rounded-xl border border-white/15 bg-[#07131b]/75 shadow-[0_40px_100px_rgba(0,0,0,0.35)] backdrop-blur-xl">
        <div className="flex h-12 items-center justify-between border-b border-white/10 px-4 sm:px-5">
          <div className="flex items-center gap-2">
            <span className="h-2.5 w-2.5 rounded-full bg-[#ff605c]" />
            <span className="h-2.5 w-2.5 rounded-full bg-[#ffbd44]" />
            <span className="h-2.5 w-2.5 rounded-full bg-[#00ca4e]" />
          </div>
          <div className="font-mono text-[10px] uppercase tracking-[0.14em] text-white/60">
            workflow / pull request #184
          </div>
          <div className="h-2 w-10 rounded-full bg-white/10" />
        </div>

        <div className="grid min-h-[440px] sm:grid-cols-[0.38fr_0.62fr]">
          <div className="border-b border-white/10 p-4 sm:border-b-0 sm:border-r sm:p-5">
            <div className="font-mono text-[10px] uppercase tracking-[0.16em] text-white/60">Requested labels</div>
            <div className="mt-3 flex flex-wrap gap-2">
              {["self-hosted", "qiniu", "ubuntu-24.04"].map((label) => (
                <span
                  key={label}
                  className="rounded border border-[#27c5f5]/20 bg-[#00aae7]/10 px-2 py-1 font-mono text-[10px] text-[#7ddcff]"
                >
                  {label}
                </span>
              ))}
            </div>

            <div className="mt-8 font-mono text-[10px] uppercase tracking-[0.16em] text-white/60">Runner policy</div>
            <div className="mt-3 space-y-3">
              {[
                ["Repository", "qiniu/ci-runner"],
                ["Runner spec", "ubuntu-24.04"],
                ["Isolation", "one job / sandbox"],
              ].map(([label, value]) => (
                <div key={label}>
                  <div className="text-[11px] text-white/60">{label}</div>
                  <div className="mt-1 truncate font-mono text-xs text-white/75">{value}</div>
                </div>
              ))}
            </div>

            <div className="mt-8 rounded-lg border border-[#27c5f5]/15 bg-[#00aae7]/[0.06] p-3">
              <div className="flex items-center gap-2 text-xs font-semibold text-[#7ddcff]">
                <ShieldCheck className="h-3.5 w-3.5" />
                Policy matched
              </div>
              <div className="mt-2 text-[11px] leading-5 text-white/60">Capacity can be provisioned.</div>
            </div>
          </div>

          <div className="relative overflow-hidden p-4 sm:p-6">
            <div className="absolute right-0 top-0 h-36 w-36 rounded-full bg-[#00aae7]/10 blur-3xl" />
            <div className="relative flex items-center justify-between">
              <div>
                <div className="font-mono text-[10px] uppercase tracking-[0.16em] text-white/60">Runner lifecycle</div>
                <div className="mt-1 text-sm font-semibold text-white">build-and-test</div>
              </div>
              <div className="flex items-center gap-2 rounded-full border border-[#27c5f5]/20 bg-[#00aae7]/10 px-2.5 py-1 font-mono text-[10px] text-[#7ddcff]">
                <span className="brand-pulse h-1.5 w-1.5 rounded-full bg-[#27c5f5]" />
                RUNNING
              </div>
            </div>

            <div className="relative mt-8">
              <div className="absolute bottom-5 left-[17px] top-5 w-px bg-gradient-to-b from-[#00aae7] via-[#00aae7]/40 to-white/10" />
              {[
                { icon: Github, title: "GitHub job accepted", detail: "workflow_job · queued", state: "complete" },
                { icon: Layers3, title: "Sandbox created", detail: "us-south-1 · 8.4s", state: "complete" },
                { icon: Code2, title: "Runner registered", detail: "ephemeral · online", state: "complete" },
                { icon: Clock3, title: "Job executing", detail: "tests · 01:42", state: "active" },
              ].map(({ icon: Icon, title, detail, state }) => (
                <div key={title} className="relative grid grid-cols-[36px_1fr_auto] items-center gap-3 py-3">
                  <div
                    className={[
                      "relative z-10 flex h-9 w-9 items-center justify-center rounded-full border",
                      state === "active"
                        ? "border-[#27c5f5]/60 bg-[#073149] text-[#7ddcff]"
                        : "border-[#00aae7]/25 bg-[#082432] text-[#27c5f5]",
                    ].join(" ")}
                  >
                    <Icon className="h-3.5 w-3.5" />
                  </div>
                  <div>
                    <div className="text-xs font-medium text-white/85">{title}</div>
                    <div className="mt-1 font-mono text-[10px] text-white/60">{detail}</div>
                  </div>
                  {state === "complete" ? (
                    <CheckCircle2 className="h-4 w-4 text-[#36d399]" />
                  ) : (
                    <span className="brand-pulse h-2 w-2 rounded-full bg-[#27c5f5]" />
                  )}
                </div>
              ))}
            </div>

            <div className="mt-6 rounded-lg border border-white/10 bg-black/20 p-3 font-mono text-[10px] leading-5 text-white/65">
              <div className="flex items-center justify-between text-white/70">
                <span>$ task test</span>
                <span className="text-[#36d399]">in progress</span>
              </div>
              <div className="mt-2">✓ UI tests passed</div>
              <div>→ Go tests running with race detection</div>
            </div>
          </div>
        </div>
      </div>

      <div className="absolute -bottom-5 -right-3 hidden items-center gap-3 rounded-lg border border-[#bdeaf8] bg-white px-4 py-3 shadow-[0_18px_50px_rgba(0,0,0,0.22)] sm:flex">
        <div className="flex h-9 w-9 items-center justify-center rounded-full bg-[#e8fff6] text-[#158f61]">
          <CheckCircle2 className="h-4 w-4" />
        </div>
        <div>
          <div className="text-xs font-semibold text-[#0a0d12]">Clean execution</div>
          <div className="mt-0.5 font-mono text-[9px] uppercase tracking-[0.12em] text-[#596b75]">
            no shared workspace
          </div>
        </div>
      </div>

      <div className="absolute -left-5 top-24 hidden items-center gap-3 rounded-lg border border-white/15 bg-[#0a2634]/95 px-4 py-3 shadow-2xl backdrop-blur sm:flex">
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-[#00aae7]/15 text-[#7ddcff]">
          <BookOpen className="h-3.5 w-3.5" />
        </div>
        <div>
          <div className="font-mono text-[9px] uppercase tracking-[0.13em] text-white/60">runner spec</div>
          <div className="mt-0.5 text-xs font-semibold text-white">ubuntu-24.04</div>
        </div>
      </div>
    </div>
  )
}
