import { type Dispatch, type FormEvent, type SetStateAction } from "react"
import { Pencil, Plus, RefreshCw, Trash2 } from "lucide-react"

import { type RunnerGroup, type RunnerSpec } from "@/admin-types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"

export type RunnerSpecFormState = {
  name: string
  labels: string
  required_labels: string
  template_id: string
  runner_group: string
  group_names: string[]
  max_concurrency: string
  min_idle: string
  priority: string
  enabled: boolean
  default_available: boolean
}

export function RunnerSpecDialogForm({
  runnerGroups,
  editingRunnerSpec,
  runnerSpecForm,
  onRunnerSpecFormChange,
  onRunnerSpecOpenChange,
  onSubmitRunnerSpec,
}: {
  runnerGroups: RunnerGroup[]
  editingRunnerSpec: RunnerSpec | null
  runnerSpecForm: RunnerSpecFormState
  onRunnerSpecFormChange: Dispatch<SetStateAction<RunnerSpecFormState>>
  onRunnerSpecOpenChange: (open: boolean) => void
  onSubmitRunnerSpec: (event: FormEvent<HTMLFormElement>) => void
}) {
  const managed = Boolean(editingRunnerSpec?.managed_by?.trim())

  return (
    <form className="grid gap-4" onSubmit={onSubmitRunnerSpec}>
      {managed ? (
        <div
          id="managed-runner-spec-note"
          className="flex items-start gap-3 rounded-md border bg-muted/35 px-3 py-2.5"
        >
          <Badge variant="secondary" className="mt-0.5">Managed</Badge>
          <p className="text-sm leading-5 text-muted-foreground">
            Catalog identity and routing fields are read-only. Capacity and availability remain operator controlled.
          </p>
        </div>
      ) : null}

      <div className="grid gap-2">
        <Label htmlFor="runner-spec-name">Name</Label>
        <Input
          id="runner-spec-name"
          value={runnerSpecForm.name}
          onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, name: event.target.value }))}
          placeholder="runner spec name"
          disabled={editingRunnerSpec !== null}
        />
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="runner-spec-labels">Labels</Label>
          <Input
            id="runner-spec-labels"
            value={runnerSpecForm.labels}
            onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, labels: event.target.value }))}
            placeholder="self-hosted,e2b"
            disabled={managed}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="runner-spec-required-labels">Required labels</Label>
          <Input
            id="runner-spec-required-labels"
            value={runnerSpecForm.required_labels}
            onChange={(event) =>
              onRunnerSpecFormChange((current) => ({ ...current, required_labels: event.target.value }))
            }
            placeholder="e2b"
            disabled={managed}
          />
          {!managed ? (
            <p className="text-xs text-muted-foreground">Every matching job must request these labels.</p>
          ) : null}
        </div>
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        {managed ? (
          <div className="grid gap-2">
            <Label htmlFor="runner-spec-default-template">Default template</Label>
            <Input
              id="runner-spec-default-template"
              value={editingRunnerSpec?.default_template_name || ""}
              disabled
            />
          </div>
        ) : (
          <div className="grid gap-2">
            <Label htmlFor="runner-spec-template-id">Template ID</Label>
            <Input
              id="runner-spec-template-id"
              value={runnerSpecForm.template_id}
              onChange={(event) =>
                onRunnerSpecFormChange((current) => ({ ...current, template_id: event.target.value }))
              }
              placeholder="template id"
            />
          </div>
        )}
        <div className="grid gap-2">
          <Label htmlFor="runner-spec-github-group">GitHub runner group</Label>
          <Input
            id="runner-spec-github-group"
            value={runnerSpecForm.runner_group}
            onChange={(event) =>
              onRunnerSpecFormChange((current) => ({ ...current, runner_group: event.target.value }))
            }
            placeholder="optional GitHub runner group"
            disabled={managed}
          />
        </div>
      </div>

      <fieldset className="grid gap-2 rounded-md border p-3" disabled={managed}>
        <legend className="px-1 text-sm font-medium">Internal runner groups</legend>
        {runnerGroups.length === 0 ? (
          <div className="text-sm text-muted-foreground">No internal runner groups configured.</div>
        ) : (
          runnerGroups.map((group) => (
            <label key={group.name} className="flex items-center gap-2 text-sm">
              <input
                id={`runner-spec-group-${group.name}`}
                type="checkbox"
                checked={runnerSpecForm.group_names.includes(group.name)}
                onChange={(event) =>
                  onRunnerSpecFormChange((current) => ({
                    ...current,
                    group_names: event.target.checked
                      ? [...current.group_names, group.name]
                      : current.group_names.filter((name) => name !== group.name),
                  }))
                }
                disabled={managed}
              />
              {group.name}
            </label>
          ))
        )}
      </fieldset>

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="grid gap-2">
          <Label htmlFor="runner-spec-max-concurrency">Max concurrency</Label>
          <Input
            id="runner-spec-max-concurrency"
            inputMode="numeric"
            value={runnerSpecForm.max_concurrency}
            onChange={(event) =>
              onRunnerSpecFormChange((current) => ({ ...current, max_concurrency: event.target.value }))
            }
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="runner-spec-min-idle">Min idle</Label>
          <Input
            id="runner-spec-min-idle"
            inputMode="numeric"
            value={runnerSpecForm.min_idle}
            onChange={(event) =>
              onRunnerSpecFormChange((current) => ({ ...current, min_idle: event.target.value }))
            }
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="runner-spec-priority">Priority</Label>
          <Input
            id="runner-spec-priority"
            inputMode="numeric"
            value={runnerSpecForm.priority}
            onChange={(event) =>
              onRunnerSpecFormChange((current) => ({ ...current, priority: event.target.value }))
            }
            disabled={managed}
          />
        </div>
      </div>

      <div className="grid gap-2">
        <label className="flex items-center gap-2 text-sm">
          <input
            id="runner-spec-enabled"
            type="checkbox"
            checked={runnerSpecForm.enabled}
            onChange={(event) =>
              onRunnerSpecFormChange((current) => ({ ...current, enabled: event.target.checked }))
            }
          />
          enabled
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            id="runner-spec-default-available"
            type="checkbox"
            checked={runnerSpecForm.default_available}
            onChange={(event) =>
              onRunnerSpecFormChange((current) => ({ ...current, default_available: event.target.checked }))
            }
            disabled={managed}
          />
          globally available by default
        </label>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onRunnerSpecOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit">Save runner spec</Button>
      </DialogFooter>
    </form>
  )
}

