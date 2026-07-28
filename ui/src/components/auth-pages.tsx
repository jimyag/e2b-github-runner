import { ArrowLeft, Github, LoaderCircle, SearchX, ShieldX } from "lucide-react"
import type { ReactNode } from "react"

import { signInURL } from "@/app-load-policy"
import { QiniuRunnerLogo } from "@/components/qiniu-runner-logo"

export function SignInPage({
  oauthEnabled,
  returnTo,
}: {
  oauthEnabled: boolean
  returnTo: string
}) {
  const destination = protectedDestinationLabel(returnTo)

  return (
    <RouteMessagePage
      eyebrow={destination}
      title="Sign in to continue"
      description={`Use your GitHub account to open ${destination.toLowerCase()}. You will return here after sign-in.`}
    >
      {oauthEnabled ? (
        <a
          href={signInURLFromReturnTo(returnTo)}
          className="inline-flex h-11 items-center justify-center gap-2 rounded-md bg-[#00aae7] px-5 text-sm font-semibold text-[#041018] transition-colors hover:bg-[#27c5f5] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#00aae7]"
        >
          <Github className="h-4 w-4" />
          Continue with GitHub
        </a>
      ) : (
        <div className="rounded-lg border border-[#f0d5a8] bg-[#fff9ed] px-4 py-3 text-sm leading-6 text-[#6f4b11]" role="status">
          <strong className="block">Sign-in unavailable</strong>
          GitHub OAuth is not configured on this runnerd instance.
        </div>
      )}
    </RouteMessagePage>
  )
}

export function AccessDeniedPage({
  login,
  onSignOut,
}: {
  login?: string
  onSignOut: () => void
}) {
  return (
    <RouteMessagePage
      eyebrow="Admin console"
      icon={<ShieldX className="h-5 w-5" />}
      title="Access denied"
      description={`${login || "This GitHub account"} is signed in but does not have administrator access.`}
    >
      <div className="flex flex-col gap-3 sm:flex-row">
        <a
          href="/jobs"
          className="inline-flex h-11 items-center justify-center rounded-md bg-[#00aae7] px-5 text-sm font-semibold text-[#041018] transition-colors hover:bg-[#27c5f5]"
        >
          Open Jobs
        </a>
        <button
          type="button"
          onClick={onSignOut}
          className="inline-flex h-11 items-center justify-center rounded-md border border-[#cbdde6] bg-white px-5 text-sm font-semibold text-[#213a48] transition-colors hover:bg-[#f2fcff]"
        >
          Sign out
        </button>
      </div>
    </RouteMessagePage>
  )
}

export function NotFoundPage() {
  return (
    <RouteMessagePage
      eyebrow="404"
      icon={<SearchX className="h-5 w-5" />}
      title="Page not found"
      description="The address does not match a Qiniu CI Runner page."
    />
  )
}

export function SessionLoadingPage() {
  return (
    <RouteMessagePage
      eyebrow="Secure route"
      icon={<LoaderCircle className="h-5 w-5 animate-spin" />}
      title="Checking your session"
      description="Confirming your GitHub access before loading this page."
    />
  )
}

function RouteMessagePage({
  eyebrow,
  icon,
  title,
  description,
  children,
}: {
  eyebrow: string
  icon?: ReactNode
  title: string
  description: string
  children?: ReactNode
}) {
  return (
    <main className="brand-home relative flex min-h-screen items-center overflow-hidden bg-[#f7fcff] px-5 py-16 text-[#0a0d12]">
      <div className="brand-grid absolute inset-0 opacity-70" />
      <div className="absolute -right-40 -top-48 h-[560px] w-[560px] rounded-full bg-[#00aae7]/15 blur-[110px]" />
      <section className="relative mx-auto w-full max-w-lg rounded-2xl border border-[#d5e4eb] bg-white p-7 shadow-[0_30px_90px_rgba(7,49,73,0.12)] sm:p-10">
        <a href="/" aria-label="Qiniu CI Runner home" className="inline-flex">
          <QiniuRunnerLogo />
        </a>
        {icon ? (
          <div className="mt-12 flex h-11 w-11 items-center justify-center rounded-lg border border-[#bdeaf8] bg-[#f2fcff] text-[#006b91]">
            {icon}
          </div>
        ) : null}
        <p className={`${icon ? "mt-7" : "mt-12"} font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-[#006b91]`}>
          {eyebrow}
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] sm:text-4xl">{title}</h1>
        <p className="mt-4 text-[15px] leading-7 text-[#596b75]">{description}</p>
        {children ? <div className="mt-8">{children}</div> : null}
        <a href="/" className="mt-9 inline-flex items-center gap-2 text-sm font-semibold text-[#006b91] hover:text-[#004e6b]">
          <ArrowLeft className="h-4 w-4" />
          Back to homepage
        </a>
      </section>
    </main>
  )
}

function signInURLFromReturnTo(returnTo: string): string {
  const parsed = returnTo.match(/^([^?]*)(.*)$/)
  return signInURL(parsed?.[1] || "/jobs", parsed?.[2] || "")
}

function protectedDestinationLabel(returnTo: string): string {
  if (returnTo.startsWith("/github/pulls/")) return "Pull request jobs"
  if (returnTo.startsWith("/github/runs/")) return "Workflow run jobs"
  if (returnTo.startsWith("/github/branches/")) return "Branch jobs"
  if (returnTo === "/repositories") return "Repositories"
  if (returnTo.startsWith("/account/") || returnTo.startsWith("/organizations/")) return "Account settings"
  if (returnTo.startsWith("/admin")) return "Admin console"
  if (returnTo !== "/jobs" && returnTo.startsWith("/jobs/")) return "Job details"
  return "Jobs"
}
