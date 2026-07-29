import { ExternalLink, Sparkles } from "lucide-react"
import { useEffect, useMemo, useRef } from "react"
import {
  EVENTS,
  STATUS,
  useJoyride,
  type Step,
  type TooltipRenderProps,
} from "react-joyride"
import { toast } from "sonner"

import type { ProductTourOnboarding } from "@/admin-types"
import { Button } from "@/components/ui/button"
import {
  productTourStateAfterEnd,
  productTourSteps,
  shouldStartProductTour,
} from "@/user-onboarding"

type ProductTourStepID = (typeof productTourSteps)[number]["id"]

export function UserOnboardingTour({
  locationPath,
  onboarding,
  onNavigateRepositories,
  onStatusChange,
  replay,
}: {
  locationPath: string
  onboarding: ProductTourOnboarding | null
  onNavigateRepositories: () => void
  onStatusChange: (state: ProductTourOnboarding) => Promise<void>
  replay: boolean
}) {
  const started = useRef(false)
  const steps = useMemo<Step[]>(
    () => {
      const presentation: Record<ProductTourStepID, Omit<Step, "id" | "target">> = {
        welcome: {
          title: "Welcome to Qiniu Runner",
          content: "Here is a quick map of the workspace, followed by the one setup step that unlocks your own Sandbox service.",
          placement: "bottom-start",
        },
        jobs: {
          title: "Jobs",
          content: "Track workflow jobs by pull request, branch, workflow run, or manual request, then open logs and terminals from one place.",
          placement: "bottom",
        },
        repositories: {
          title: "Repositories",
          content: "Connect GitHub repositories and see whether each account or organization has the Sandbox service needed to run.",
          placement: "bottom",
        },
        "account-menu": {
          title: "Account menu",
          content: "Open Settings, switch the theme, replay this tour, or sign out from here.",
          placement: "bottom-end",
        },
        settings: {
          title: "Runner readiness",
          content: "Repository access and Sandbox service status are checked together for the selected account or organization.",
          placement: "bottom-start",
          before: async () => {
            onNavigateRepositories()
            await waitForNextPaint()
          },
        },
        "sandbox-service": {
          title: "Sandbox readiness",
          content: (
            <span>
              Ready accounts need no action. If setup is required, use Configure Sandbox to open Settings, then choose a region and get an API Key from the{" "}
              <a
                href="https://portal.qiniu.com/developer/user/api-key"
                target="_blank"
                rel="noreferrer"
                className="font-medium text-primary underline underline-offset-4"
              >
                Qiniu portal
                <ExternalLink className="ml-1 inline h-3.5 w-3.5" />
              </a>
              .
            </span>
          ),
          placement: "top-start",
        },
      }

      return productTourSteps.map((definition) => ({
        ...presentation[definition.id],
        id: definition.id,
        target: visibleOnboardingTarget(definition.target),
      }))
    },
    [onNavigateRepositories],
  )
  const { controls, Tour } = useJoyride({
    continuous: true,
    steps,
    tooltipComponent: ProductTourTooltip,
    options: {
      buttons: ["back", "skip", "primary"],
      blockTargetInteraction: true,
      dismissKeyAction: false,
      overlayClickAction: false,
      showProgress: true,
      skipBeacon: true,
      spotlightPadding: 8,
      spotlightRadius: 8,
      targetWaitTimeout: 5000,
      zIndex: 80,
    },
    onEvent: (data) => {
      if (data.type !== EVENTS.TOUR_END) return
      if (data.status !== STATUS.FINISHED && data.status !== STATUS.SKIPPED) return
      const nextState = productTourStateAfterEnd(onboarding, replay, data.status)
      if (!nextState) return
      void onStatusChange(nextState).catch((error) => {
        toast.error(error instanceof Error ? error.message : "Failed to save product tour progress")
      })
    },
  })

  useEffect(() => {
    if (!shouldStartProductTour(locationPath, onboarding, replay) || started.current) return
    started.current = true
    controls.start()
  }, [controls, locationPath, onboarding, replay])

  return Tour
}

function ProductTourTooltip({
  backProps,
  index,
  isLastStep,
  primaryProps,
  size,
  skipProps,
  step,
  tooltipProps,
}: TooltipRenderProps) {
  return (
    <div
      {...tooltipProps}
      className="w-[min(24rem,calc(100vw-2rem))] rounded-xl border bg-background p-5 text-foreground shadow-2xl"
    >
      <div className="mb-4 flex items-center justify-between gap-4">
        <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          <Sparkles className="h-3.5 w-3.5 text-primary" />
          Quick tour
        </div>
        <div className="font-mono text-xs text-muted-foreground">
          {index + 1}/{size}
        </div>
      </div>
      {step.title ? <h2 className="text-lg font-semibold">{step.title}</h2> : null}
      <div className="mt-2 text-sm leading-6 text-muted-foreground">{step.content}</div>
      <div className="mt-5 flex items-center justify-between gap-3">
        <Button {...skipProps} type="button" variant="ghost" size="sm">
          Skip tour
        </Button>
        <div className="ml-auto flex items-center gap-2">
          {index > 0 ? (
            <Button {...backProps} type="button" variant="outline" size="sm">
              Back
            </Button>
          ) : null}
          <Button {...primaryProps} type="button" size="sm">
            {isLastStep ? "Configure API Key" : "Next"}
          </Button>
        </div>
      </div>
    </div>
  )
}

function visibleOnboardingTarget(name: string) {
  return () => {
    const matches = document.querySelectorAll<HTMLElement>(`[data-onboarding="${name}"]`)
    return Array.from(matches).find((element) => element.getClientRects().length > 0) ?? null
  }
}

function waitForNextPaint(): Promise<void> {
  return new Promise((resolve) => {
    window.requestAnimationFrame(() => window.requestAnimationFrame(() => resolve()))
  })
}
