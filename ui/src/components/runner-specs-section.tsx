import { type Dispatch, type FormEvent, type SetStateAction } from "react"
import { Plus, RefreshCw, Trash2 } from "lucide-react"

import { type RunnerGroup, type RunnerSpec } from "@/admin-types"
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
  template_id: string
  runner_group: string
  group_names: string[]
  max_concurrency: string
  min_idle: string
  priority: string
  enabled: boolean
  default_available: boolean
}

export function RunnerSpecsSection({
  loading,
  runnerSpecs,
  runnerGroups,
  runnerSpecOpen,
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
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {runnerSpecs.map((runnerSpec) => (
                <TableRow key={runnerSpec.name} className="cursor-pointer" onClick={() => onEditRunnerSpec(runnerSpec)}>
                  <TableCell><div className="max-w-[220px] truncate">{runnerSpec.name}</div></TableCell>
                  <TableCell><div className="max-w-[260px] truncate">{runnerSpec.labels.join(", ")}</div></TableCell>
                  <TableCell><div className="max-w-[220px] truncate">{runnerSpec.template_id}</div></TableCell>
                  <TableCell><div className="max-w-[220px] truncate">{runnerSpec.runner_group || "-"}</div></TableCell>
                  <TableCell><div className="max-w-[260px] truncate">{groupNamesForSpec(runnerSpec.name).join(", ") || "-"}</div></TableCell>
                  <TableCell>{runnerSpec.default_available ? "yes" : "no"}</TableCell>
                  <TableCell>{runnerSpec.max_concurrency}</TableCell>
                  <TableCell>
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
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Dialog open={runnerSpecOpen} onOpenChange={onRunnerSpecOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{runnerSpecForm.name ? "Edit runner spec" : "Create runner spec"}</DialogTitle>
            <DialogDescription>Define labels, template, group membership, and capacity.</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={onSubmitRunnerSpec}>
            <Input
              value={runnerSpecForm.name}
              onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, name: event.target.value }))}
              placeholder="runner spec name"
            />
            <Input
              value={runnerSpecForm.labels}
              onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, labels: event.target.value }))}
              placeholder="self-hosted,e2b"
            />
            <div className="grid gap-2 sm:grid-cols-2">
              <Input
                value={runnerSpecForm.template_id}
                onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, template_id: event.target.value }))}
                placeholder="template id"
              />
              <Input
                value={runnerSpecForm.runner_group}
                onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, runner_group: event.target.value }))}
                placeholder="optional GitHub runner group"
              />
            </div>
            <div className="grid gap-2 rounded-md border p-3">
              {runnerGroups.length === 0 ? (
                <div className="text-sm text-muted-foreground">No internal runner groups configured.</div>
              ) : (
                runnerGroups.map((group) => (
                  <label key={group.name} className="flex items-center gap-2 text-sm">
                    <input
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
                    />
                    {group.name}
                  </label>
                ))
              )}
            </div>
            <div className="grid grid-cols-3 gap-2">
              <Input
                value={runnerSpecForm.max_concurrency}
                onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, max_concurrency: event.target.value }))}
                placeholder="max concurrency"
              />
              <Input
                value={runnerSpecForm.min_idle}
                onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, min_idle: event.target.value }))}
                placeholder="min idle"
              />
              <Input
                value={runnerSpecForm.priority}
                onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, priority: event.target.value }))}
                placeholder="priority"
              />
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={runnerSpecForm.enabled}
                onChange={(event) => onRunnerSpecFormChange((current) => ({ ...current, enabled: event.target.checked }))}
              />
              enabled
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={runnerSpecForm.default_available}
                onChange={(event) =>
                  onRunnerSpecFormChange((current) => ({ ...current, default_available: event.target.checked }))
                }
              />
              globally available by default
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onRunnerSpecOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit">Save runner spec</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
