import type { UserRunnerSpec } from "@/admin-types"
import { sandboxRegions, type SandboxRegion } from "@/components/sandbox-catalog-utils"

export function runnerTypeCatalogRegion(regionID: string, regions: readonly SandboxRegion[] = sandboxRegions) {
	const catalog = regions.length > 0 ? regions : sandboxRegions
	return catalog.some((region) => region.id === regionID) ? regionID : catalog[0].id
}

export function runnerTypeWorkflowYAML(labels: string[]) {
  return `runs-on: [${labels.map((label) => JSON.stringify(label)).join(", ")}]`
}

export function runnerTypeOverridesGlobal(labels: string[], items: UserRunnerSpec[]) {
  const key = [...new Set(labels.map((label) => label.trim()).filter(Boolean))].sort().join("\u0000")
  return key !== "" && items.some((item) => item.source !== "scoped_custom" && [...new Set(item.workflow_labels)].sort().join("\u0000") === key)
}
