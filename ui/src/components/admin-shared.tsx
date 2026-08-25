import { type ReactNode } from "react"
import { AlertCircle, CheckCircle2, Clock3, Loader2, Play, Square } from "lucide-react"

import { type RunnerDisplayStatus } from "@/admin-types"
import { runnerStatusLabel } from "@/admin-format"
import { Badge } from "@/components/ui/badge"

export function StatusBadge({ status }: { status: RunnerDisplayStatus }) {
  if (status === "running") {
    return (
      <Badge variant="success">
        <Play />
        {runnerStatusLabel(status)}
      </Badge>
    )
  }
  if (status === "failed") {
    return (
      <Badge variant="danger">
        <AlertCircle />
        {runnerStatusLabel(status)}
      </Badge>
    )
  }
  if (status === "unmatched") {
    return (
      <Badge variant="secondary">
        <AlertCircle />
        {runnerStatusLabel(status)}
      </Badge>
    )
  }
  if (status === "completed") {
    return (
      <Badge variant="outline">
        <Square />
        {runnerStatusLabel(status)}
      </Badge>
    )
  }
  if (status === "creating") {
    return (
      <Badge variant="warning">
        <Loader2 className="animate-spin" />
        {runnerStatusLabel(status)}
      </Badge>
    )
  }
  if (status === "stopping") {
    return (
      <Badge variant="warning">
        <Clock3 />
        {runnerStatusLabel(status)}
      </Badge>
    )
  }
  return (
    <Badge variant="secondary">
      <CheckCircle2 />
      {runnerStatusLabel(status)}
    </Badge>
  )
}

export function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="grid grid-cols-[110px_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
      <div className="text-muted-foreground">{label}</div>
      <div className="min-w-0 break-words font-medium">{value}</div>
    </div>
  )
}
