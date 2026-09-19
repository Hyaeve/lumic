export function postQueryKey(query) {
  return JSON.stringify(Object.entries(query).filter(([, value]) => value !== '').sort(([a], [b]) => a.localeCompare(b)))
}

// Pages are retained by query so returning to a timeline reuses its exact order.
// Loading one page never starts a background loop over the rest of the library.
export function createPostPager({ fetchPage, onPage, onChange = () => {} }) {
  const entries = new Map()
  let generation = 0
  function entry(query) {
    const key = postQueryKey(query)
    if (!entries.has(key)) entries.set(key, { query: { ...query }, ids: [], total: 0, headerMedia: [], cursor: '', hasMore: true, loaded: false, loading: false, error: '', promise: null, version: 0 })
    return entries.get(key)
  }
  async function next(query, retry = false) {
    const page = entry(query)
    if (page.loading) return page.promise
    if ((page.loaded && !page.hasMore) || (page.error && !retry)) return page
    const requestGeneration = generation
    const requestVersion = page.version
    const current = () => requestGeneration === generation && requestVersion === page.version
    const first = !page.loaded
    page.loading = true
    page.error = ''
    onChange()
    page.promise = (async () => {
      try {
        const result = await fetchPage(page.query, first ? '' : page.cursor)
        if (!current()) return page
        if (!Array.isArray(result.items)) throw new Error('动态数据格式不正确')
        page.ids = [...new Set([...(first ? [] : page.ids), ...result.items.map(item => String(item.id))])]
        page.total = result.total ?? page.ids.length
        if (first) {
          const gallery = result.headerMedia || []
          const start = Math.floor(Math.random() * gallery.length)
          page.headerMedia = [...gallery.slice(start), ...gallery.slice(0, start)]
        }
        page.cursor = result.nextCursor || ''
        page.hasMore = Boolean(result.hasMore && page.cursor)
        page.loaded = true
        onPage(result)
      } catch (error) {
        if (current()) page.error = error.message || '动态加载失败'
      } finally {
        if (current()) {
          page.loading = false
          page.promise = null
          onChange()
        }
      }
      return page
    })()
    return page.promise
  }
  function ensure(query) {
    const page = entry(query)
    return page.loaded ? Promise.resolve(page) : next(query)
  }
  function invalidate(predicate = () => true) {
    for (const page of entries.values()) {
      if (!predicate(page.query)) continue
      page.version++
      page.loaded = false
      page.loading = false
      page.error = ''
      page.promise = null
      page.cursor = ''
      page.hasMore = true
    }
    onChange()
  }
  function clear() {
    generation++
    entries.clear()
    onChange()
  }
  return { entry, ensure, next, invalidate, clear }
}
