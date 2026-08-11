import type { LocalizedLogText } from "@/app-log-state"

export function githubLogFailureState(error: unknown): LocalizedLogText {
  return error instanceof Error
    ? { kind: "text", text: error.message }
    : { kind: "message", key: "user.githubLogFailed" }
}
