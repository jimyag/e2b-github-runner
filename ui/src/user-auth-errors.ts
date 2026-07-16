export type RequestError = Error & { code?: string }

export function requiresGitHubReauthentication(error: unknown) {
  return error instanceof Error && (error as RequestError).code === "REAUTH_REQUIRED"
}

export function createGitHubReauthenticationGate() {
  let started = false
  return () => {
    if (started) return false
    started = true
    return true
  }
}
