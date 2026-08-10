import type { TFunction } from "i18next"

export function githubLogFailureMessage(error: unknown, t: TFunction) {
  return error instanceof Error ? error.message : t("user.githubLogFailed")
}
