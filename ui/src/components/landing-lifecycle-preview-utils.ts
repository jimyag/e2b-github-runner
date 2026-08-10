import { Clock3, Code2, Github, Layers3 } from "lucide-react"
import type { TFunction } from "i18next"

export function runnerPolicyItems(t: TFunction) {
  return [
    { id: "repository", label: t("landing.preview.repository"), value: "qiniu/ci-runner" },
    { id: "runner-spec", label: t("landing.preview.runnerSpec"), value: "ubuntu-24.04" },
    { id: "isolation", label: t("landing.preview.isolation"), value: t("landing.preview.isolationValue") },
  ]
}

export function runnerLifecycleItems(t: TFunction) {
  return [
    { id: "github-accepted", icon: Github, title: t("landing.preview.githubAccepted"), detail: "workflow_job · queued", state: "complete" },
    { id: "sandbox-created", icon: Layers3, title: t("landing.preview.sandboxCreated"), detail: "us-south-1 · 8.4s", state: "complete" },
    { id: "runner-registered", icon: Code2, title: t("landing.preview.runnerRegistered"), detail: "ephemeral · online", state: "complete" },
    { id: "job-executing", icon: Clock3, title: t("landing.preview.jobExecuting"), detail: "tests · 01:42", state: "active" },
  ] as const
}
