import { type Dispatch, type FormEvent, type SetStateAction } from "react"
import { Plus, RefreshCw, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

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
  const { t, i18n } = useTranslation()
  return (
    <div className="grid gap-4">
      <Card className="min-w-0">
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>{t("sidebar.runnerPolicies")}</CardTitle>
            <CardDescription>{t("admin.policiesDescription")}</CardDescription>
          </div>
          <div className="flex gap-2">
            <Button type="button" onClick={onCreateRunnerPolicy}>
              <Plus />
              {t("admin.createPolicy")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon"
              onClick={onRefresh}
              disabled={loading}
              title={t("common.refresh")}
            >
              <RefreshCw className={cn(loading && "animate-spin")} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.repository")}</TableHead>
                <TableHead>{t("admin.target")}</TableHead>
                <TableHead>{t("common.enabled")}</TableHead>
                <TableHead>{t("common.created")}</TableHead>
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
                  <TableCell>{policy.enabled ? t("common.yes") : t("common.no")}</TableCell>
                  <TableCell>{formatTime(policy.created_at, i18n.resolvedLanguage)}</TableCell>
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
                      {t("common.delete")}
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
            <DialogTitle>{runnerPolicyForm.id > 0 ? t("admin.editPolicy") : t("admin.createPolicy")}</DialogTitle>
            <DialogDescription>{t("admin.policyDialogDescription")}</DialogDescription>
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
                <SelectValue placeholder={t("admin.targetTypePlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="group">{t("admin.runnerGroup")}</SelectItem>
                <SelectItem value="spec">{t("common.runnerSpec")}</SelectItem>
              </SelectContent>
            </Select>
            {runnerPolicyForm.target_type === "group" ? (
              runnerGroups.length === 0 ? (
                <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                  {t("admin.createGroupBeforePolicy")}
                </div>
              ) : (
                <Select
                  value={runnerPolicyForm.runner_group_name}
                  onValueChange={(value) =>
                    onRunnerPolicyFormChange((current) => ({ ...current, runner_group_name: value }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder={t("admin.runnerGroupPlaceholder")} />
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
                {t("admin.createSpecBeforePolicy")}
              </div>
            ) : (
              <Select
                value={runnerPolicyForm.runner_spec_name}
                onValueChange={(value) =>
                  onRunnerPolicyFormChange((current) => ({ ...current, runner_spec_name: value }))
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder={t("admin.runnerSpecPlaceholder")} />
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
              {t("common.enabled")}
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onRunnerPolicyOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit">{t("admin.savePolicy")}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
