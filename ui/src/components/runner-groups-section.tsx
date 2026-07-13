import { type Dispatch, type FormEvent, type SetStateAction } from "react"
import { Plus, RefreshCw, Trash2 } from "lucide-react"

import { formatTime } from "@/admin-format"
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

export type RunnerGroupFormState = {
  name: string
  description: string
  spec_names: string[]
  enabled: boolean
}

export function RunnerGroupsSection({
  loading,
  runnerGroups,
  runnerSpecs,
  runnerGroupOpen,
  runnerGroupForm,
  onRefresh,
  onResetRunnerGroupForm,
  onRunnerGroupOpenChange,
  onRunnerGroupFormChange,
  onSubmitRunnerGroup,
  onEditRunnerGroup,
  onDeleteRunnerGroup,
}: {
  loading: boolean
  runnerGroups: RunnerGroup[]
  runnerSpecs: RunnerSpec[]
  runnerGroupOpen: boolean
  runnerGroupForm: RunnerGroupFormState
  onRefresh: () => void
  onResetRunnerGroupForm: () => void
  onRunnerGroupOpenChange: (open: boolean) => void
  onRunnerGroupFormChange: Dispatch<SetStateAction<RunnerGroupFormState>>
  onSubmitRunnerGroup: (event: FormEvent<HTMLFormElement>) => void
  onEditRunnerGroup: (group: RunnerGroup) => void
  onDeleteRunnerGroup: (name: string) => void
}) {
  return (
    <div className="grid gap-4">
      <Card className="min-w-0">
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>Runner groups</CardTitle>
            <CardDescription>Click a group row to edit its runner specs.</CardDescription>
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              onClick={() => {
                onResetRunnerGroupForm()
                onRunnerGroupOpenChange(true)
              }}
            >
              <Plus />
              Create
            </Button>
            <Button type="button" variant="outline" size="icon" onClick={onRefresh} disabled={loading} title="Refresh">
              <RefreshCw className={cn(loading && "animate-spin")} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Specs</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Updated</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {runnerGroups.map((group) => (
                <TableRow key={group.name} className="cursor-pointer" onClick={() => onEditRunnerGroup(group)}>
                  <TableCell><div className="max-w-[220px] truncate">{group.name}</div></TableCell>
                  <TableCell><div className="max-w-[420px] truncate">{group.spec_names.join(", ") || "-"}</div></TableCell>
                  <TableCell>{group.enabled ? "yes" : "no"}</TableCell>
                  <TableCell>{formatTime(group.updated_at)}</TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={(event) => {
                        event.stopPropagation()
                        onDeleteRunnerGroup(group.name)
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
      <Dialog open={runnerGroupOpen} onOpenChange={onRunnerGroupOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{runnerGroupForm.name ? "Edit runner group" : "Create runner group"}</DialogTitle>
            <DialogDescription>Group runner specs so repositories can allow a named set.</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={onSubmitRunnerGroup}>
            <Input
              value={runnerGroupForm.name}
              onChange={(event) => onRunnerGroupFormChange((current) => ({ ...current, name: event.target.value }))}
              placeholder="runner group name"
            />
            <Input
              value={runnerGroupForm.description}
              onChange={(event) => onRunnerGroupFormChange((current) => ({ ...current, description: event.target.value }))}
              placeholder="description"
            />
            <div className="grid gap-2 rounded-md border p-3">
              {runnerSpecs.length === 0 ? (
                <div className="text-sm text-muted-foreground">Create a runner spec before adding specs to a group.</div>
              ) : (
                runnerSpecs.map((runnerSpec) => (
                  <label key={runnerSpec.name} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={runnerGroupForm.spec_names.includes(runnerSpec.name)}
                      onChange={(event) =>
                        onRunnerGroupFormChange((current) => ({
                          ...current,
                          spec_names: event.target.checked
                            ? [...current.spec_names, runnerSpec.name]
                            : current.spec_names.filter((name) => name !== runnerSpec.name),
                        }))
                      }
                    />
                    {runnerSpec.name}
                  </label>
                ))
              )}
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={runnerGroupForm.enabled}
                onChange={(event) => onRunnerGroupFormChange((current) => ({ ...current, enabled: event.target.checked }))}
              />
              enabled
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onRunnerGroupOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit">Save runner group</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
