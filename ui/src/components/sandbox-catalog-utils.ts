import { createContext, useContext } from "react"
import type { SandboxInstance, SandboxTemplate } from "@/admin-types"

export type SandboxCatalogRequest = (url: string, options?: RequestInit) => Promise<unknown>

export type SandboxRegion = {
  id: string
  label: string
  apiURL: string
  s3Region?: string
  s3Endpoint?: string
}

/** Region catalog provided by runnerd.yaml through GET /sandbox/regions. */
export const SandboxRegionsContext = createContext<SandboxRegion[]>([])

export function useSandboxRegions(): SandboxRegion[] {
  return useContext(SandboxRegionsContext)
}

export function findSandboxRegionByAPIURL(regions: readonly SandboxRegion[], value: string) {
  const normalized = value.trim().replace(/\/+$/, "").toLowerCase()
  return regions.find((region) => region.apiURL.trim().replace(/\/+$/, "").toLowerCase() === normalized)
}

/**
 * Fetch sandbox regions from the server and return them, or null on error.
 */
export async function fetchSandboxRegions(): Promise<SandboxRegion[] | null> {
  try {
    const res = await fetch("/sandbox/regions")
    if (!res.ok) return null
    const data = await res.json()
    if (!Array.isArray(data) || data.length === 0) return null
    return data.map((item: { id: string; label?: string; api_url?: string; s3_region?: string; s3_endpoint?: string }) => ({
      id: item.id,
      label: item.label ?? item.id,
      apiURL: item.api_url ?? "",
      s3Region: item.s3_region || undefined,
      s3Endpoint: item.s3_endpoint || undefined,
    }))
  } catch {
    return null
  }
}

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

export function formatOptionalTime(value: string, locale?: string) {
  if (!value || value.startsWith("0001-01-01")) return "—"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString(locale)
}
