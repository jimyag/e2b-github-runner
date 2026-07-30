export function getLocalAuthSessionRoute(externalBaseURL: string | undefined) {
  if (externalBaseURL) {
    return null
  }

  return {
    pattern: "**/auth/session",
    json: {
      authenticated: false,
      oauth_enabled: false,
    },
  }
}
