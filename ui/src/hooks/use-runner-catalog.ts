import { type Dispatch, type FormEvent, type SetStateAction, useCallback, useState } from "react"
import { toast } from "sonner"

import { type RunnerGroupFormState } from "@/components/runner-groups-section"
import { type RunnerPolicyFormState } from "@/components/runner-policies-section"
import { type RunnerSpecFormState } from "@/components/runner-specs-section"
import { type AdminSection, type RunnerGroup, type RunnerPolicy, type RunnerSpec } from "@/admin-types"

type RequestFn = (url: string, options?: RequestInit) => Promise<unknown>

export function useRunnerCatalog({
  runnerSpecs,
  runnerGroups,
  setRunnerPolicies,
  request,
  loadAll,
  setSection,
  parseLabels,
}: {
  runnerSpecs: RunnerSpec[]
  runnerGroups: RunnerGroup[]
  setRunnerPolicies: Dispatch<SetStateAction<RunnerPolicy[]>>
  request: RequestFn
  loadAll: () => Promise<void>
  setSection: (next: AdminSection) => void
  parseLabels: (value: string) => string[]
}) {
  const [runnerSpecOpen, setRunnerSpecOpen] = useState(false)
  const [runnerGroupOpen, setRunnerGroupOpen] = useState(false)
  const [runnerPolicyOpen, setRunnerPolicyOpen] = useState(false)
  const [runnerSpecForm, setRunnerSpecForm] = useState<RunnerSpecFormState>({
    name: "",
    labels: "self-hosted,e2b",
    template_id: "",
    runner_group: "",
    group_names: [],
    max_concurrency: "10",
    min_idle: "0",
    priority: "0",
    enabled: true,
    default_available: true,
  })
  const [runnerPolicyForm, setPolicyForm] = useState<RunnerPolicyFormState>({
    id: 0,
    repository_full_name: "",
    target_type: "spec",
    runner_spec_name: "",
    runner_group_name: "",
    enabled: true,
  })
  const [runnerGroupForm, setRunnerGroupForm] = useState<RunnerGroupFormState>({
    name: "",
    description: "",
    spec_names: [],
    enabled: true,
  })

  const groupNamesForSpec = useCallback(
    (specName: string) =>
      runnerGroups
        .filter((group) => group.spec_names.includes(specName))
        .map((group) => group.name),
    [runnerGroups]
  )

  const resetRunnerSpecForm = () => {
    setRunnerSpecForm({
      name: "",
      labels: "self-hosted,e2b",
      template_id: "",
      runner_group: "",
      group_names: [],
      max_concurrency: "10",
      min_idle: "0",
      priority: "0",
      enabled: true,
      default_available: true,
    })
  }

  const resetRunnerPolicyForm = () => {
    setPolicyForm({
      id: 0,
      repository_full_name: "",
      target_type: "spec",
      runner_spec_name: "",
      runner_group_name: "",
      enabled: true,
    })
  }

  const resetRunnerGroupForm = () => {
    setRunnerGroupForm({ name: "", description: "", spec_names: [], enabled: true })
  }

  const createRunnerPolicy = () => {
    if (runnerSpecs.length === 0 && runnerGroups.length === 0) {
      toast.error("Create a runner spec or runner group before adding policies")
      setSection("runner_specs")
      return
    }
    setPolicyForm({
      id: 0,
      repository_full_name: "",
      target_type: runnerGroups.length > 0 ? "group" : "spec",
      runner_spec_name: runnerSpecs[0]?.name || "",
      runner_group_name: runnerGroups[0]?.name || "",
      enabled: true,
    })
    setRunnerPolicyOpen(true)
  }

  const saveRunnerSpec = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      const payload = {
        name: runnerSpecForm.name.trim(),
        labels: parseLabels(runnerSpecForm.labels),
        template_id: runnerSpecForm.template_id.trim(),
        runner_group: runnerSpecForm.runner_group.trim(),
        max_concurrency: Number(runnerSpecForm.max_concurrency) || 0,
        min_idle: Number(runnerSpecForm.min_idle) || 0,
        priority: Number(runnerSpecForm.priority) || 0,
        enabled: runnerSpecForm.enabled,
        default_available: runnerSpecForm.default_available,
      }
      const isUpdate = runnerSpecs.some((runnerSpec) => runnerSpec.name === payload.name)
      const url = isUpdate ? `/runner_specs/${encodeURIComponent(payload.name)}` : "/runner_specs"
      const method = isUpdate ? "PATCH" : "POST"
      await request(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      })
      for (const group of runnerGroups) {
        const shouldContain = runnerSpecForm.group_names.includes(group.name)
        const currentSpecs = new Set(group.spec_names)
        if (shouldContain === currentSpecs.has(payload.name)) {
          continue
        }
        if (shouldContain) currentSpecs.add(payload.name)
        else currentSpecs.delete(payload.name)
        await request(`/runner_groups/${encodeURIComponent(group.name)}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            spec_names: Array.from(currentSpecs).sort(),
            enabled: group.enabled,
          }),
        })
      }
      toast.success(`Runner spec ${payload.name} saved`)
      setRunnerSpecOpen(false)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save runner spec")
    }
  }

  const loadRunnerSpecIntoForm = (runnerSpec: RunnerSpec) => {
    setSection("runner_specs")
    setRunnerSpecForm({
      name: runnerSpec.name,
      labels: runnerSpec.labels.join(","),
      template_id: runnerSpec.template_id,
      runner_group: runnerSpec.runner_group || "",
      group_names: runnerGroups
        .filter((group) => group.spec_names.includes(runnerSpec.name))
        .map((group) => group.name),
      max_concurrency: String(runnerSpec.max_concurrency),
      min_idle: String(runnerSpec.min_idle),
      priority: String(runnerSpec.priority),
      enabled: runnerSpec.enabled,
      default_available: runnerSpec.default_available,
    })
    setRunnerSpecOpen(true)
  }

  const deleteRunnerSpec = async (name: string) => {
    try {
      await request(`/runner_specs/${encodeURIComponent(name)}`, { method: "DELETE" })
      toast.success(`Runner spec ${name} deleted`)
      if (runnerSpecForm.name === name) {
        resetRunnerSpecForm()
      }
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete runner spec")
    }
  }

  const saveRunnerGroup = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      const payload = {
        name: runnerGroupForm.name.trim(),
        description: runnerGroupForm.description.trim(),
        spec_names: runnerGroupForm.spec_names,
        enabled: runnerGroupForm.enabled,
      }
      const isUpdate = runnerGroups.some((group) => group.name === payload.name)
      const url = isUpdate ? `/runner_groups/${encodeURIComponent(payload.name)}` : "/runner_groups"
      const method = isUpdate ? "PATCH" : "POST"
      await request(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      })
      toast.success(`Runner group ${payload.name} saved`)
      setRunnerGroupOpen(false)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save runner group")
    }
  }

  const loadRunnerGroupIntoForm = (group: RunnerGroup) => {
    setSection("runner_groups")
    setRunnerGroupForm({
      name: group.name,
      description: group.description || "",
      spec_names: [...group.spec_names],
      enabled: group.enabled,
    })
    setRunnerGroupOpen(true)
  }

  const deleteRunnerGroup = async (name: string) => {
    try {
      await request(`/runner_groups/${encodeURIComponent(name)}`, { method: "DELETE" })
      toast.success(`Runner group ${name} deleted`)
      if (runnerGroupForm.name === name) resetRunnerGroupForm()
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete runner group")
    }
  }

  const savePolicy = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      const payload: {
        repository_full_name: string
        runner_spec_name?: string
        runner_group_name?: string
        enabled: boolean
      } = {
        repository_full_name: runnerPolicyForm.repository_full_name.trim(),
        enabled: runnerPolicyForm.enabled,
      }
      if (runnerPolicyForm.target_type === "group") payload.runner_group_name = runnerPolicyForm.runner_group_name.trim()
      else payload.runner_spec_name = runnerPolicyForm.runner_spec_name.trim()
      const isUpdate = runnerPolicyForm.id > 0
      const url = isUpdate ? `/runner_policies/${runnerPolicyForm.id}` : "/runner_policies"
      const method = isUpdate ? "PATCH" : "POST"
      const saved = (await request(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      })) as RunnerPolicy
      setRunnerPolicies((current) => {
        const index = current.findIndex((policy) => policy.id === saved.id)
        if (index === -1) return [saved, ...current]
        const next = [...current]
        next[index] = saved
        return next
      })
      toast.success(`Runner policy #${saved.id} saved`)
      setRunnerPolicyOpen(false)
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save runner policy")
    }
  }

  const loadPolicyIntoForm = (policy: RunnerPolicy) => {
    setSection("runner_policies")
    setPolicyForm({
      id: policy.id,
      repository_full_name: policy.repository_full_name,
      target_type: policy.runner_group_name ? "group" : "spec",
      runner_spec_name: policy.runner_spec_name || "",
      runner_group_name: policy.runner_group_name || "",
      enabled: policy.enabled,
    })
    setRunnerPolicyOpen(true)
  }

  const deletePolicy = async (id: number) => {
    try {
      await request(`/runner_policies/${id}`, { method: "DELETE" })
      toast.success("Runner policy deleted")
      if (runnerPolicyForm.id === id) {
        resetRunnerPolicyForm()
      }
      await loadAll()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete runner policy")
    }
  }

  return {
    runnerSpecOpen,
    runnerGroupOpen,
    runnerPolicyOpen,
    runnerSpecForm,
    runnerGroupForm,
    runnerPolicyForm,
    setRunnerSpecOpen,
    setRunnerGroupOpen,
    setRunnerPolicyOpen,
    setRunnerSpecForm,
    setRunnerGroupForm,
    setPolicyForm,
    groupNamesForSpec,
    resetRunnerSpecForm,
    resetRunnerGroupForm,
    createRunnerPolicy,
    saveRunnerSpec,
    loadRunnerSpecIntoForm,
    deleteRunnerSpec,
    saveRunnerGroup,
    loadRunnerGroupIntoForm,
    deleteRunnerGroup,
    savePolicy,
    loadPolicyIntoForm,
    deletePolicy,
  }
}
