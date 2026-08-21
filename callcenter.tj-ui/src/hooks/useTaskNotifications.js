import { useCallback, useEffect, useState } from 'react'
import { tasks as tasksApi } from 'src/api'

const POLL_MS = 20000

// Polls task-status-change notifications for the current user (only ever
// non-empty for SuperAdmin/TenantAdmin — they're the only role that creates
// tasks and therefore the only recipient of task_notifications rows).
export default function useTaskNotifications(enabled) {
  const [items, setItems] = useState([])

  const load = useCallback(() => {
    if (!enabled) return
    tasksApi.notifications().then((d) => setItems(d.notifications || [])).catch(() => {})
  }, [enabled])

  useEffect(() => {
    if (!enabled) return undefined
    load()
    const timer = setInterval(load, POLL_MS)
    return () => clearInterval(timer)
  }, [enabled, load])

  const unreadCount = items.filter((n) => !n.isRead).length

  const markRead = async (id) => {
    await tasksApi.markNotificationRead(id)
    load()
  }
  const markAllRead = async () => {
    await tasksApi.markAllNotificationsRead()
    load()
  }

  return { items, unreadCount, markRead, markAllRead }
}
