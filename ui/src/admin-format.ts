export function formatTime(value?: string) {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

export function formatRunnerDuration(job: {
  running_at?: string
  created_at?: string
  completed_at?: string
  failed_at?: string
  updated_at?: string
}) {
  const start = timeValue(job.running_at || job.created_at)
  const end = timeValue(job.completed_at || job.failed_at || job.updated_at)
  if (!start || !end || end <= start) return ""
  const totalSeconds = Math.max(0, Math.round((end - start) / 1000))
  if (totalSeconds < 60) return `${totalSeconds}s`
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (minutes < 60) return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`
  const hours = Math.floor(minutes / 60)
  return `${hours}h ${minutes % 60}m`
}

function timeValue(value?: string) {
  if (!value) return 0
  const time = Date.parse(value)
  return Number.isFinite(time) ? time : 0
}
