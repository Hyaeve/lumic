import { readonly, ref } from 'vue'

export function createNotificationCenter({ schedule = setTimeout, cancel = clearTimeout } = {}) {
  const items = ref([])
  const timers = new Map()
  const paused = new Map()
  let sequence = 0
  function dismiss(id) {
    cancel(timers.get(id))
    timers.delete(id)
    paused.delete(id)
    items.value = items.value.filter(item => item.id !== id)
  }
  function restart(item) {
    cancel(timers.get(item.id))
    timers.delete(item.id)
    if (!paused.get(item.id)?.size) timers.set(item.id, schedule(() => dismiss(item.id), item.duration))
  }
  function notify(message, tone = 'success') {
    const text = String(message || '').trim()
    if (!text) return
    const existing = items.value.find(item => item.message === text && item.tone === tone)
    if (existing) { restart(existing); return existing.id }
    const item = { id: ++sequence, message: text, tone, duration: tone === 'error' ? 6000 : 4000 }
    if (items.value.length >= 3) dismiss(items.value[0].id)
    items.value = [...items.value, item]
    restart(item)
    return item.id
  }
  function pause(id, reason, active) {
    const item = items.value.find(item => item.id === id)
    if (!item) return
    const reasons = paused.get(id) || new Set()
    if (active) reasons.add(reason)
    else reasons.delete(reason)
    paused.set(id, reasons)
    restart(item)
  }
  function clear() { items.value.forEach(item => dismiss(item.id)) }
  return { items: readonly(items), notify, dismiss, pause, clear }
}

export const notifications = createNotificationCenter()
export const notify = notifications.notify
