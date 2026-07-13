export function formatOptionalTime(value: string) {
  if (!value || value.startsWith("0001-01-01")) return "—"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString()
}
