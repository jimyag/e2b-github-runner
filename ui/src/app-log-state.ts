import type { TFunction } from "i18next"

export type LocalizedLogMessageKey =
  | "app.loadFailed"
  | "common.loading"
  | "user.githubLogEmpty"
  | "user.githubLogFailed"
  | "user.loadingGitHubLog"
  | "user.loadingRunnerLog"
  | "user.runnerLogEmpty"
  | "user.runnerLogFailed"

export type LocalizedLogText =
  | { kind: "text"; text: string }
  | { kind: "message"; key: LocalizedLogMessageKey }

export function localizedLogTextForView(logText: LocalizedLogText, t: TFunction) {
  return logText.kind === "message" ? t(logText.key) : logText.text
}

export function runnerLogTextForView(selectedID: string, logText: LocalizedLogText, t: TFunction) {
  if (!selectedID) return t("app.noRunnerSelected")
  return localizedLogTextForView(logText, t)
}
