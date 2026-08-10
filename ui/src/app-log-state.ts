import type { TFunction } from "i18next"

export function runnerLogTextForView(selectedID: string, logText: string, t: TFunction) {
  return selectedID ? logText : t("app.noRunnerSelected")
}
