import {
  ArrowRight,
  BookOpen,
  Check,
  ChevronRight,
  CircleDot,
  Github,
  GitPullRequest,
  Layers3,
  ShieldCheck,
  Sparkles,
  Terminal,
  Trash2,
  Workflow,
  Zap,
} from "lucide-react"

import { RunnerLifecyclePreview } from "@/components/landing-lifecycle-preview"
import { QiniuRunnerLogo } from "@/components/qiniu-runner-logo"

const documentationURL = "https://github.com/qiniu/ci-runner#documentation"

const capabilities = [
  {
    icon: Layers3,
    eyebrow: "ISOLATED BY DEFAULT",
    title: "One sandbox per job",
    description:
      "Every workflow job starts in a clean Qiniu Sandbox, keeping builds independent and repeatable.",
    detail: "Disposable compute",
  },
  {
    icon: Zap,
    eyebrow: "JUST IN TIME",
    title: "Capacity when Actions calls",
    description:
      "runnerd matches labels and policies, creates the runner, and connects it to GitHub only when needed.",
    detail: "Queue-aware provisioning",
  },
  {
    icon: ShieldCheck,
    eyebrow: "CLEAN LIFECYCLE",
    title: "Finished means removed",
    description:
      "Runner registration and sandbox capacity are cleaned up after completion, timeout, or failure.",
    detail: "Automatic cleanup",
  },
]

const workflowSteps = [
  {
    number: "01",
    icon: GitPullRequest,
    title: "Workflow queued",
    description: "GitHub sends a workflow_job webhook with the repository and requested labels.",
  },
  {
    number: "02",
    icon: Workflow,
    title: "Policy matched",
    description: "runnerd selects an allowed runner spec and queues the request until worker capacity is available.",
  },
  {
    number: "03",
    icon: Terminal,
    title: "Sandbox running",
    description: "A clean Qiniu Sandbox starts an ephemeral self-hosted GitHub Actions runner.",
  },
  {
    number: "04",
    icon: Trash2,
    title: "Resources cleared",
    description: "After the job exits, the registration and sandbox are removed automatically.",
  },
]

