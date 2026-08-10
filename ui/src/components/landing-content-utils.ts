import type { TFunction } from "i18next"

const landingChecklistDefinitions = [
  { id: "orchestration", translationKey: "landing.checklist.orchestration" },
  { id: "policy", translationKey: "landing.checklist.policy" },
  { id: "cleanup", translationKey: "landing.checklist.cleanup" },
] as const

const deploymentRequirementIDs = ["github", "sandbox", "runner"] as const

export function landingChecklistItems(t: TFunction) {
  return landingChecklistDefinitions.map(({ id, translationKey }) => ({
    id,
    label: t(translationKey),
  }))
}

export function deploymentRequirementItems(t: TFunction) {
  const labels = t("landing.deploymentRequirements", { returnObjects: true }) as unknown as string[]
  return deploymentRequirementIDs.map((id, index) => ({ id, label: labels[index] }))
}
