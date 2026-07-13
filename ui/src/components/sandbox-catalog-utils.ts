import type { SandboxInstance, SandboxTemplate } from "@/admin-types"

export type SandboxCatalogRequest = (url: string, options?: RequestInit) => Promise<unknown>

export const sandboxRegions = [
  {
    id: "us-south-1",
    label: "United States · Dallas 1",
    apiURL: "https://us-south-1-sandbox.qiniuapi.com",
  },
  {
    id: "cn-yangzhou-1",
    label: "China · Yangzhou 1",
    apiURL: "https://cn-yangzhou-1-sandbox.qiniuapi.com",
  },
]

function sandboxCatalogURL(path: string, region: string, installationID?: number, templateID = "") {
  const params = new URLSearchParams({ region })
  if (installationID) params.set("installation_id", String(installationID))
  if (templateID) params.set("template_id", templateID)
  return `/user/sandbox/${path}?${params.toString()}`
}

export async function loadSandboxTemplates(
  request: SandboxCatalogRequest,
  region: string,
  installationID?: number,
) {
  const data = await request(sandboxCatalogURL("templates", region, installationID))
  return Array.isArray(data) ? (data as SandboxTemplate[]) : []
}

export async function loadSandboxInstances(
  request: SandboxCatalogRequest,
  region: string,
  installationID?: number,
  templateID = "",
) {
  const data = await request(sandboxCatalogURL("instances", region, installationID, templateID))
  return Array.isArray(data) ? (data as SandboxInstance[]) : []
}

export function sandboxInstancesViewState({
  templatesLoading,
  instancesLoading,
  templatesError,
  instancesError,
}: {
  templatesLoading: boolean
  instancesLoading: boolean
  templatesError: string
  instancesError: string
}) {
  return {
    loading: templatesLoading || instancesLoading,
    error: instancesError,
    filterDisabled: templatesLoading || Boolean(templatesError),
  }
}

export function formatOptionalTime(value: string) {
  if (!value || value.startsWith("0001-01-01")) return "—"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString()
}
