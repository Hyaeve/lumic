import test from 'node:test'
import assert from 'node:assert/strict'
import { createPostPager } from './postPager.js'

test('retains scoped statistics and gallery across pagination and cached returns', async () => {
  const pager = createPostPager({
    fetchPage: async (_, cursor) => cursor
      ? { items: [{ id: 'second' }], total: 2, hasMore: false }
      : { items: [{ id: 'first' }], total: 2, hasMore: true, nextCursor: 'next', headerMedia: ['/flow/a.jpg'], headerPostIds: { '/flow/a.jpg': 'second' }, scopeStats: { total: 2, today: 1, favorites: 2 } },
    onPage: () => {}
  })
  const query = { liked: 'true' }
  await pager.ensure(query)
  await pager.next(query)
  await pager.ensure(query)
  assert.deepEqual(pager.entry(query).scopeStats, { total: 2, today: 1, favorites: 2 })
  assert.deepEqual(pager.entry(query).headerMedia, ['/flow/a.jpg'])
  assert.deepEqual(pager.entry(query).headerPostIds, { '/flow/a.jpg': 'second' })
})

test('loads only requested pages and retains the origin order across navigation', async () => {
  const requests = []
  const pager = createPostPager({
    fetchPage: async (query, cursor) => {
      requests.push({ query, cursor })
      return { items: [{ id: `${query.seed || query.author}-${cursor || 'first'}` }], total: 20, nextCursor: cursor ? '' : 'next', hasMore: !cursor }
    },
    onPage: () => {}
  })
  const all = { order: 'random', seed: 'one' }
  await pager.ensure(all)
  assert.equal(requests.length, 1)
  await pager.next(all)
  assert.equal(requests.length, 2)
  const origin = [...pager.entry(all).ids]
  await pager.ensure({ order: 'newest', author: 'Alice' })
  await pager.ensure(all)
  assert.equal(requests.length, 3)
  assert.deepEqual(pager.entry(all).ids, origin)
  await pager.ensure({ order: 'random', seed: 'two' })
  assert.equal(requests.length, 4)
})

test('coalesces requests, ignores invalidated responses and allows explicit retry', async () => {
  let resolve
  let calls = 0
  const pager = createPostPager({
    fetchPage: () => { calls++; return new Promise(done => { resolve = done }) },
    onPage: () => {}
  })
  const query = { order: 'random', seed: 'one' }
  const first = pager.ensure(query)
  const duplicate = pager.ensure(query)
  assert.equal(calls, 1)
  pager.invalidate()
  resolve({ items: [{ id: 'stale' }], total: 1 })
  await Promise.all([first, duplicate])
  assert.deepEqual(pager.entry(query).ids, [])
  const fresh = pager.ensure(query)
  resolve({ items: [{ id: 'fresh' }], total: 1 })
  await fresh
  assert.deepEqual(pager.entry(query).ids, ['fresh'])

  let attempts = 0
  const failures = createPostPager({ fetchPage: async () => { attempts++; throw new Error('offline') }, onPage: () => {} })
  await failures.next(query)
  await failures.next(query)
  assert.equal(attempts, 1)
  await failures.next(query, true)
  assert.equal(attempts, 2)
})
