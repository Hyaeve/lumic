import test from 'node:test'
import assert from 'node:assert/strict'
import { createNotificationCenter } from './notifications.js'

function center() {
  const jobs = new Map()
  let sequence = 0
  const notices = createNotificationCenter({
    schedule: (callback, duration) => { const id = ++sequence; jobs.set(id, { callback, duration }); return id },
    cancel: id => jobs.delete(id)
  })
  return { ...notices, jobs }
}

test('notifications deduplicate, cap the stack and cancel removed timers', () => {
  const n = center()
  assert.equal(n.notify(''), undefined)
  const first = n.notify('保存成功')
  assert.equal(n.notify('保存成功'), first)
  assert.equal(n.items.value.length, 1)
  assert.equal(n.jobs.size, 1)
  n.notify('删除成功'); n.notify('测试成功'); n.notify('保存失败', 'error')
  assert.equal(n.items.value.length, 3)
  assert.equal(n.items.value.some(item => item.id === first), false)
  assert.equal(n.jobs.size, 3)
  n.clear()
  assert.equal(n.items.value.length, 0)
  assert.equal(n.jobs.size, 0)
})

test('success and error messages expire and hovering/focus pause independently', () => {
  const n = center()
  const id = n.notify('已保存')
  assert.equal([...n.jobs.values()][0].duration, 4000)
  n.pause(id, 'hover', true); n.pause(id, 'focus', true)
  assert.equal(n.jobs.size, 0)
  n.pause(id, 'hover', false)
  assert.equal(n.jobs.size, 0)
  n.pause(id, 'focus', false)
  assert.equal(n.jobs.size, 1)
  ;[...n.jobs.values()][0].callback()
  assert.equal(n.items.value.length, 0)
  const error = n.notify('失败请重试', 'error')
  assert.equal([...n.jobs.values()][0].duration, 6000)
  n.dismiss(error)
  assert.equal(n.jobs.size, 0)
})
