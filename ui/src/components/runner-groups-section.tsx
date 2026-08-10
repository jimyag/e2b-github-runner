import { type Dispatch, type FormEvent, type SetStateAction } from "react"
import { Plus, RefreshCw, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

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
  const { t, i18n } = useTranslation()
  return (
    <div className="grid gap-4">
      <Card className="min-w-0">
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>{t("sidebar.runnerGroups")}</CardTitle>
            <CardDescription>{t("admin.groupsDescription")}</CardDescription>
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
              {t("admin.createRunnerGroup")}
            </Button>
            <Button type="button" variant="outline" size="icon" onClick={onRefresh} disabled={loading} title={t("common.refresh")}>
              <RefreshCw className={cn(loading && "animate-spin")} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("admin.specs")}</TableHead>
                <TableHead>{t("common.enabled")}</TableHead>
                <TableHead>{t("common.updated")}</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {runnerGroups.map((group) => (
                <TableRow key={group.name} className="cursor-pointer" onClick={() => onEditRunnerGroup(group)}>
                  <TableCell><div className="max-w-[220px] truncate">{group.name}</div></TableCell>
                  <TableCell><div className="max-w-[420px] truncate">{group.spec_names.join(", ") || "-"}</div></TableCell>
                  <TableCell>{group.enabled ? t("common.yes") : t("common.no")}</TableCell>
                  <TableCell>{formatTime(group.updated_at, i18n.resolvedLanguage)}</TableCell>
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
                      {t("common.delete")}
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
            <DialogTitle>{runnerGroupForm.name ? t("admin.editRunnerGroup") : t("admin.createRunnerGroup")}</DialogTitle>
            <DialogDescription>{t("admin.groupDialogDescription")}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={onSubmitRunnerGroup}>
            <Input
              value={runnerGroupForm.name}
              onChange={(event) => onRunnerGroupFormChange((current) => ({ ...current, name: event.target.value }))}
              placeholder={t("admin.runnerGroupNamePlaceholder")}
            />
            <Input
              value={runnerGroupForm.description}
              onChange={(event) => onRunnerGroupFormChange((current) => ({ ...current, description: event.target.value }))}
              placeholder={t("admin.descriptionPlaceholder")}
            />
            <div className="grid gap-2 rounded-md border p-3">
              {runnerSpecs.length === 0 ? (
                <div className="text-sm text-muted-foreground">{t("admin.noSpecsForGroup")}</div>
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
              {t("common.enabled")}
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onRunnerGroupOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit">{t("admin.saveRunnerGroup")}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