export function LandingPage() {
  return (
    <div className="brand-home min-h-screen overflow-x-hidden bg-white text-[#0a0d12] [color-scheme:light]">
      <a
        href="#main-content"
        className="fixed left-4 top-4 z-[60] -translate-y-24 rounded-md bg-[#0a0d12] px-4 py-2 text-sm font-semibold text-white transition-transform focus:translate-y-0"
      >
        Skip to content
      </a>

      <header className="absolute inset-x-0 top-0 z-50 border-b border-white/10">
        <div className="mx-auto flex h-20 max-w-[1440px] items-center justify-between px-5 sm:px-8 lg:px-12">
          <a href="/" className="group" aria-label="Qiniu Runner home">
            <QiniuRunnerLogo inverse />
          </a>

          <nav className="hidden items-center gap-8 text-sm text-white/70 md:flex" aria-label="Primary navigation">
            <a className="transition-colors hover:text-white" href="#capabilities">
              Product
            </a>
            <a className="transition-colors hover:text-white" href="#how-it-works">
              How it works
            </a>
            <a
              className="inline-flex items-center gap-1.5 transition-colors hover:text-white"
              href={documentationURL}
              target="_blank"
              rel="noreferrer"
            >
              Documentation
              <ArrowRight className="h-3.5 w-3.5 -rotate-45" />
            </a>
          </nav>

          <div className="flex items-center gap-2">
            <a
              href={documentationURL}
              target="_blank"
              rel="noreferrer"
              aria-label="Documentation"
              className="inline-flex h-10 w-10 items-center justify-center rounded-md border border-white/20 text-white transition-colors hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#27c5f5] md:hidden"
            >
              <BookOpen className="h-4 w-4" />
            </a>
            <LandingAccessAction compact />
          </div>
        </div>
      </header>

      <main id="main-content" tabIndex={-1}>
        <section className="relative isolate min-h-[760px] overflow-hidden bg-[#07131b] pb-20 pt-28 sm:pt-32 lg:min-h-[760px] lg:pb-24 lg:pt-36">
        <div className="absolute inset-0 -z-30 bg-[linear-gradient(112deg,#07131b_0%,#092433_54%,#06324a_100%)]" />
        <div className="absolute -right-40 -top-48 -z-20 h-[680px] w-[680px] rounded-full bg-[#00aae7]/20 blur-[110px]" />
        <div className="absolute -bottom-72 -left-32 -z-20 h-[620px] w-[620px] rounded-full bg-[#0088c2]/20 blur-[130px]" />
        <div className="brand-grid absolute inset-0 -z-10 opacity-50" />

        <div className="mx-auto grid max-w-[1440px] items-center gap-16 px-5 sm:px-8 lg:grid-cols-[0.92fr_1.08fr] lg:gap-20 lg:px-12">
          <div className="brand-reveal max-w-2xl">
            <div className="mb-7 inline-flex items-center gap-2 rounded-full border border-[#27c5f5]/30 bg-[#00aae7]/10 px-3.5 py-2 font-mono text-[11px] uppercase tracking-[0.16em] text-[#7ddcff]">
              <Sparkles className="h-3.5 w-3.5" />
              Ephemeral Actions infrastructure
            </div>

            <h1
              aria-label="GitHub Actions, powered by Qiniu Sandbox"
              className="max-w-[760px] text-[clamp(3rem,5.4vw,5.8rem)] font-semibold leading-[0.94] tracking-[-0.065em] text-white"
            >
              GitHub Actions,
              <span className="mt-2 block text-[#27c5f5]">powered by</span>
              <span className="mt-2 block">Qiniu Sandbox.</span>
            </h1>

            <p className="mt-8 max-w-xl text-base leading-7 text-[#c5d7e1] sm:text-lg sm:leading-8">
              Launch a clean, isolated runner for every job. Keep the GitHub workflow your team already
              knows—move the execution into disposable cloud capacity.
            </p>

            <div className="mt-10 flex flex-col gap-3 sm:flex-row sm:items-center">
              <LandingAccessAction />
              <a
                href="#how-it-works"
                className="group inline-flex h-12 items-center justify-center gap-2 rounded-md border border-white/20 px-5 text-sm font-semibold text-white transition-all hover:border-white/40 hover:bg-white/5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#27c5f5]"
              >
                See how it works
                <ChevronRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
              </a>
            </div>

            <div className="mt-9 flex flex-wrap gap-x-6 gap-y-3 text-xs text-white/55 sm:text-sm">
              {["GitHub-native workflows", "Policy-scoped access", "Automatic cleanup"].map((item) => (
                <span key={item} className="inline-flex items-center gap-2">
                  <Check className="h-3.5 w-3.5 text-[#27c5f5]" />
                  {item}
                </span>
              ))}
            </div>
          </div>

          <RunnerLifecyclePreview />
        </div>

        <div className="pointer-events-none absolute bottom-0 left-1/2 h-20 w-px bg-gradient-to-b from-[#27c5f5]/0 to-[#27c5f5]/70" />
      </section>

      <section className="border-b border-[#e5edf4] bg-[#f8fcfe]">
        <div className="mx-auto grid max-w-[1440px] divide-y divide-[#e5edf4] px-5 sm:px-8 md:grid-cols-3 md:divide-x md:divide-y-0 lg:px-12">
          {[
            ["01", "GitHub sends the job"],
            ["02", "runnerd creates capacity"],
            ["03", "Qiniu Sandbox executes"],
          ].map(([number, label]) => (
            <div key={number} className="flex items-center gap-4 py-5 md:px-6 first:md:pl-0">
              <span className="font-mono text-xs text-[#006b91]">{number}</span>
              <span className="text-sm font-medium text-[#384b57]">{label}</span>
            </div>
          ))}
        </div>
      </section>

      <section id="capabilities" className="scroll-mt-8 bg-white py-24 sm:py-32">
        <div className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12">
          <SectionIntro
            label="Built for clean execution"
            title="Capacity appears for the job. Then disappears."
            description="Qiniu CI Runner turns GitHub workflow demand into short-lived, isolated compute—with the controls teams need around it."
          />

          <div className="mt-14 grid gap-px overflow-hidden rounded-xl border border-[#e5edf4] bg-[#e5edf4] lg:grid-cols-3">
            {capabilities.map(({ icon: Icon, eyebrow, title, description, detail }, index) => (
              <article
                key={title}
                className="group relative min-h-[360px] overflow-hidden bg-white p-7 transition-colors hover:bg-[#f7fcff] sm:p-9"
              >
                <div className="absolute right-0 top-0 font-mono text-[92px] font-semibold leading-none tracking-[-0.08em] text-[#f0f7fa] transition-colors group-hover:text-[#e7f7fc]">
                  0{index + 1}
                </div>
                <div className="relative flex h-full flex-col">
                  <div className="flex h-12 w-12 items-center justify-center rounded-lg border border-[#bdeaf8] bg-[#f2fcff] text-[#006b91]">
                    <Icon className="h-5 w-5" />
                  </div>
                  <p className="mt-14 font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-[#006b91]">
                    {eyebrow}
                  </p>
                  <h3 className="mt-4 text-2xl font-semibold tracking-[-0.035em] text-[#0a0d12]">{title}</h3>
                  <p className="mt-4 max-w-sm text-[15px] leading-7 text-[#596b75]">{description}</p>
                  <div className="mt-auto flex items-center gap-2 border-t border-[#e5edf4] pt-6 text-xs font-medium text-[#384b57]">
                    <CircleDot className="h-3.5 w-3.5 text-[#006b91]" />
                    {detail}
                  </div>
                </div>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section id="how-it-works" className="scroll-mt-8 overflow-hidden bg-[#081720] py-24 text-white sm:py-32">
        <div className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12">
          <div className="grid gap-12 lg:grid-cols-[0.7fr_1.3fr] lg:gap-20">
            <div>
              <p className="font-mono text-[11px] font-semibold uppercase tracking-[0.18em] text-[#27c5f5]">
                How it works
              </p>
              <h2 className="mt-5 max-w-lg text-4xl font-semibold leading-[1.04] tracking-[-0.05em] sm:text-5xl">
                From webhook to clean slate.
              </h2>
              <p className="mt-6 max-w-md text-base leading-7 text-[#a9bfcb]">
                runnerd connects the GitHub Actions lifecycle to Qiniu Sandbox without changing the workflow
                developers use every day.
              </p>
              <a
                href={documentationURL}
                target="_blank"
                rel="noreferrer"
                className="group mt-9 inline-flex items-center gap-2 text-sm font-semibold text-[#7ddcff] transition-colors hover:text-white"
              >
                Read the documentation
                <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-1" />
              </a>
            </div>

            <div className="relative">
              <div className="absolute bottom-8 left-[41px] top-8 w-px -translate-x-1/2 bg-gradient-to-b from-[#27c5f5] via-[#00aae7]/35 to-transparent sm:left-[57px]" />
              <div className="space-y-3">
                {workflowSteps.map(({ number, icon: Icon, title, description }) => (
                  <article
                    key={number}
                    className="group relative grid grid-cols-[48px_1fr] gap-4 rounded-xl border border-white/10 bg-white/[0.035] p-4 transition-all hover:border-[#27c5f5]/35 hover:bg-[#00aae7]/[0.06] sm:grid-cols-[64px_1fr] sm:gap-6 sm:p-6"
                  >
                    <div className="relative z-10 flex h-12 w-12 items-center justify-center rounded-full border border-[#27c5f5]/35 bg-[#0a2634] text-[#7ddcff] sm:h-16 sm:w-16">
                      <Icon className="h-5 w-5 sm:h-6 sm:w-6" />
                    </div>
                    <div className="py-1">
                      <div className="flex items-center justify-between gap-4">
                        <h3 className="text-lg font-semibold tracking-[-0.02em] sm:text-xl">{title}</h3>
                        <span className="font-mono text-xs text-white/60">{number}</span>
                      </div>
                      <p className="mt-2 max-w-xl text-sm leading-6 text-[#9bb2be] sm:text-[15px]">{description}</p>
                    </div>
                  </article>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="relative overflow-hidden bg-[#00aae7] py-20 sm:py-24">
        <div className="brand-grid brand-grid-dark absolute inset-0 opacity-30" />
        <div className="relative mx-auto flex max-w-[1440px] flex-col items-start justify-between gap-10 px-5 sm:px-8 lg:flex-row lg:items-end lg:px-12">
          <div>
            <p className="font-mono text-[11px] font-semibold uppercase tracking-[0.18em] text-[#073149]">
              Ready for the next job
            </p>
            <h2 className="mt-4 max-w-3xl text-4xl font-semibold leading-[1.02] tracking-[-0.055em] text-[#07131b] sm:text-5xl lg:text-6xl">
              Give every workflow a clean place to run.
            </h2>
          </div>
          <LandingAccessAction inverted />
        </div>
        </section>
      </main>

      <footer className="border-t border-[#e5edf4] bg-white">
        <div className="mx-auto flex max-w-[1440px] flex-col gap-6 px-5 py-8 text-sm text-[#596b75] sm:px-8 md:flex-row md:items-center md:justify-between lg:px-12">
          <div className="flex items-center gap-4 text-[#0a0d12]">
            <QiniuRunnerLogo />
            <span className="hidden h-8 w-px bg-[#d5e4eb] sm:block" aria-hidden="true" />
            <div className="hidden text-xs text-[#596b75] sm:block">Powered by Qiniu Sandbox</div>
          </div>
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            <a className="transition-colors hover:text-[#006b91]" href="#capabilities">
              Product
            </a>
            <a className="transition-colors hover:text-[#006b91]" href="#how-it-works">
              How it works
            </a>
            <a
              className="inline-flex items-center gap-1.5 transition-colors hover:text-[#006b91]"
              href={documentationURL}
              target="_blank"
              rel="noreferrer"
            >
              Documentation
              <ArrowRight className="h-3.5 w-3.5 -rotate-45" />
            </a>
            <a
              className="inline-flex items-center gap-1.5 transition-colors hover:text-[#006b91]"
              href="https://github.com/qiniu/ci-runner"
              target="_blank"
              rel="noreferrer"
            >
              GitHub
              <Github className="h-3.5 w-3.5" />
            </a>
          </div>
        </div>
      </footer>
    </div>
  )
}

function LandingAccessAction({
  inverted = false,
  compact = false,
}: {
  inverted?: boolean
  compact?: boolean
}) {
  const colorClass = inverted
    ? "bg-[#07131b] text-white hover:bg-[#0d2533] focus-visible:outline-[#07131b]"
    : compact
      ? "bg-white text-[#0a0d12] hover:bg-[#eaf9fe] focus-visible:outline-[#27c5f5]"
      : "bg-[#00aae7] text-[#041018] hover:bg-[#27c5f5] focus-visible:outline-[#27c5f5]"
  const sizeClass = compact ? "h-10 px-4" : "h-12 px-5"

  return (
    <a
      href="/jobs"
      aria-label={compact ? "Open Jobs" : undefined}
      className={`group inline-flex items-center justify-center gap-2.5 rounded-md text-sm font-semibold transition-all focus-visible:outline-2 focus-visible:outline-offset-2 ${sizeClass} ${colorClass}`}
    >
      <span className={compact ? "hidden sm:inline" : undefined}>Open Jobs</span>
      {compact ? <Terminal className="h-4 w-4 sm:hidden" /> : null}
      <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
    </a>
  )
}

function SectionIntro({
  label,
  title,
  description,
}: {
  label: string
  title: string
  description: string
}) {
  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_0.7fr] lg:items-end">
      <div>
        <p className="font-mono text-[11px] font-semibold uppercase tracking-[0.18em] text-[#006b91]">{label}</p>
        <h2 className="mt-5 max-w-4xl text-4xl font-semibold leading-[1.04] tracking-[-0.05em] text-[#0a0d12] sm:text-5xl lg:text-6xl">
          {title}
        </h2>
      </div>
      <p className="max-w-lg text-base leading-7 text-[#596b75] lg:justify-self-end">{description}</p>
    </div>
  )
}
