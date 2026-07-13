import { type Dispatch, type FormEvent, type SetStateAction } from "react"
import { Plus, RefreshCw, Trash2 } from "lucide-react"

import { formatTime } from "@/admin-format"
import { type RunnerGroup, type RunnerPolicy, type RunnerSpec } from "@/admin-types"
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"

export type RunnerPolicyFormState = {
  id: number
  repository_full_name: string
  target_type: string
  runner_spec_name: string
  runner_group_name: string
  enabled: boolean
}

export function RunnerPoliciesSection({
  loading,
  runnerPolicies,
  runnerSpecs,
  runnerGroups,
  runnerPolicyOpen,
  runnerPolicyForm,
  onRefresh,
  onCreateRunnerPolicy,
  onRunnerPolicyOpenChange,
  onRunnerPolicyFormChange,
  onSubmitRunnerPolicy,
  onEditRunnerPolicy,
  onDeleteRunnerPolicy,
}: {
  loading: boolean
  runnerPolicies: RunnerPolicy[]
  runnerSpecs: RunnerSpec[]
  runnerGroups: RunnerGroup[]
  runnerPolicyOpen: boolean
  runnerPolicyForm: RunnerPolicyFormState
  onRefresh: () => void
  onCreateRunnerPolicy: () => void
  onRunnerPolicyOpenChange: (open: boolean) => void
  onRunnerPolicyFormChange: Dispatch<SetStateAction<RunnerPolicyFormState>>
  onSubmitRunnerPolicy: (event: FormEvent<HTMLFormElement>) => void
  onEditRunnerPolicy: (policy: RunnerPolicy) => void
  onDeleteRunnerPolicy: (id: number) => void
}) {
  return (
    <div className="grid gap-4">
      <Card className="min-w-0">
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>Runner policies</CardTitle>
            <CardDescription>Click a policy row to edit it.</CardDescription>
          </div>
          <div className="flex gap-2">
            <Button type="button" onClick={onCreateRunnerPolicy}>
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
                <TableHead>Repository</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {runnerPolicies.map((policy) => (
                <TableRow key={policy.id} className="cursor-pointer" onClick={() => onEditRunnerPolicy(policy)}>
                  <TableCell><div className="max-w-[260px] truncate">{policy.repository_full_name}</div></TableCell>
                  <TableCell>
                    <div className="max-w-[260px] truncate">
                      {policy.runner_group_name
                        ? `group:${policy.runner_group_name}`
                        : `spec:${policy.runner_spec_name || "-"}`}
                    </div>
                  </TableCell>
                  <TableCell>{policy.enabled ? "yes" : "no"}</TableCell>
                  <TableCell>{formatTime(policy.created_at)}</TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={(event) => {
                        event.stopPropagation()
                        onDeleteRunnerPolicy(policy.id)
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
      <Dialog open={runnerPolicyOpen} onOpenChange={onRunnerPolicyOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{runnerPolicyForm.id > 0 ? "Edit runner policy" : "Create runner policy"}</DialogTitle>
            <DialogDescription>Bind a repository pattern to an allowed runner spec or group.</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={onSubmitRunnerPolicy}>
            <Input
              value={runnerPolicyForm.repository_full_name}
              onChange={(event) =>
                onRunnerPolicyFormChange((current) => ({ ...current, repository_full_name: event.target.value }))
              }
              placeholder="owner/repo or owner/*"
            />
            <Select
              value={runnerPolicyForm.target_type}
              onValueChange={(value) => onRunnerPolicyFormChange((current) => ({ ...current, target_type: value }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="target type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="group">Runner group</SelectItem>
                <SelectItem value="spec">Runner spec</SelectItem>
              </SelectContent>
            </Select>
            {runnerPolicyForm.target_type === "group" ? (
              runnerGroups.length === 0 ? (
                <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                  Create a runner group before adding group policies.
                </div>
              ) : (
                <Select
                  value={runnerPolicyForm.runner_group_name}
                  onValueChange={(value) =>
                    onRunnerPolicyFormChange((current) => ({ ...current, runner_group_name: value }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="runner group" />
                  </SelectTrigger>
                  <SelectContent>
                    {runnerGroups.map((group) => (
                      <SelectItem key={group.name} value={group.name}>
                        {group.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )
            ) : runnerSpecs.length === 0 ? (
              <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                Create a runner spec before adding runner policies.
              </div>
            ) : (
              <Select
                value={runnerPolicyForm.runner_spec_name}
                onValueChange={(value) =>
                  onRunnerPolicyFormChange((current) => ({ ...current, runner_spec_name: value }))
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="runner spec" />
                </SelectTrigger>
                <SelectContent>
                  {runnerSpecs.map((runnerSpec) => (
                    <SelectItem key={runnerSpec.name} value={runnerSpec.name}>
                      {runnerSpec.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={runnerPolicyForm.enabled}
                onChange={(event) =>
                  onRunnerPolicyFormChange((current) => ({ ...current, enabled: event.target.checked }))
                }
              />
              enabled
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onRunnerPolicyOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit">Save policy</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