export function RunnerSpecsSection({
  loading,
  runnerSpecs,
  runnerGroups,
  runnerSpecOpen,
  editingRunnerSpec,
  runnerSpecForm,
  onRefresh,
  onResetRunnerSpecForm,
  onRunnerSpecOpenChange,
  onRunnerSpecFormChange,
  onSubmitRunnerSpec,
  onEditRunnerSpec,
  onDeleteRunnerSpec,
  groupNamesForSpec,
}: {
  loading: boolean
  runnerSpecs: RunnerSpec[]
  runnerGroups: RunnerGroup[]
  runnerSpecOpen: boolean
  editingRunnerSpec: RunnerSpec | null
  runnerSpecForm: RunnerSpecFormState
  onRefresh: () => void
  onResetRunnerSpecForm: () => void
  onRunnerSpecOpenChange: (open: boolean) => void
  onRunnerSpecFormChange: Dispatch<SetStateAction<RunnerSpecFormState>>
  onSubmitRunnerSpec: (event: FormEvent<HTMLFormElement>) => void
  onEditRunnerSpec: (runnerSpec: RunnerSpec) => void
  onDeleteRunnerSpec: (name: string) => void
  groupNamesForSpec: (specName: string) => string[]
}) {
  return (
    <div className="grid gap-4">
      <Card className="min-w-0">
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>Runner specs</CardTitle>
            <CardDescription>Click a runner spec row to edit it.</CardDescription>
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              onClick={() => {
                onResetRunnerSpecForm()
                onRunnerSpecOpenChange(true)
              }}
            >
              <Plus />
              Create
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon"
              onClick={onRefresh}
              disabled={loading}
              title="Refresh"
            >
              <RefreshCw className={cn(loading && "animate-spin")} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Labels</TableHead>
                <TableHead>Template</TableHead>
                <TableHead>GitHub group</TableHead>
                <TableHead>Runner groups</TableHead>
                <TableHead>Default</TableHead>
                <TableHead>Limit</TableHead>
                <TableHead className="w-44">
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runnerSpecs.map((runnerSpec) => {
                const managed = Boolean(runnerSpec.managed_by?.trim())
                return (
                <TableRow key={runnerSpec.name} className="cursor-pointer" onClick={() => onEditRunnerSpec(runnerSpec)}>
                  <TableCell>
                    <div className="flex max-w-[240px] items-center gap-2">
                      <span className="truncate">{runnerSpec.name}</span>
                      {managed ? <Badge variant="secondary">Managed</Badge> : null}
                    </div>
                  </TableCell>
                  <TableCell><div className="max-w-[260px] truncate">{runnerSpec.labels.join(", ")}</div></TableCell>
                  <TableCell>
                    <div className="max-w-[240px]">
                      <div className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                        {managed ? "Default template" : "Template ID"}
                      </div>
                      <div className="truncate">
                        {managed ? runnerSpec.default_template_name || "—" : runnerSpec.template_id}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell><div className="max-w-[220px] truncate">{runnerSpec.runner_group || "-"}</div></TableCell>
                  <TableCell><div className="max-w-[260px] truncate">{groupNamesForSpec(runnerSpec.name).join(", ") || "-"}</div></TableCell>
                  <TableCell>{runnerSpec.default_available ? "yes" : "no"}</TableCell>
                  <TableCell>{runnerSpec.max_concurrency}</TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        aria-label={`Edit ${runnerSpec.name}`}
                        onClick={(event) => {
                          event.stopPropagation()
                          onEditRunnerSpec(runnerSpec)
                        }}
                      >
                        <Pencil />
                        Edit
                      </Button>
                      {!managed ? (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={(event) => {
                            event.stopPropagation()
                            onDeleteRunnerSpec(runnerSpec.name)
                          }}
                        >
                          <Trash2 />
                          Delete
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Dialog open={runnerSpecOpen} onOpenChange={onRunnerSpecOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingRunnerSpec ? "Edit runner spec" : "Create runner spec"}</DialogTitle>
            <DialogDescription>Define labels, template, group membership, and capacity.</DialogDescription>
          </DialogHeader>
          <RunnerSpecDialogForm
            runnerGroups={runnerGroups}
            editingRunnerSpec={editingRunnerSpec}
            runnerSpecForm={runnerSpecForm}
            onRunnerSpecFormChange={onRunnerSpecFormChange}
            onRunnerSpecOpenChange={onRunnerSpecOpenChange}
            onSubmitRunnerSpec={onSubmitRunnerSpec}
          />
        </DialogContent>
      </Dialog>
    </div>
  )
}
