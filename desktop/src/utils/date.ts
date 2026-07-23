/** Format memory timestamp for settings list (zh-CN, compact). */
export function formatMemoryTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''

  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`

  if (d.toDateString() === now.toDateString()) return `今天 ${time}`

  const yesterday = new Date(now)
  yesterday.setDate(yesterday.getDate() - 1)
  if (d.toDateString() === yesterday.toDateString()) return `昨天 ${time}`

  if (d.getFullYear() === now.getFullYear()) {
    return `${d.getMonth() + 1}月${d.getDate()}日 ${time}`
  }

  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()} ${time}`
}
