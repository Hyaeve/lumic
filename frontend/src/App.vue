<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import bilibiliIcon from '../icon/bilibili.png'
import bilibiliLineIcon from '../icon/bilibili-1.png'
import pixivIcon from '../icon/pixiv.png'
import pixivLineIcon from '../icon/pixiv-1.png'
import weiboIcon from '../icon/weibo.png'
import weiboLineIcon from '../icon/weibo-1.png'
import twitterIcon from '../icon/推特.png'
import twitterLineIcon from '../icon/推特-1.png'
import timelineNavIcon from '../icon/动态.png'
import favoriteNavIcon from '../icon/收藏.png'
import subscriptionsNavIcon from '../icon/订阅平台.png'
import settingsNavIcon from '../icon/设置.png'
import dayThemeIcon from '../icon/日间模式.png'
import nightThemeIcon from '../icon/夜间模式.png'
import newestSortIcon from '../icon/时间排序－正序.png'
import oldestSortIcon from '../icon/时间排序－倒序.png'
import listViewIcon from '../icon/列表.png'
import masonryViewIcon from '../icon/瀑布流.png'
import originalSizeIcon from '../icon/原始尺寸.png'
import rotateLightboxIcon from '../icon/旋转.png'
import downloadLightboxIcon from '../icon/下载.png'
import visitPostIcon from '../icon/访问.png'

const authenticated = ref(false)
const sessionChecked = ref(false)
const loginError = ref('')
const showSettings = ref(false)
const settingsTab = ref('settings')
const settingsError = ref('')
const settingsBusy = ref(false)
const settingsForm = ref({ username: '', currentPassword: '', newPassword: '' })
const proxyForm = ref({ proxyUrl: '' })
const proxyStatus = ref({ proxyEnabled: false, proxyUrl: '' })
const proxyMessage = ref('')
const selectedFeed = ref(null)
const cronEditing = ref(false)
const showFeedSettings = ref(false)
const selectedPlatform = ref(null)
const credentialPlatform = ref(null)
const sourceActionBusy = ref('')
const sourceActionMessage = ref('')
const confirmDialog = ref({ open: false, title: '', message: '', confirmText: '确认', cancelText: '取消', tone: 'danger' })
let confirmResolver = null
const loginBusy = ref(false)
const credentials = ref({ username: '', password: '' })
const activeNav = ref('all')
const activeSource = ref('all')
const sourcesExpanded = ref(true)
const mobileMenuOpen = ref(false)
const showBrandMenu = ref(false)
const phonePortrait = ref(false)
const selectedAuthor = ref(null)
const selectedTag = ref('')
const authorReturnState = ref({ nav: 'all', source: 'all' })
const isDark = ref(false)
const showAdd = ref(false)
const showBilibili = ref(false)
const showWeibo = ref(false)
const showPixiv = ref(false)
const weiboKeyword = ref('')
const weiboResults = ref([])
const weiboSubscriptionTags = ref('')
const biliAccount = ref({ configured: false, userId: '', userName: '', avatar: '' })
const biliCredentials = ref({ cookie: '', SESSDATA: '', bili_jct: '', buvid3: '', DedeUserID: '', ac_time_value: '', buvid4: '', DedeUserID__ckMd5: '' })
const biliKeyword = ref('')
const biliResults = ref([])
const biliBusy = ref(false)
const biliError = ref('')
const biliSubscriptionTags = ref('')
const biliQR = ref(null)
const biliQRImage = ref('')
const biliQRStatus = ref('')
let biliPollTimer = null
const pixivAccount = ref({ configured: false, userId: '', userName: '', avatar: '' })
const emptyPixivCredentials = () => ({ userAgent: '', baggage: '', cookie: '', userId: '', sentryTrace: '', csrfToken: '' })
const pixivCredentials = ref(emptyPixivCredentials())
const pixivBusy = ref(false)
const pixivError = ref('')
const pixivArtistId = ref('')
const pixivSubscriptionTags = ref('')
const weiboAccount = ref({ configured: false, userId: '', userName: '', avatar: '' })
const weiboCredentials = ref({ cookie: '', userId: '' })
const weiboPasswordCredentials = ref({ username: '', password: '' })
const weiboQR = ref(null)
const weiboBusy = ref(false)
const weiboError = ref('')
let weiboPollTimer = null
const syncing = ref(false)
const posts = ref([])
const feeds = ref([])
const postActionBusy = ref('')
const timelineMessage = ref('')
const selectionMode = ref(false)
const selectionAction = ref('delete')
const selectedPostIds = ref([])
const contextMenu = ref({ open: false, x: 0, y: 0, post: null })
const timelineSort = ref('newest')
const timelineView = ref('list')
const timelineSearch = ref('')
const timelineSearchFocused = ref(false)
const mediaShapes = ref({})
const mediaRatios = ref({})
const videoRatios = ref({})
const timelineStart = ref(0)
const timelineEnd = ref(15)
const timelineHeights = ref({})
const masonryHeights = ref({})
const masonryColumnCount = ref(3)
const masonryColumnWidth = ref(260)
const masonryGap = ref(14)
const masonryViewportTop = ref(0)
const masonryViewportBottom = ref(900)
const masonryDetailPost = ref(null)
const mobileDetailIndex = ref(0)
const mobileDetailMotion = ref('enter')
const mobilePostReturnPath = ref('/')
const pendingPostId = ref('')
const feedListElement = ref(null)
const estimatedPostHeight = 560
const timelineOverscan = 5
let phonePortraitQuery = null
let timelineFrame = 0
let lastTimelineScrollY = 0
let masonryScrollDirection = 'idle'
let postResizeObserver = null
let sessionPollTimer = null
const observedPostElements = new Map()
const animatedMasonryPostIds = new Set()
const transientTimers = new Set()
const lightbox = ref({ open: false, media: [], index: 0, author: '', scale: 1, rotation: 0, fit: true, x: 0, y: 0, dragging: false, motion: 'enter' })
const lightboxImageElement = ref(null)
const lightboxScalePercent = ref(100)
const lightboxAtOriginalSize = ref(false)
const lightboxDockVisible = ref(true)
const mobileLightboxMenu = ref({ open: false, x: 0, y: 0 })
const meteorBurst = ref([])
const lightboxPointers = new Map()
let lightboxGesture = null
let lightboxGestureHadPinch = false
let lightboxScaleFrame = 0
let lightboxLastTap = { time: 0, x: 0, y: 0 }
let lightboxDockTimer = 0
let lightboxLongPressTimer = 0
let meteorBurstTimer = 0
let meteorCleanupTimer = 0
let meteorBurstSequence = 0
let mobileDetailTouch = null
let mobileDetailSwipeClickBlocked = false

const sourceMeta = {
  bilibili: { label: '哔哩哔哩', icon: 'bl', image: bilibiliIcon, lineImage: bilibiliLineIcon, color: 'blue' },
  weibo: { label: '微博', icon: 'wb', image: weiboIcon, lineImage: weiboLineIcon, color: 'coral' },
  pixiv: { label: 'Pixiv', icon: 'px', image: pixivIcon, lineImage: pixivLineIcon, color: 'violet' },
  twitter: { label: '推特', icon: 'tw', image: twitterIcon, lineImage: twitterLineIcon, nightImage: twitterLineIcon, color: 'twitter' }
}
const validSources = new Set(Object.keys(sourceMeta))
function sourceIconFor(source) {
  const meta = sourceMeta[source]
  return isDark.value && meta?.nightImage ? meta.nightImage : meta?.image
}
const statsPosts = computed(() => {
  const allPosts = Array.isArray(posts.value) ? posts.value : []
  return activeSource.value === 'all' ? allPosts : allPosts.filter(post => post.source === activeSource.value)
})
const totalStatsCount = computed(() => statsPosts.value.length)
const todayStatsCount = computed(() => {
  const today = new Date()
  return statsPosts.value.filter(post => {
    const published = new Date(post.published)
    return !Number.isNaN(published.getTime()) && published.getFullYear() === today.getFullYear() && published.getMonth() === today.getMonth() && published.getDate() === today.getDate()
  }).length
})
const favoriteStatsCount = computed(() => statsPosts.value.filter(post => post.liked).length)
const selectedPostCount = computed(() => selectedPostIds.value.length)
const mobileDetailMedia = computed(() => postDetailMedia(masonryDetailPost.value))
const mobileDetailCurrentMedia = computed(() => mobileDetailMedia.value[mobileDetailIndex.value] || null)
const filteredPosts = computed(() => {
  const allPosts = Array.isArray(posts.value) ? posts.value : []
  const timeline = activeNav.value === 'liked' ? allPosts.filter(post => post.liked) : allPosts
  const sourceTimeline = activeSource.value === 'all' ? timeline : timeline.filter(post => post.source === activeSource.value)
  let result = sourceTimeline
  if (selectedTag.value) result = result.filter(post => post.tags?.includes(selectedTag.value))
  else if (selectedAuthor.value) result = result.filter(post => post.source === selectedAuthor.value.source && post.author === selectedAuthor.value.name)
  const keywords = timelineSearch.value.trim().normalize('NFKC').toLocaleLowerCase().split(/\s+/).filter(Boolean)
  if (keywords.length) result = result.filter(post => {
    const tags = Array.isArray(post.tags) ? post.tags : []
    const searchable = [post.caption, post.author, ...tags, ...tags.map(tag => `#${tag}`)].map(value => String(value || '').normalize('NFKC').toLocaleLowerCase())
    return keywords.every(keyword => searchable.some(value => value.includes(keyword)))
  })
  return [...result].sort((left, right) => {
    const difference = new Date(right.published).getTime() - new Date(left.published).getTime()
    return timelineSort.value === 'newest' ? difference : -difference
  })
})
const effectiveTimelineView = computed(() => timelineView.value)
const isMasonryView = computed(() => effectiveTimelineView.value === 'masonry')
const visiblePosts = computed(() => filteredPosts.value.slice(timelineStart.value, timelineEnd.value))
function estimateMasonryPostHeight(post) {
  const width = masonryColumnWidth.value
  const inset = phonePortrait.value ? 9 : 12
  let height = inset * 2 + 40
  const firstMedia = post.media?.[0]
  const firstVideo = primaryVideo(post)
  const firstPoster = firstVideo?.poster
  if (firstMedia || firstVideo) {
    const ratioKey = `${post.id}:${firstMedia ? 0 : 'video'}`
    const ratio = mediaRatios.value[ratioKey] || videoRatios.value[post.id] || (firstMedia ? (mediaShapes.value[`${post.id}:0`] === 'portrait' ? 0.74 : mediaShapes.value[`${post.id}:0`] === 'landscape' ? 1.42 : 1) : 16 / 9)
    height += Math.min(width * 1.75, Math.max(width * 0.56, width / ratio)) + 9
  }
  if (post.caption) {
    const charactersPerLine = Math.max(12, Math.floor((width - inset * 2) / (phonePortrait.value ? 13 : 13.5)))
    const lines = Math.min(firstMedia || firstPoster ? 3 : 7, Math.max(1, Math.ceil([...post.caption].length / charactersPerLine)))
    height += lines * (phonePortrait.value ? 20 : 21) + 8
  }
  if (post.tags?.length) height += 24
  return Math.ceil(height)
}
const masonryLayout = computed(() => {
  const count = Math.max(1, masonryColumnCount.value)
  const heights = Array.from({ length: count }, () => 0)
  const items = filteredPosts.value.map((post, index) => {
    let column = 0
    for (let candidate = 1; candidate < count; candidate++) {
      if (heights[candidate] < heights[column]) column = candidate
    }
    const height = masonryHeights.value[post.id] || estimateMasonryPostHeight(post)
    const item = { post, index, column, x: column * (masonryColumnWidth.value + masonryGap.value), y: heights[column], height }
    heights[column] += height + masonryGap.value
    return item
  })
  return { items, height: Math.max(0, ...heights) - (items.length ? masonryGap.value : 0) }
})
const visibleMasonryItems = computed(() => {
  const overscan = phonePortrait.value ? 700 : 1000
  const top = Math.max(0, masonryViewportTop.value - overscan)
  const bottom = masonryViewportBottom.value + overscan
  return masonryLayout.value.items.filter(item => item.y + item.height >= top && item.y <= bottom)
})
const masonryFeedStyle = computed(() => isMasonryView.value && filteredPosts.value.length ? { height: `${Math.ceil(masonryLayout.value.height)}px` } : undefined)
const allFilteredPostsSelected = computed(() => filteredPosts.value.length > 0 && filteredPosts.value.every(post => selectedPostIds.value.includes(post.id)))
const timelineTopSpace = computed(() => filteredPosts.value.slice(0, timelineStart.value).reduce((height, post) => height + (timelineHeights.value[post.id] || estimatedPostHeight) + 15, 0))
const timelineBottomSpace = computed(() => filteredPosts.value.slice(timelineEnd.value).reduce((height, post) => height + (timelineHeights.value[post.id] || estimatedPostHeight) + 15, 0))
const authorProfile = computed(() => {
  if (!selectedAuthor.value) return null
  const authorPosts = posts.value.filter(post => post.source === selectedAuthor.value.source && post.author === selectedAuthor.value.name)
  const latest = authorPosts[0]
  return { ...selectedAuthor.value, avatar: latest?.avatar || selectedAuthor.value.avatar, count: authorPosts.length }
})
const platformCards = computed(() => [
  { key: 'bilibili', label: '哔哩哔哩', short: '哔', ...sourceMeta.bilibili, configured: biliAccount.value.configured, account: biliAccount.value.configured ? (biliAccount.value.userName || `UID ${biliAccount.value.userId}`) : '尚未连接', avatar: biliAccount.value.avatar, path: '/flow/bilibili', description: 'UP 主动态、专栏与账号收藏', feeds: feeds.value.filter(feed => feed.source === 'bilibili') },
  { key: 'weibo', label: '微博', short: '微', ...sourceMeta.weibo, configured: weiboAccount.value.configured, account: weiboAccount.value.configured ? (weiboAccount.value.userName || `UID ${weiboAccount.value.userId}`) : '尚未连接', avatar: weiboAccount.value.avatar, path: '/flow/weibo', description: '博主动态与图文媒体', feeds: feeds.value.filter(feed => feed.source === 'weibo') },
  { key: 'pixiv', label: 'Pixiv', short: 'P', ...sourceMeta.pixiv, configured: pixivAccount.value.configured, account: pixivAccount.value.configured ? (pixivAccount.value.userName || `UID ${pixivAccount.value.userId}`) : '尚未连接', avatar: pixivAccount.value.avatar, path: '/flow/pixiv', description: '画师作品与插画媒体', feeds: feeds.value.filter(feed => feed.source === 'pixiv') },
  { key: 'twitter', label: '推特', short: '推', ...sourceMeta.twitter, image: sourceIconFor('twitter'), configured: false, account: '暂未开放', avatar: '', path: '/flow/twitter', description: '推文、图片与媒体动态', feeds: feeds.value.filter(feed => feed.source === 'twitter') }
])
const hasWeiboLikesSource = computed(() => feeds.value.some(feed => feed.id?.startsWith('weibo-likes-')))
const hasPixivBookmarksSource = computed(() => feeds.value.some(feed => feed.id?.startsWith('pixiv-bookmarks-')))
const hasBilibiliFavoriteOpusSource = computed(() => feeds.value.some(feed => feed.id?.startsWith('bili-opus-favorites-')))
function isAccountCollectionFeed(feed) {
  return feed?.id?.startsWith('weibo-likes-') || feed?.id?.startsWith('pixiv-bookmarks-') || feed?.id?.startsWith('bili-opus-favorites-')
}
function sourceAvatarCandidates(feed, platform) {
  const candidates = []
  if (isAccountCollectionFeed(feed)) {
    if (feed?.avatar?.startsWith('/flow/')) candidates.push(feed.avatar, platform?.avatar)
    else candidates.push(platform?.avatar, feed?.avatar)
  } else {
    candidates.push(feed?.avatar)
  }
  candidates.push(platform?.image)
  return [...new Set(candidates.filter(Boolean))]
}
function sourceAvatar(feed, platform) {
  return sourceAvatarCandidates(feed, platform)[0] || ''
}
function handleSourceAvatarError(event, feed, platform) {
  const image = event.currentTarget
  const candidates = sourceAvatarCandidates(feed, platform)
  const nextIndex = Number(image.dataset.fallbackIndex || 0) + 1
  image.dataset.fallbackIndex = String(nextIndex)
  if (candidates[nextIndex]) image.src = candidates[nextIndex]
}
function platformCardForSource(source) {
  return platformCards.value.find(platform => platform.key === source)
}
const localGreeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 6) return '夜深了'
  if (hour < 11) return '早上好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})
const localSeason = computed(() => {
  const month = new Date().getMonth() + 1
  if (month >= 3 && month <= 5) return 'spring'
  if (month >= 6 && month <= 8) return 'summer'
  if (month >= 9 && month <= 11) return 'autumn'
  return 'winter'
})

async function responseError(response, fallback) {
  try { return (await response.json()).error || fallback } catch { return fallback }
}
function relativeTime(date) {
  const minutes = Math.max(1, Math.floor((Date.now() - new Date(date)) / 60000))
  if (minutes < 60) return `${minutes} 分钟前`
  if (minutes < 1440) return `${Math.floor(minutes / 60)} 小时前`
  return `${Math.floor(minutes / 1440)} 天前`
}
async function login() {
  loginError.value = ''
  loginBusy.value = true
  try {
    const response = await fetch('/api/login', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(credentials.value) })
    if (!response.ok) throw new Error('账号或密码不正确')
    authenticated.value = true
    await loadData()
    await loadPlatformAccounts()
  } catch (error) { loginError.value = error.message }
  finally { loginBusy.value = false }
}
async function logout() {
  try {
    await fetch('/api/logout', { method: 'POST', credentials: 'same-origin' })
  } finally {
    showBrandMenu.value = false
    authenticated.value = false
    posts.value = []
    feeds.value = []
    selectedPlatform.value = null
    credentialPlatform.value = null
    stopSelection()
  }
}
async function loadData() {
  try {
    const postResponse = await fetch('/api/posts', { cache: 'no-store' })
    if (!postResponse.ok) throw new Error('api unavailable')
    const payload = await postResponse.json()
    posts.value = Array.isArray(payload) ? payload : []
    resolveRoutedPost()
  } catch { posts.value = [] }
  await loadFeeds()
}
async function loadFeeds(fallbackFeed = null) {
  const endpoints = ['/api/feeds', '/api/bilibili/subscriptions', '/api/weibo/subscriptions', '/api/pixiv/subscriptions']
  const responses = await Promise.allSettled(endpoints.map(endpoint => fetch(endpoint, { cache: 'no-store' })))
  const loaded = []
  for (const result of responses) {
    if (result.status !== 'fulfilled' || !result.value.ok) continue
    try {
      const items = await result.value.json()
      if (Array.isArray(items)) loaded.push(...items)
    } catch {}
  }
  if (fallbackFeed) loaded.push(fallbackFeed)
  if (!loaded.length && responses.every(result => result.status === 'rejected' || !result.value.ok)) return false
  const unique = new Map()
  for (const feed of loaded) {
    if (feed?.id) unique.set(feed.id, feed)
  }
  feeds.value = [...unique.values()]
  return true
}
async function loadPlatformAccounts() {
  const [biliResponse, weiboResponse, pixivResponse] = await Promise.allSettled([
    fetch('/api/bilibili/account', { cache: 'no-store' }),
    fetch('/api/weibo/account', { cache: 'no-store' }),
    fetch('/api/pixiv/account', { cache: 'no-store' })
  ])
  if (biliResponse.status === 'fulfilled' && biliResponse.value.ok) biliAccount.value = await biliResponse.value.json()
  if (weiboResponse.status === 'fulfilled' && weiboResponse.value.ok) weiboAccount.value = await weiboResponse.value.json()
  if (pixivResponse.status === 'fulfilled' && pixivResponse.value.ok) pixivAccount.value = await pixivResponse.value.json()
}
function setDarkMode(value) {
  isDark.value = Boolean(value)
  localStorage.setItem('lumic-theme', isDark.value ? 'dark' : 'light')
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', isDark.value ? '#080a0e' : '#fbf7ea')
}
function stopNightMeteorLoop() {
  if (meteorBurstTimer) window.clearTimeout(meteorBurstTimer)
  if (meteorCleanupTimer) window.clearTimeout(meteorCleanupTimer)
  meteorBurstTimer = 0
  meteorCleanupTimer = 0
  meteorBurst.value = []
}
function scheduleNightMeteorBurst(delay = 22000 + Math.random() * 16000) {
  if (meteorBurstTimer) window.clearTimeout(meteorBurstTimer)
  if (!isDark.value) return
  meteorBurstTimer = window.setTimeout(triggerNightMeteorBurst, delay)
}
function triggerNightMeteorBurst() {
  if (!isDark.value) {
    stopNightMeteorLoop()
    return
  }
  const count = 1 + Math.floor(Math.random() * 5)
  const viewportHeight = Math.max(window.innerHeight, 560)
  let maximumDuration = 0
  meteorBurst.value = Array.from({ length: count }, (_, index) => ({
    id: `${Date.now()}-${meteorBurstSequence++}`,
    style: (() => {
      const top = -90 + Math.random() * 72
      const travelY = viewportHeight + 180 + Math.random() * 120
      const angle = 37 + Math.random() * 3
      const travelX = travelY / Math.tan(angle * Math.PI / 180)
      const delay = (count >= 3 ? index * 0.62 : index * 0.32) + Math.random() * 0.34
      const duration = 2.8 + Math.random() * 1.5
      maximumDuration = Math.max(maximumDuration, duration + delay)
      const right = count >= 3
        ? 3 + (index / (count - 1)) * 78 + (Math.random() - 0.5) * 7
        : -4 + Math.random() * 68
      return {
        '--meteor-top': `${top}px`,
        '--meteor-right': `${Math.max(-5, Math.min(84, right))}%`,
        '--meteor-width': `${24 + Math.random() * 22}px`,
        '--meteor-angle': `${(-angle).toFixed(2)}deg`,
        '--meteor-delay': `${delay}s`,
        '--meteor-duration': `${duration}s`,
        '--meteor-x': `${-travelX}px`,
        '--meteor-y': `${travelY}px`
      }
    })()
  }))
  if (meteorCleanupTimer) window.clearTimeout(meteorCleanupTimer)
  meteorCleanupTimer = window.setTimeout(() => {
    meteorBurst.value = []
    meteorCleanupTimer = 0
  }, maximumDuration * 1000 + 500)
  scheduleNightMeteorBurst(22000 + Math.random() * 16000)
}
function startNightMeteorLoop() {
  stopNightMeteorLoop()
  if (isDark.value) scheduleNightMeteorBurst(7000 + Math.random() * 5000)
}
async function syncNow() {
  syncing.value = true
  try { await fetch('/api/sync', { method: 'POST' }) } catch {}
  scheduleTransient(() => { syncing.value = false }, 900)
}
async function openSettings(_section = 'settings', updateHistory = true) {
  mobileMenuOpen.value = false
  showBrandMenu.value = false
  settingsTab.value = 'settings'; showSettings.value = true; activeNav.value = 'settings'; settingsError.value = ''; proxyMessage.value = ''; pixivError.value = ''; weiboError.value = ''
  if (updateHistory) updateRoute('/settings')
  try {
    const [projectResponse, biliResponse, pixivResponse, weiboResponse] = await Promise.all([fetch('/api/project/settings'), fetch('/api/bilibili/account'), fetch('/api/pixiv/account'), fetch('/api/weibo/account')])
    if (projectResponse.ok) proxyStatus.value = await projectResponse.json()
    if (biliResponse.ok) biliAccount.value = await biliResponse.json()
    if (pixivResponse.ok) pixivAccount.value = await pixivResponse.json()
    if (weiboResponse.ok) weiboAccount.value = await weiboResponse.json()
  } catch { settingsError.value = '无法读取项目设置' }
}
async function saveProxy() {
  settingsBusy.value = true; settingsError.value = ''; proxyMessage.value = ''
  try {
    const response = await fetch('/api/project/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(proxyForm.value) })
    if (!response.ok) throw new Error(await responseError(response, '代理保存失败'))
    proxyStatus.value = await response.json(); proxyForm.value.proxyUrl = ''; proxyMessage.value = proxyStatus.value.proxyEnabled ? '代理已保存' : '已关闭项目代理'
  } catch (error) { settingsError.value = error.message } finally { settingsBusy.value = false }
}
async function testProxy() {
  if (!proxyForm.value.proxyUrl) { settingsError.value = '请输入完整代理地址后再测试'; return }
  settingsBusy.value = true; settingsError.value = ''; proxyMessage.value = ''
  try {
    const response = await fetch('/api/project/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(proxyForm.value) })
    if (!response.ok) throw new Error(await responseError(response, '代理测试失败'))
    proxyMessage.value = (await response.json()).message
  } catch (error) { settingsError.value = error.message } finally { settingsBusy.value = false }
}
async function saveSettings() {
  settingsError.value = ''
  settingsBusy.value = true
  try {
    const response = await fetch('/api/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(settingsForm.value) })
    if (!response.ok) throw new Error(response.status === 401 ? '当前密码不正确' : '保存失败，请检查输入')
    showSettings.value = false
    settingsForm.value = { username: '', currentPassword: '', newPassword: '' }
  } catch (error) { settingsError.value = error.message }
  finally { settingsBusy.value = false }
}
async function openBilibili() {
  showAdd.value = false; biliError.value = ''
  try {
    const response = await fetch('/api/bilibili/account')
    if (response.ok) biliAccount.value = await response.json()
    if (!biliAccount.value.configured) { await openSettings('platforms'); return }
    showBilibili.value = true
  } catch { biliError.value = '无法读取 B 站配置' }
}
async function openWeibo() {
  showAdd.value = false; weiboError.value = ''
  try {
    const response = await fetch('/api/weibo/account')
    if (response.ok) weiboAccount.value = await response.json()
    if (!weiboAccount.value.configured) { await openSettings('platforms'); return }
    selectedPlatform.value = null; showWeibo.value = true
  } catch { weiboError.value = '无法读取微博配置' }
}
function stopBilibiliPolling() { if (biliPollTimer) clearTimeout(biliPollTimer); biliPollTimer = null }
async function pollBilibiliQR() {
  if (!biliQR.value?.id) return
  try {
    const response = await fetch(`/api/bilibili/qr?id=${encodeURIComponent(biliQR.value.id)}`)
    if (!response.ok) throw new Error(await responseError(response, 'B 站扫码状态查询失败'))
    const result = await response.json(); biliQRStatus.value = result.message || '等待扫码'
    if (result.status === 'connected') { biliAccount.value = { configured: true, userId: result.userId, userName: result.userName || '', avatar: result.avatar || '' }; biliQR.value = null; biliQRImage.value = ''; biliBusy.value = false; stopBilibiliPolling(); return }
    biliPollTimer = setTimeout(pollBilibiliQR, 3000)
  } catch (error) { biliError.value = error.message; biliBusy.value = false; stopBilibiliPolling() }
}
async function startBilibiliQR() {
  stopBilibiliPolling(); biliBusy.value = true; biliError.value = ''; biliQR.value = null; biliQRImage.value = ''; biliQRStatus.value = '正在生成二维码…'
  try {
    const response = await fetch('/api/bilibili/qr', { method: 'POST' })
    if (!response.ok) throw new Error(await responseError(response, '无法获取 B 站二维码'))
    biliQR.value = await response.json(); biliQRImage.value = await QRCode.toDataURL(biliQR.value.url, { width: 220, margin: 2 }); biliQRStatus.value = '请使用哔哩哔哩手机客户端扫码'; biliPollTimer = setTimeout(pollBilibiliQR, 2000)
  } catch (error) { biliError.value = error.message; biliBusy.value = false }
}
async function saveBilibiliAccount() {
  biliBusy.value = true; biliError.value = ''
  try {
    const response = await fetch('/api/bilibili/account', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(biliCredentials.value) })
    if (!response.ok) throw new Error(await responseError(response, '凭证验证失败'))
    biliAccount.value = await response.json()
    biliCredentials.value = { cookie: '', SESSDATA: '', bili_jct: '', buvid3: '', DedeUserID: '', ac_time_value: '', buvid4: '', DedeUserID__ckMd5: '' }
  } catch (error) { biliError.value = error.message } finally { biliBusy.value = false }
}
async function savePixivAccount() {
  pixivBusy.value = true; pixivError.value = ''
  try {
    const response = await fetch('/api/pixiv/account', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.fromEntries(Object.entries(pixivCredentials.value).map(([key, value]) => [key, value.trim()]))) })
    if (!response.ok) throw new Error(await responseError(response, 'Pixiv 凭证验证失败'))
    pixivAccount.value = await response.json(); pixivCredentials.value = emptyPixivCredentials()
  } catch (error) { pixivError.value = error.message } finally { pixivBusy.value = false }
}
async function openPixiv() {
  showAdd.value = false; pixivError.value = ''
  try {
    const response = await fetch('/api/pixiv/account')
    if (response.ok) pixivAccount.value = await response.json()
    if (!pixivAccount.value.configured) { await openSettings('platforms'); return }
    selectedPlatform.value = null; showPixiv.value = true
  } catch { pixivError.value = '无法读取 Pixiv 配置' }
}
function stopWeiboPolling() { if (weiboPollTimer) clearTimeout(weiboPollTimer); weiboPollTimer = null }
async function pollWeiboQR() {
  if (!weiboQR.value?.id) return
  try {
    const response = await fetch(`/api/weibo/qr?id=${encodeURIComponent(weiboQR.value.id)}`)
    if (!response.ok) throw new Error(await responseError(response, '微博扫码状态查询失败'))
    const result = await response.json()
    if (result.status === 'connected') { weiboAccount.value = { configured: true, userId: result.userId, userName: result.userName, avatar: result.avatar || '' }; weiboQR.value = null; weiboBusy.value = false; return }
    weiboPollTimer = setTimeout(pollWeiboQR, 2000)
  } catch (error) { weiboError.value = error.message; weiboBusy.value = false; stopWeiboPolling() }
}
async function startWeiboQR() {
  stopWeiboPolling(); weiboBusy.value = true; weiboError.value = ''; weiboQR.value = null
  try {
    const response = await fetch('/api/weibo/qr', { method: 'POST' })
    if (!response.ok) throw new Error(await responseError(response, '无法获取微博二维码'))
    weiboQR.value = await response.json(); weiboPollTimer = setTimeout(pollWeiboQR, 1500)
  } catch (error) { weiboError.value = error.message; weiboBusy.value = false }
}
async function saveWeiboAccount() {
  weiboBusy.value = true; weiboError.value = ''
  try {
    const response = await fetch('/api/weibo/account', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ cookie: weiboCredentials.value.cookie.trim(), userId: weiboCredentials.value.userId.trim() }) })
    if (!response.ok) throw new Error(await responseError(response, '微博 Cookie 验证失败'))
    weiboAccount.value = await response.json()
    weiboCredentials.value = { cookie: '', userId: '' }
  } catch (error) { weiboError.value = error.message } finally { weiboBusy.value = false }
}
async function downloadConfigurationBackup() {
  settingsBusy.value = true; settingsError.value = ''; proxyMessage.value = ''
  try {
    const response = await fetch('/api/configuration/backup')
    if (!response.ok) throw new Error(await responseError(response, '配置备份失败'))
    const blob = await response.blob()
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = `lumic-config-${new Date().toISOString().slice(0, 10)}.json`
    link.click()
    URL.revokeObjectURL(link.href)
    proxyMessage.value = '配置备份已下载'
  } catch (error) { settingsError.value = error.message } finally { settingsBusy.value = false }
}
async function restoreConfiguration(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  const confirmed = await askConfirm({ title: '恢复配置', message: '恢复将覆盖当前登录账号、平台凭证、代理和订阅来源。动态记录与图片不会改变，确定继续吗？', confirmText: '恢复配置' })
  if (!confirmed) return
  settingsBusy.value = true; settingsError.value = ''; proxyMessage.value = ''
  try {
    const response = await fetch('/api/configuration/backup', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: await file.text() })
    if (!response.ok) throw new Error(await responseError(response, '配置恢复失败'))
    proxyMessage.value = '配置已恢复，请使用备份中的账号信息重新登录'
    await loadData()
  } catch (error) { settingsError.value = error.message } finally { settingsBusy.value = false }
}
function postDateTime(date) {
  return new Date(date).toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}
function openOriginalPost(post) {
  if (!post.originalUrl) return
  window.open(post.originalUrl, '_blank', 'noopener,noreferrer')
}
function parseTagInput(value) { return [...new Set(String(value || '').split(/\s+/).map(tag => tag.replace(/^#+/, '').trim()).filter(Boolean))] }
function formatTagInput(tags) { return (tags || []).map(tag => `#${tag}`).join(' ') }
function parseKeywordInput(value) { return [...new Set(String(value || '').split(/\s+/).map(keyword => keyword.trim()).filter(Boolean))] }
function formatKeywordInput(values) { return (values || []).join(' ') }
function cronFieldMatches(field, value, min, max) {
  return field.split(',').some(rawPart => {
    let part = rawPart.trim(); let step = 1
    const stepParts = part.split('/')
    if (stepParts.length === 2) { step = Number(stepParts[1]); part = stepParts[0]; if (!Number.isInteger(step) || step < 1) return false }
    let start = min; let end = max
    if (part !== '*') {
      const range = part.split('-').map(Number)
      if (range.some(Number.isNaN)) return false
      if (range.length === 1) start = end = range[0]
      else if (range.length === 2) [start, end] = range
      else return false
    }
    return start >= min && end <= max && start <= end && value >= start && value <= end && (value - start) % step === 0
  })
}
function nextCronExecution(expression) {
  const fields = String(expression || '').trim().split(/\s+/)
  if (fields.length !== 5) return 'Cron 表达式无效，双击修改'
  const next = new Date(); next.setSeconds(0, 0); next.setMinutes(next.getMinutes() + 1)
  for (let minutes = 0; minutes < 366 * 24 * 60; minutes++, next.setMinutes(next.getMinutes() + 1)) {
    if (cronFieldMatches(fields[0], next.getMinutes(), 0, 59) && cronFieldMatches(fields[1], next.getHours(), 0, 23) && cronFieldMatches(fields[2], next.getDate(), 1, 31) && cronFieldMatches(fields[3], next.getMonth() + 1, 1, 12) && cronFieldMatches(fields[4], next.getDay(), 0, 6)) return `下次执行：${next.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'short', hour: '2-digit', minute: '2-digit' })}`
  }
  return '未来一年内没有匹配的执行时间'
}
async function loginWeiboAccount() {
  weiboBusy.value = true; weiboError.value = ''
  try {
    const response = await fetch('/api/weibo/account', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: weiboPasswordCredentials.value.username.trim(), password: weiboPasswordCredentials.value.password }) })
    if (!response.ok) throw new Error(await responseError(response, '微博账号登录失败'))
    weiboAccount.value = await response.json()
    weiboPasswordCredentials.value = { username: '', password: '' }
  } catch (error) { weiboError.value = error.message } finally { weiboBusy.value = false }
}
async function searchBilibili() {
  if (!biliKeyword.value.trim()) return
  biliBusy.value = true; biliError.value = ''; biliResults.value = []
  try {
    const response = await fetch(`/api/bilibili/search?keyword=${encodeURIComponent(biliKeyword.value.trim())}`)
    if (!response.ok) throw new Error(response.status === 412 ? '请先配置 B 站凭证' : '搜索暂时不可用，请稍后重试')
    biliResults.value = await response.json()
  } catch (error) { biliError.value = error.message } finally { biliBusy.value = false }
}
async function searchWeibo() {
  if (!weiboKeyword.value.trim()) return
  weiboBusy.value = true; weiboError.value = ''; weiboResults.value = []
  try {
    const response = await fetch(`/api/weibo/search?keyword=${encodeURIComponent(weiboKeyword.value.trim())}`)
    if (!response.ok) throw new Error(await responseError(response, response.status === 412 ? '请先连接微博账号' : '搜索暂时不可用，请稍后重试'))
    weiboResults.value = await response.json()
  } catch (error) { weiboError.value = error.message } finally { weiboBusy.value = false }
}
async function runFullSync() {
  syncing.value = true; timelineMessage.value = ''
  try {
    const response = await fetch('/api/sync', { method: 'POST' })
    const result = await response.json()
    if (!response.ok && !result.message) throw new Error(await responseError(response, '同步失败'))
    timelineMessage.value = result.message || '拉取任务已完成'
    await loadData()
  } catch (error) { timelineMessage.value = error.message } finally { syncing.value = false }
}
async function subscribeWeibo(user) {
  weiboBusy.value = true; weiboError.value = ''
  try {
    const response = await fetch('/api/weibo/subscriptions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId: user.userId, name: user.name, avatar: user.avatar, schedule: '0 6 * * *', tags: parseTagInput(weiboSubscriptionTags.value) }) })
    if (!response.ok) throw new Error(await responseError(response, response.status === 409 ? '已经订阅该微博博主' : '订阅失败'))
    const saved = await response.json()
    await loadFeeds(saved)
    weiboSubscriptionTags.value = ''; showWeibo.value = false
    navigateTo('pulls')
  } catch (error) { weiboError.value = error.message } finally { weiboBusy.value = false }
}
async function subscribeBilibili(user) {
  biliBusy.value = true; biliError.value = ''
  try {
    const response = await fetch('/api/bilibili/subscriptions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId: user.userId, name: user.name, avatar: user.avatar, schedule: '0 6 * * *', tags: parseTagInput(biliSubscriptionTags.value) }) })
    if (!response.ok) throw new Error(response.status === 409 ? '已经订阅该 UP 主' : '订阅失败')
    const saved = await response.json()
    await loadFeeds(saved)
    biliSubscriptionTags.value = ''; showBilibili.value = false
    navigateTo('pulls')
  } catch (error) { biliError.value = error.message } finally { biliBusy.value = false }
}
function openPlatformSettings(platform) {
  selectedPlatform.value = platform
}
async function subscribePixiv() {
  const userId = pixivArtistId.value.trim()
  if (!/^\d+$/.test(userId)) { pixivError.value = '请输入画师主页中的数字用户 ID'; return }
  pixivBusy.value = true; pixivError.value = ''
  try {
    const response = await fetch('/api/pixiv/subscriptions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId, schedule: '0 6 * * *', tags: parseTagInput(pixivSubscriptionTags.value) }) })
    if (!response.ok) throw new Error(await responseError(response, response.status === 409 ? '已经订阅该 Pixiv 画师' : '订阅失败'))
    const saved = await response.json()
    await loadFeeds(saved)
    pixivArtistId.value = ''; pixivSubscriptionTags.value = ''; showPixiv.value = false
    navigateTo('pulls')
  } catch (error) { pixivError.value = error.message } finally { pixivBusy.value = false }
}
function openCredentialSettings(platformKey) {
  credentialPlatform.value = platformCards.value.find(platform => platform.key === platformKey) || null
  biliError.value = ''
  weiboError.value = ''
  pixivError.value = ''
}
function handleCredentialCardClick(platformKey) {
  if (phonePortrait.value) openCredentialSettings(platformKey)
}
function managePlatformCredentials(platformKey) {
  selectedPlatform.value = null
  openSettings().then(() => openCredentialSettings(platformKey))
}
function openFeedSettings(feed) {
  cronEditing.value = false
  const contentTypes = feed.source === 'bilibili' && (!feed.contentTypes || feed.contentTypes.length === 0) ? ['DRAW', 'ARTICLE'] : [...(feed.contentTypes || [])]
  selectedFeed.value = { ...feed, includeVideos: Boolean(feed.includeVideos), contentTypes, tags: [...(feed.tags || [])], tagInput: formatTagInput(feed.tags), includeKeywordInput: formatKeywordInput(feed.includeKeywords), excludeKeywordInput: formatKeywordInput(feed.excludeKeywords) }
  showFeedSettings.value = true
}
async function saveFeedSettings() {
  if (!selectedFeed.value) return
  settingsBusy.value = true; settingsError.value = ''
  try {
    selectedFeed.value.tags = parseTagInput(selectedFeed.value.tagInput)
    selectedFeed.value.includeKeywords = parseKeywordInput(selectedFeed.value.includeKeywordInput)
    selectedFeed.value.excludeKeywords = parseKeywordInput(selectedFeed.value.excludeKeywordInput)
    if ((selectedFeed.value.source === 'bilibili' && selectedFeed.value.id.startsWith('bili-')) || (selectedFeed.value.source === 'weibo' && selectedFeed.value.id.startsWith('weibo-')) || (selectedFeed.value.source === 'pixiv' && selectedFeed.value.id.startsWith('pixiv-'))) {
      const response = await fetch(sourceOperationEndpoint(selectedFeed.value), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(selectedFeed.value) })
      if (!response.ok) throw new Error(await responseError(response, '来源设置保存失败'))
      const saved = await response.json(); const index = feeds.value.findIndex(feed => feed.id === saved.id)
      if (index >= 0) feeds.value[index] = saved
      await loadData()
      if (selectedPlatform.value) {
        const platformFeedIndex = selectedPlatform.value.feeds.findIndex(feed => feed.id === saved.id)
        if (platformFeedIndex >= 0) selectedPlatform.value.feeds[platformFeedIndex] = saved
      }
    } else {
      const index = feeds.value.findIndex(feed => feed.id === selectedFeed.value.id)
      if (index >= 0) feeds.value[index] = { ...selectedFeed.value }
    }
    showFeedSettings.value = false
    cronEditing.value = false
  } catch (error) { settingsError.value = error.message } finally { settingsBusy.value = false }
}
function sourceOperationEndpoint(feed) {
  if (feed.source === 'bilibili' && feed.id.startsWith('bili-')) return '/api/bilibili/subscriptions'
  if (feed.source === 'weibo' && feed.id.startsWith('weibo-')) return '/api/weibo/subscriptions'
  if (feed.source === 'pixiv' && feed.id.startsWith('pixiv-')) return '/api/pixiv/subscriptions'
  return '/api/feeds'
}
async function toggleFeedEnabled(feed) {
  if (!feed || sourceActionBusy.value) return
  const enabled = !feed.enabled
  sourceActionBusy.value = `toggle:${feed.id}`
  sourceActionMessage.value = ''
  settingsError.value = ''
  try {
    const response = await fetch(sourceOperationEndpoint(feed), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...feed, enabled }) })
    if (!response.ok) throw new Error(await responseError(response, enabled ? '启用自动同步失败' : '停用自动同步失败'))
    const saved = await response.json()
    const index = feeds.value.findIndex(item => item.id === saved.id)
    if (index >= 0) feeds.value[index] = saved
    if (selectedPlatform.value) {
      const platformIndex = selectedPlatform.value.feeds.findIndex(item => item.id === saved.id)
      if (platformIndex >= 0) selectedPlatform.value.feeds[platformIndex] = saved
    }
    if (selectedFeed.value?.id === saved.id) selectedFeed.value = { ...selectedFeed.value, ...saved }
    sourceActionMessage.value = saved.enabled ? `已启用“${saved.name}”自动同步` : `已停用“${saved.name}”自动同步`
  } catch (error) {
    settingsError.value = error.message
  } finally {
    sourceActionBusy.value = ''
  }
}
async function syncSource(feed, full = false) {
  const action = full ? 'resync' : 'sync'
  sourceActionBusy.value = `${action}:${feed.id}`; sourceActionMessage.value = ''; settingsError.value = ''
  const endpoint = sourceOperationEndpoint(feed)
  try {
    const response = await fetch(`${endpoint}?action=${action}&id=${encodeURIComponent(feed.id)}`, { method: 'POST' })
    const result = await response.json()
    if (!response.ok && result.status !== 'failed') throw new Error(result.error || '无法启动来源拉取')
    sourceActionMessage.value = result.message || '拉取任务已完成'
    const updated = result.source; const index = feeds.value.findIndex(item => item.id === feed.id)
    if (updated && index >= 0) feeds.value[index] = updated
    if (updated && selectedPlatform.value) {
      const platformIndex = selectedPlatform.value.feeds.findIndex(item => item.id === feed.id)
      if (platformIndex >= 0) selectedPlatform.value.feeds[platformIndex] = updated
    }
  } catch (error) { settingsError.value = error.message } finally { sourceActionBusy.value = '' }
}
function closeConfirmDialog(result = false) {
  confirmDialog.value.open = false
  if (confirmResolver) {
    const resolve = confirmResolver
    confirmResolver = null
    resolve(result)
  }
}
function askConfirm({ title, message, confirmText = '确认', cancelText = '取消', tone = 'danger' }) {
  if (confirmResolver) closeConfirmDialog(false)
  confirmDialog.value = { open: true, title, message, confirmText, cancelText, tone }
  return new Promise(resolve => { confirmResolver = resolve })
}
function resetLightboxView() {
  lightbox.value.scale = 1
  lightbox.value.rotation = 0
  lightbox.value.fit = true
  lightbox.value.x = 0
  lightbox.value.y = 0
  lightbox.value.dragging = false
  lightboxAtOriginalSize.value = false
  lightboxPointers.clear()
  lightboxGesture = null
  lightboxGestureHadPinch = false
  lightboxLastTap = { time: 0, x: 0, y: 0 }
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  clearLightboxLongPress()
  scheduleLightboxScaleUpdate()
}
function openLightbox(post, index) {
  lightbox.value = { open: true, media: post.media || [], index, author: post.author, scale: 1, rotation: 0, fit: true, x: 0, y: 0, dragging: false, motion: 'enter' }
  lightboxScalePercent.value = 100
  lightboxAtOriginalSize.value = false
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  showLightboxDock(true)
  scheduleLightboxScaleUpdate()
}
function closeLightbox() {
  lightbox.value = { open: false, media: [], index: 0, author: '', scale: 1, rotation: 0, fit: true, x: 0, y: 0, dragging: false, motion: 'enter' }
  lightboxPointers.clear()
  lightboxGesture = null
  lightboxGestureHadPinch = false
  lightboxAtOriginalSize.value = false
  lightboxLastTap = { time: 0, x: 0, y: 0 }
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  clearLightboxLongPress()
  clearLightboxDockTimer()
  if (lightboxScaleFrame) window.cancelAnimationFrame(lightboxScaleFrame)
  lightboxScaleFrame = 0
}
function moveLightbox(step) {
  const total = lightbox.value.media.length
  if (total > 1) {
    lightbox.value.index = (lightbox.value.index + step + total) % total
    resetLightboxView()
    lightbox.value.motion = step > 0 ? 'next' : 'previous'
  }
}
function zoomLightbox(event) {
  zoomLightboxBy(event.deltaY < 0 ? 0.15 : -0.15)
}
function zoomLightboxBy(step) {
  lightbox.value.scale = Math.min(lightboxMaximumScale(), Math.max(0.25, Number((lightbox.value.scale + step).toFixed(2))))
  scheduleLightboxScaleUpdate()
}
function toggleLightboxFit() {
  lightbox.value.fit = !lightbox.value.fit
  lightbox.value.scale = 1
  lightbox.value.x = 0
  lightbox.value.y = 0
  scheduleLightboxScaleUpdate()
}
function rotateLightbox() {
  lightbox.value.rotation = (lightbox.value.rotation + 90) % 360
}
async function downloadLightboxImage() {
  const source = lightbox.value.media[lightbox.value.index]
  if (!source) return
  const fallbackName = `${lightbox.value.author || 'Lumic'}-${lightbox.value.index + 1}.jpg`
  try {
    const response = await fetch(source)
    if (!response.ok) throw new Error('download failed')
    const blobURL = URL.createObjectURL(await response.blob())
    const link = document.createElement('a')
    link.href = blobURL
    link.download = decodeURIComponent(source.split('/').pop()?.split('?')[0] || fallbackName)
    link.click()
    URL.revokeObjectURL(blobURL)
  } catch {
    const link = document.createElement('a')
    link.href = source
    link.download = fallbackName
    link.target = '_blank'
    link.rel = 'noopener'
    link.click()
  }
}
function clearLightboxDockTimer() {
  if (lightboxDockTimer) window.clearTimeout(lightboxDockTimer)
  lightboxDockTimer = 0
}
function scheduleLightboxDockHide(delay = 2400) {
  clearLightboxDockTimer()
  if (phonePortrait.value) return
  lightboxDockTimer = window.setTimeout(() => {
    lightboxDockVisible.value = false
    lightboxDockTimer = 0
  }, delay)
}
function showLightboxDock(autoHide = false) {
  if (phonePortrait.value) return
  clearLightboxDockTimer()
  lightboxDockVisible.value = true
  if (autoHide) scheduleLightboxDockHide()
}
function clearLightboxLongPress() {
  if (lightboxLongPressTimer) window.clearTimeout(lightboxLongPressTimer)
  lightboxLongPressTimer = 0
}
function scheduleLightboxLongPress(event) {
  clearLightboxLongPress()
  if (!phonePortrait.value || event.pointerType !== 'touch' || event.target !== lightboxImageElement.value) return
  const pointerId = event.pointerId
  const x = event.clientX
  const y = event.clientY
  lightboxLongPressTimer = window.setTimeout(() => {
    const pointer = lightboxPointers.get(pointerId)
    if (!pointer || lightboxPointers.size !== 1 || Math.hypot(pointer.x - x, pointer.y - y) > 10) return
    if (lightboxGesture?.type === 'pan' && lightboxGesture.pointerId === pointerId) lightboxGesture.longPressed = true
    lightbox.value.dragging = false
    mobileLightboxMenu.value = { open: true, x: 0, y: 0 }
    navigator.vibrate?.(12)
    lightboxLongPressTimer = 0
  }, 560)
}
function saveMobileLightboxImage() {
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  downloadLightboxImage()
}
function scheduleLightboxScaleUpdate() {
  if (typeof window === 'undefined') return
  if (lightboxScaleFrame) window.cancelAnimationFrame(lightboxScaleFrame)
  nextTick(() => {
    lightboxScaleFrame = window.requestAnimationFrame(() => {
      lightboxScaleFrame = 0
      const image = lightboxImageElement.value
      if (!image?.naturalWidth) return
      const baseScale = lightbox.value.fit ? image.clientWidth / image.naturalWidth : 1
      const renderedScale = baseScale * lightbox.value.scale
      lightboxScalePercent.value = Math.max(1, Math.round(renderedScale * 100))
      lightboxAtOriginalSize.value = Math.abs(renderedScale - 1) < 0.015
    })
  })
}
function lightboxBaseScale() {
  const image = lightboxImageElement.value
  if (!image?.naturalWidth || !lightbox.value.fit) return 1
  return Math.max(0.01, image.clientWidth / image.naturalWidth)
}
function lightboxMaximumScale() {
  return Math.min(20, Math.max(5, 2 / lightboxBaseScale()))
}
function isLightboxOriginalSize() {
  return Math.abs(lightboxBaseScale() * lightbox.value.scale - 1) < 0.015
}
function lightboxPointerDistance(points) {
  return Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y)
}
function lightboxPointerCenter(points) {
  return { x: (points[0].x + points[1].x) / 2, y: (points[0].y + points[1].y) / 2 }
}
function beginLightboxPan(pointer) {
  lightboxGesture = {
    type: 'pan', pointerId: pointer.id, startX: pointer.x, startY: pointer.y,
    startTime: pointer.time, imageX: lightbox.value.x, imageY: lightbox.value.y,
    startScale: lightbox.value.scale, startFit: lightbox.value.fit,
    startOriginal: isLightboxOriginalSize(), startedOnImage: pointer.startedOnImage, longPressed: false
  }
}
function beginLightboxPinch() {
  clearLightboxLongPress()
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  const points = [...lightboxPointers.values()].slice(0, 2)
  if (points.length < 2) return
  lightboxGestureHadPinch = true
  lightbox.value.dragging = true
  lightboxGesture = {
    type: 'pinch', startDistance: Math.max(1, lightboxPointerDistance(points)),
    startCenter: lightboxPointerCenter(points), startScale: lightbox.value.scale,
    imageX: lightbox.value.x, imageY: lightbox.value.y
  }
}
function startLightboxGesture(event) {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  if (mobileLightboxMenu.value.open) mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  event.currentTarget.setPointerCapture(event.pointerId)
  lightboxPointers.set(event.pointerId, { id: event.pointerId, x: event.clientX, y: event.clientY, time: event.timeStamp, startedOnImage: event.target === lightboxImageElement.value })
  lightbox.value.dragging = !(phonePortrait.value && isLightboxOriginalSize())
  if (lightboxPointers.size === 1) {
    lightboxGestureHadPinch = false
    beginLightboxPan(lightboxPointers.get(event.pointerId))
    scheduleLightboxLongPress(event)
  }
  else beginLightboxPinch()
}
function moveLightboxGesture(event) {
  const pointer = lightboxPointers.get(event.pointerId)
  if (!pointer) return
  pointer.x = event.clientX
  pointer.y = event.clientY
  if (lightboxGesture?.type === 'pan' && Math.hypot(event.clientX - lightboxGesture.startX, event.clientY - lightboxGesture.startY) > 10) clearLightboxLongPress()
  if (lightboxPointers.size >= 2) {
    if (lightboxGesture?.type !== 'pinch') beginLightboxPinch()
    const points = [...lightboxPointers.values()].slice(0, 2)
    const center = lightboxPointerCenter(points)
    const ratio = lightboxPointerDistance(points) / lightboxGesture.startDistance
    lightbox.value.scale = Math.min(lightboxMaximumScale(), Math.max(0.25, Number((lightboxGesture.startScale * ratio).toFixed(3))))
    lightbox.value.x = lightboxGesture.imageX + center.x - lightboxGesture.startCenter.x
    lightbox.value.y = lightboxGesture.imageY + center.y - lightboxGesture.startCenter.y
    scheduleLightboxScaleUpdate()
    return
  }
  if (lightboxGesture?.type !== 'pan' || lightboxGesture.pointerId !== event.pointerId) return
  if (phonePortrait.value && lightboxGesture.startOriginal) return
  lightbox.value.x = lightboxGesture.imageX + event.clientX - lightboxGesture.startX
  lightbox.value.y = lightboxGesture.imageY + event.clientY - lightboxGesture.startY
}
function toggleLightboxDoubleTap() {
  if (phonePortrait.value) {
    if (isLightboxOriginalSize() && Math.abs(lightbox.value.x) < 2 && Math.abs(lightbox.value.y) < 2) return
    lightbox.value.fit = false
  } else {
    lightbox.value.fit = !lightbox.value.fit || Math.abs(lightbox.value.scale - 1) > 0.02 || Math.abs(lightbox.value.x) > 2 || Math.abs(lightbox.value.y) > 2
  }
  lightbox.value.scale = 1
  lightbox.value.x = 0
  lightbox.value.y = 0
  scheduleLightboxScaleUpdate()
}
function stopLightboxGesture(event) {
  const pointer = lightboxPointers.get(event.pointerId)
  const gesture = lightboxGesture
  if (!pointer) return
  const dx = event.clientX - (gesture?.startX ?? event.clientX)
  const dy = event.clientY - (gesture?.startY ?? event.clientY)
  const duration = event.timeStamp - (gesture?.startTime ?? event.timeStamp)
  const wasSinglePan = gesture?.type === 'pan' && gesture.pointerId === event.pointerId && lightboxPointers.size === 1
  const wasCancelled = event.type === 'pointercancel'
  const canSwipe = phonePortrait.value ? gesture?.startOriginal : gesture?.startFit && gesture?.startScale <= 1.02 && Math.abs(gesture?.imageX || 0) < 3 && Math.abs(gesture?.imageY || 0) < 3
  const isSwipe = !wasCancelled && !lightboxGestureHadPinch && wasSinglePan && !gesture.longPressed && canSwipe && duration < 620 && Math.abs(dx) > 58 && Math.abs(dx) > Math.abs(dy) * 1.3
  const isTap = !wasCancelled && !lightboxGestureHadPinch && wasSinglePan && !gesture.longPressed && duration < 280 && Math.hypot(dx, dy) < 10

  clearLightboxLongPress()
  if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  lightboxPointers.delete(event.pointerId)
  if (isSwipe && lightbox.value.media.length > 1) {
    moveLightbox(dx < 0 ? 1 : -1)
  } else if (isTap) {
    const sinceLastTap = event.timeStamp - lightboxLastTap.time
    const nearLastTap = Math.hypot(event.clientX - lightboxLastTap.x, event.clientY - lightboxLastTap.y) < 34
    if (sinceLastTap > 0 && sinceLastTap < 340 && nearLastTap) {
      toggleLightboxDoubleTap()
      lightboxLastTap = { time: 0, x: 0, y: 0 }
    } else {
      lightboxLastTap = { time: event.timeStamp, x: event.clientX, y: event.clientY }
    }
  }

  if (lightboxPointers.size >= 2) beginLightboxPinch()
  else if (lightboxPointers.size === 1) beginLightboxPan([...lightboxPointers.values()][0])
  else {
    lightbox.value.dragging = false
    lightboxGesture = null
    lightboxGestureHadPinch = false
    if (phonePortrait.value && isLightboxOriginalSize()) {
      lightbox.value.x = 0
      lightbox.value.y = 0
    }
  }
}
function closeMobileNavigationOnInputFocus() {
  timelineSearchFocused.value = true
  if (phonePortrait.value) mobileMenuOpen.value = false
}
function releaseTimelineSearchFocus() {
  timelineSearchFocused.value = false
}
function handleWindowResize() {
  scheduleTimelineWindow()
  if (lightbox.value.open) scheduleLightboxScaleUpdate()
}
function handleGlobalKeydown(event) {
  if (showBrandMenu.value && event.key === 'Escape') {
    showBrandMenu.value = false
    return
  }
  if (contextMenu.value.open) {
    if (event.key === 'Escape') closeContextMenu()
    return
  }
  if (lightbox.value.open) {
    if (event.key === 'Escape') closeLightbox()
    if (event.key === 'ArrowLeft') moveLightbox(-1)
    if (event.key === 'ArrowRight') moveLightbox(1)
    if (event.key === '+' || event.key === '=') zoomLightboxBy(0.15)
    if (event.key === '-') zoomLightboxBy(-0.15)
    if (event.key.toLowerCase() === 'r') rotateLightbox()
    return
  }
  if (masonryDetailPost.value && event.key === 'Escape') {
    closePostDetail()
    return
  }
  if (credentialPlatform.value && event.key === 'Escape') {
    credentialPlatform.value = null
    return
  }
  if (!confirmDialog.value.open) return
  if (event.key === 'Escape') closeConfirmDialog(false)
  if (event.key === 'Enter') closeConfirmDialog(true)
}
async function deleteSource(feed) {
  const confirmed = await askConfirm({
    title: '删除订阅',
    message: `确定删除“${feed.name}”的订阅吗？已采集到 /flow 的文件会保留。`,
    confirmText: '删除订阅'
  })
  if (!confirmed) return
  sourceActionBusy.value = `delete:${feed.id}`; sourceActionMessage.value = ''; settingsError.value = ''
  const endpoint = sourceOperationEndpoint(feed)
  try {
    const response = await fetch(`${endpoint}?id=${encodeURIComponent(feed.id)}`, { method: 'DELETE' })
    if (!response.ok) throw new Error(await responseError(response, '删除订阅失败'))
    feeds.value = feeds.value.filter(item => item.id !== feed.id)
    if (selectedPlatform.value) selectedPlatform.value.feeds = selectedPlatform.value.feeds.filter(item => item.id !== feed.id)
    sourceActionMessage.value = '订阅已删除，已采集文件予以保留'
  } catch (error) { settingsError.value = error.message } finally { sourceActionBusy.value = '' }
}
async function deletePost(post) {
  const summary = post.caption?.trim() ? `“${post.caption.trim().slice(0, 36)}${post.caption.trim().length > 36 ? '…' : ''}”` : '这条动态'
  const confirmed = await askConfirm({
    title: '删除动态',
    message: `确定删除 ${post.author} 的${summary}吗？该操作会同时删除时间线记录及已下载到 /flow 的关联媒体文件。`,
    confirmText: '删除动态'
  })
  if (!confirmed) return
  postActionBusy.value = post.id; timelineMessage.value = ''
  try {
    const response = await fetch(`/api/posts?id=${encodeURIComponent(post.id)}`, { method: 'DELETE' })
    if (!response.ok) throw new Error(await responseError(response, '删除动态失败'))
    posts.value = posts.value.filter(item => item.id !== post.id)
    timelineMessage.value = '动态已从时间线删除'
    scheduleTransient(() => { if (timelineMessage.value === '动态已从时间线删除') timelineMessage.value = '' }, 2500)
  } catch (error) { timelineMessage.value = error.message } finally { postActionBusy.value = '' }
}
function togglePostSelection(post) {
  selectedPostIds.value = selectedPostIds.value.includes(post.id) ? selectedPostIds.value.filter(id => id !== post.id) : [...selectedPostIds.value, post.id]
}
function handlePostSelectionClick(event, post) {
  if (!selectionMode.value) return
  event.preventDefault()
  event.stopPropagation()
  togglePostSelection(post)
}
function stopSelection() { selectionMode.value = false; selectionAction.value = 'delete'; selectedPostIds.value = [] }
function closeContextMenu() { contextMenu.value = { open: false, x: 0, y: 0, post: null } }
function openContextMenu(event, post = null) {
  const width = 218
  const height = post ? 118 : 76
  contextMenu.value = { open: true, x: Math.min(event.clientX, window.innerWidth - width - 12), y: Math.min(event.clientY, window.innerHeight - height - 12), post }
}
function startMultiSelectMode() {
  const unfavoriteMode = activeNav.value === 'liked'
  closeContextMenu()
  selectionAction.value = unfavoriteMode ? 'unfavorite' : 'delete'
  selectedPostIds.value = []
  selectionMode.value = true
}
function toggleSelectAllPosts() {
  selectedPostIds.value = allFilteredPostsSelected.value ? [] : filteredPosts.value.map(post => post.id)
}
function deleteContextPost() {
  const post = contextMenu.value.post
  closeContextMenu()
  if (post) deletePost(post)
}
async function unfavoriteContextPost() {
  const post = contextMenu.value.post
  closeContextMenu()
  if (post) await updatePostFavorite(post, false)
}
async function deleteSelectedPosts() {
  if (!selectedPostCount.value) return
  const confirmed = await askConfirm({ title: '批量删除动态', message: `确定永久删除选中的 ${selectedPostCount.value} 条动态吗？关联媒体文件也会一并删除。`, confirmText: '删除所选动态' })
  if (!confirmed) return
  postActionBusy.value = 'batch-delete'
  try {
    const response = await fetch('/api/posts', { method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids: selectedPostIds.value }) })
    if (!response.ok) throw new Error(await responseError(response, '批量删除失败'))
    const removed = new Set(selectedPostIds.value)
    posts.value = posts.value.filter(post => !removed.has(post.id))
    timelineMessage.value = `已删除 ${removed.size} 条动态`
    stopSelection()
  } catch (error) { timelineMessage.value = error.message } finally { postActionBusy.value = '' }
}
async function unfavoriteSelectedPosts() {
  if (!selectedPostCount.value) return
  postActionBusy.value = 'batch-unfavorite'
  timelineMessage.value = ''
  try {
    const response = await fetch('/api/posts', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids: selectedPostIds.value, liked: false }) })
    if (!response.ok) throw new Error(await responseError(response, '批量取消收藏失败'))
    const selected = new Set(selectedPostIds.value)
    posts.value = posts.value.map(post => selected.has(post.id) ? { ...post, liked: false, favoriteExplicit: true } : post)
    timelineMessage.value = `已取消收藏 ${selected.size} 条动态`
    stopSelection()
  } catch (error) { timelineMessage.value = error.message } finally { postActionBusy.value = '' }
}
async function deleteAuthorPosts(source, author) {
  const count = posts.value.filter(post => post.source === source && post.author === author).length
  const countText = count ? `全部 ${count} 条动态` : '全部动态记录'
  const confirmed = await askConfirm({ title: '删除作者全部动态', message: `确定永久删除“${author}”的${countText}及其 /flow 内容目录吗？订阅关系会保留，后续同步仍可重新创建目录并拉取。`, confirmText: '删除动态及文件' })
  if (!confirmed) return
  postActionBusy.value = `author:${source}:${author}`
  try {
    const response = await fetch('/api/posts', { method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ source, author }) })
    if (!response.ok) throw new Error(await responseError(response, '删除作者动态失败'))
    const result = await response.json()
    posts.value = posts.value.filter(post => post.source !== source || post.author !== author)
    timelineMessage.value = `已删除 ${author} 的 ${result.count ?? count} 条动态及相关文件`
  } catch (error) { timelineMessage.value = error.message } finally { postActionBusy.value = '' }
}
async function syncWeiboLikes() {
  if (!weiboAccount.value.configured) { await openSettings('platforms'); return }
  syncing.value = true; timelineMessage.value = ''
  try {
    const response = await fetch('/api/weibo/likes', { method: 'POST' })
    if (!response.ok) throw new Error(await responseError(response, '微博点赞同步失败'))
    const result = await response.json()
    await loadData()
    timelineMessage.value = result.message
  } catch (error) { timelineMessage.value = error.message } finally { syncing.value = false }
}
async function addWeiboLikesSource() {
  sourceActionBusy.value = 'add:weibo-likes'; sourceActionMessage.value = ''; settingsError.value = ''
  try {
    const response = await fetch('/api/weibo/subscriptions?type=likes', { method: 'POST' })
    if (!response.ok) throw new Error(await responseError(response, response.status === 409 ? '微博我的点赞来源已添加' : '添加微博点赞来源失败'))
    const feed = await response.json(); feeds.value.push(feed)
    if (selectedPlatform.value?.key === 'weibo') selectedPlatform.value.feeds.push(feed)
    sourceActionMessage.value = '已添加“我的点赞”，可设置启停、Cron 和过滤规则'
  } catch (error) { settingsError.value = error.message } finally { sourceActionBusy.value = '' }
}
async function addPixivBookmarksSource() {
  sourceActionBusy.value = 'add:pixiv-bookmarks'; sourceActionMessage.value = ''; settingsError.value = ''
  try {
    const response = await fetch('/api/pixiv/subscriptions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ bookmarks: true, schedule: '0 6 * * *', tags: ['P站收藏'] }) })
    if (!response.ok) throw new Error(await responseError(response, response.status === 409 ? 'P站收藏来源已添加' : '添加 P站收藏来源失败'))
    const feed = await response.json(); await loadFeeds(feed)
    if (selectedPlatform.value?.key === 'pixiv') selectedPlatform.value.feeds = feeds.value.filter(item => item.source === 'pixiv')
    sourceActionMessage.value = '已添加“P站收藏”，默认标签为 #P站收藏'
  } catch (error) { settingsError.value = error.message } finally { sourceActionBusy.value = '' }
}
async function addBilibiliFavoriteOpusSource() {
  sourceActionBusy.value = 'add:bili-opus-favorites'; sourceActionMessage.value = ''; settingsError.value = ''
  try {
    const response = await fetch('/api/bilibili/subscriptions?type=favorite-opus', { method: 'POST' })
    if (!response.ok) throw new Error(await responseError(response, response.status === 409 ? 'B站收藏专栏来源已添加' : '添加 B站收藏专栏来源失败'))
    const feed = await response.json(); await loadFeeds(feed)
    if (selectedPlatform.value?.key === 'bilibili') selectedPlatform.value.feeds = feeds.value.filter(item => item.source === 'bilibili')
    sourceActionMessage.value = '已添加“收藏专栏”，可手动获取尚未归档的历史收藏'
  } catch (error) { settingsError.value = error.message } finally { sourceActionBusy.value = '' }
}
function setMediaShape(post, mediaIndex, event) {
  const image = event.target
  if (!image.naturalWidth || !image.naturalHeight) return
  mediaShapes.value[`${post.id}:${mediaIndex}`] = image.naturalWidth >= image.naturalHeight ? 'landscape' : 'portrait'
  mediaRatios.value[`${post.id}:${mediaIndex}`] = image.naturalWidth / image.naturalHeight
}
function setMasonryCoverRatio(post, event, videoPoster = false) {
  const media = event.target
  const width = media.naturalWidth || media.videoWidth
  const height = media.naturalHeight || media.videoHeight
  if (!width || !height) return
  const ratio = width / height
  mediaRatios.value[`${post.id}:${videoPoster ? 'video' : 0}`] = ratio
  if (videoPoster) videoRatios.value[post.id] = ratio
  scheduleTimelineWindow()
}
function mediaShape(post, mediaIndex) {
  return mediaShapes.value[`${post.id}:${mediaIndex}`] || 'unknown'
}
function masonryCover(post) {
  return post.media?.[0] || primaryVideo(post)?.poster || ''
}
function masonryCoverIsVideo(post) {
  return !post.media?.length && Boolean(primaryVideo(post))
}
function masonryPostHasVideo(post) {
  return Boolean(primaryVideo(post))
}
function primaryVideo(post) {
  return post?.videos?.find(video => String(video?.url || '').trim()) || null
}
function postDetailMedia(post) {
  if (!post) return []
  const images = (post.media || []).map((src, index) => ({ type: 'image', src, key: `image:${index}:${src}` }))
  const video = primaryVideo(post)
  if (video) images.push({ type: 'video', src: video.url, poster: video.poster || '', key: `video:${video.url}` })
  return images
}
function setPostVideoRatio(post, event) {
  const video = event.target
  if (!video.videoWidth || !video.videoHeight) return
  const ratio = video.videoWidth / video.videoHeight
  videoRatios.value[post.id] = ratio
  mediaRatios.value[`${post.id}:video`] = ratio
  scheduleTimelineWindow()
}
function postVideoFrameClass(post) {
  const ratio = videoRatios.value[post?.id] || mediaRatios.value[`${post?.id}:video`] || 16 / 9
  return ratio < 1 ? 'portrait' : 'landscape'
}
function postVideoFrameStyle(post) {
  const ratio = videoRatios.value[post?.id] || mediaRatios.value[`${post?.id}:video`] || 16 / 9
  return { '--post-video-ratio': String(ratio) }
}
function toggleTimelineView(event) {
  timelineView.value = timelineView.value === 'list' ? 'masonry' : 'list'
  if (event?.detail > 0) event.currentTarget?.blur()
}
function openMasonryPost(post) {
  if (selectionMode.value) return
  masonryDetailPost.value = post
  mobileDetailIndex.value = 0
  mobileDetailMotion.value = 'enter'
  if (phonePortrait.value) {
    mobilePostReturnPath.value = window.location.pathname.startsWith('/post/') ? '/' : window.location.pathname
    updateRoute(`/post/${encodeURIComponent(post.id)}`)
    window.scrollTo({ top: 0, behavior: 'auto' })
  }
}
function closePostDetail() {
  masonryDetailPost.value = null
  mobileDetailIndex.value = 0
  mobileDetailMotion.value = 'enter'
  pendingPostId.value = ''
  if (window.location.pathname.startsWith('/post/')) {
    updateRoute(mobilePostReturnPath.value || '/', true)
  }
}
function moveMobileDetailMedia(direction) {
  const count = mobileDetailMedia.value.length
  if (count < 2) return
  mobileDetailMotion.value = direction > 0 ? 'next' : 'previous'
  mobileDetailIndex.value = (mobileDetailIndex.value + direction + count) % count
}
function selectMobileDetailMedia(index) {
  if (index === mobileDetailIndex.value) return
  moveMobileDetailMedia(index > mobileDetailIndex.value ? index - mobileDetailIndex.value : index - mobileDetailIndex.value)
}
function openMobileDetailImage() {
  if (mobileDetailSwipeClickBlocked || mobileDetailCurrentMedia.value?.type !== 'image') {
    mobileDetailSwipeClickBlocked = false
    return
  }
  openLightbox(masonryDetailPost.value, mobileDetailIndex.value)
}
function beginMobileDetailSwipe(event) {
  const touch = event.changedTouches?.[0]
  if (!touch) return
  mobileDetailSwipeClickBlocked = false
  mobileDetailTouch = { x: touch.clientX, y: touch.clientY }
}
function finishMobileDetailSwipe(event) {
  const touch = event.changedTouches?.[0]
  if (!touch || !mobileDetailTouch) return
  const deltaX = touch.clientX - mobileDetailTouch.x
  const deltaY = touch.clientY - mobileDetailTouch.y
  mobileDetailTouch = null
  if (Math.abs(deltaX) < 48 || Math.abs(deltaX) <= Math.abs(deltaY) * 1.15) return
  mobileDetailSwipeClickBlocked = true
  scheduleTransient(() => { mobileDetailSwipeClickBlocked = false }, 450)
  moveMobileDetailMedia(deltaX < 0 ? 1 : -1)
}
function masonryItemStyle(item) {
  return {
    left: `${item.x}px`,
    top: `${item.y}px`,
    width: `${masonryColumnWidth.value}px`,
    '--masonry-delay': `${(item.index % 4) * 18}ms`
  }
}
function previewMedia(media) {
  const value = String(media || '')
  if (!value.startsWith('/flow/')) return value
  const path = value.split('?', 1)[0]
  return `/preview/${path.slice('/flow/'.length)}?v=4`
}
function scheduleTransient(callback, delay) {
  const timer = window.setTimeout(() => {
    transientTimers.delete(timer)
    callback()
  }, delay)
  transientTimers.add(timer)
  return timer
}
function prunePostCaches() {
  const activeIds = new Set(posts.value.map(post => String(post.id)))
  for (const id of animatedMasonryPostIds) {
    if (!activeIds.has(id)) animatedMasonryPostIds.delete(id)
  }
  for (const [key, element] of observedPostElements) {
    const id = key.slice(key.indexOf(':') + 1)
    if (activeIds.has(id)) continue
    postResizeObserver?.unobserve(element)
    observedPostElements.delete(key)
  }
  timelineHeights.value = Object.fromEntries(Object.entries(timelineHeights.value).filter(([id]) => activeIds.has(id)))
  masonryHeights.value = Object.fromEntries(Object.entries(masonryHeights.value).filter(([id]) => activeIds.has(id)))
  mediaShapes.value = Object.fromEntries(Object.entries(mediaShapes.value).filter(([key]) => activeIds.has(key.split(':', 1)[0])))
  mediaRatios.value = Object.fromEntries(Object.entries(mediaRatios.value).filter(([key]) => activeIds.has(key.split(':', 1)[0])))
  videoRatios.value = Object.fromEntries(Object.entries(videoRatios.value).filter(([id]) => activeIds.has(id)))
  if (masonryDetailPost.value && !activeIds.has(String(masonryDetailPost.value.id))) masonryDetailPost.value = null
}
async function updatePostFavorite(post, liked) {
	if (postActionBusy.value) return
	const previous = Boolean(post.liked)
	if (previous === Boolean(liked)) return
	post.liked = Boolean(liked)
	postActionBusy.value = `like:${post.id}`
  timelineMessage.value = ''
  try {
    const response = await fetch(`/api/posts?id=${encodeURIComponent(post.id)}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ liked: post.liked }) })
    if (!response.ok) throw new Error(await responseError(response, '收藏状态保存失败'))
    const updated = await response.json()
    post.liked = Boolean(updated.liked)
  } catch (error) {
    post.liked = previous
    timelineMessage.value = error.message
  } finally {
    postActionBusy.value = ''
	}
}
function togglePostLike(post) { return updatePostFavorite(post, !post.liked) }
function navigateTo(nav, source = activeSource.value) {
  stopSelection()
  masonryDetailPost.value = null
  showBrandMenu.value = false
  selectedTag.value = ''
  showSettings.value = false
  selectedPlatform.value = null
  showFeedSettings.value = false
  activeNav.value = nav
  activeSource.value = source
  selectedAuthor.value = null
  mobileMenuOpen.value = false
  const path = nav === 'source' ? `/source/${source}` : nav === 'liked' ? '/favorites' : nav === 'pulls' ? '/pulls' : '/'
  updateRoute(path)
  if (nav === 'pulls') void Promise.all([loadFeeds(), loadPlatformAccounts()])
}
function openTag(tag) {
  stopSelection(); masonryDetailPost.value = null; showSettings.value = false; selectedAuthor.value = null; selectedTag.value = tag; activeNav.value = 'tag'; activeSource.value = 'all'
  updateRoute(`/tag/${encodeURIComponent(tag)}`)
}
function measurePostElement(post, element, layout = element?.dataset.layout || 'list') {
  const height = Math.ceil(element.getBoundingClientRect().height)
  const heights = layout === 'masonry' ? masonryHeights : timelineHeights
  if (height > 0 && heights.value[post.id] !== height) heights.value[post.id] = height
}
function setPostCard(post, element, layout = 'list') {
  const id = String(post.id)
  const key = `${layout}:${id}`
  const previous = observedPostElements.get(key)
  if (!element) {
    if (previous) postResizeObserver?.unobserve(previous)
    observedPostElements.delete(key)
    return
  }
  if (previous && previous !== element) postResizeObserver?.unobserve(previous)
  element.dataset.postId = post.id
  element.dataset.layout = layout
  observedPostElements.set(key, element)
  if (layout === 'masonry' && !animatedMasonryPostIds.has(id)) {
    animatedMasonryPostIds.add(id)
    if (masonryScrollDirection !== 'up') {
      element.classList.add('masonry-card-enter')
      scheduleTransient(() => element.classList.remove('masonry-card-enter'), 620)
    }
  }
  measurePostElement(post, element, layout)
  postResizeObserver?.observe(element)
}
function updateMasonryMetrics() {
  if (!isMasonryView.value || !feedListElement.value) return
  const availableWidth = Math.max(0, feedListElement.value.clientWidth)
  if (!availableWidth) return
  const gap = phonePortrait.value ? 8 : 14
  const count = phonePortrait.value ? 2 : Math.min(5, Math.max(3, Math.floor((availableWidth + gap) / (252 + gap))))
  const width = Math.max(0, (availableWidth - gap * (count - 1)) / count)
  if (masonryGap.value !== gap) masonryGap.value = gap
  if (masonryColumnCount.value !== count) masonryColumnCount.value = count
  if (Math.abs(masonryColumnWidth.value - width) > 0.5) masonryColumnWidth.value = width
}
function updateTimelineWindow() {
  timelineFrame = 0
  if (!filteredPosts.value.length || showSettings.value || activeNav.value === 'pulls') return
  const currentScrollY = window.scrollY
  if (isMasonryView.value) {
    const scrollDelta = currentScrollY - lastTimelineScrollY
    if (scrollDelta > 2) masonryScrollDirection = 'down'
    else if (scrollDelta < -2) masonryScrollDirection = 'up'
    lastTimelineScrollY = currentScrollY
    updateMasonryMetrics()
    const listTop = feedListElement.value?.getBoundingClientRect().top + window.scrollY || 0
    masonryViewportTop.value = Math.max(0, window.scrollY - listTop)
    masonryViewportBottom.value = masonryViewportTop.value + window.innerHeight
    return
  }
  lastTimelineScrollY = currentScrollY
  const listTop = feedListElement.value?.getBoundingClientRect().top + window.scrollY || 0
  const viewportTop = Math.max(0, window.scrollY - listTop)
  const viewportBottom = viewportTop + window.innerHeight
  let offset = 0
  let first = 0
  let last = filteredPosts.value.length
  for (let index = 0; index < filteredPosts.value.length; index++) {
    const height = (timelineHeights.value[filteredPosts.value[index].id] || estimatedPostHeight) + 15
    if (offset + height >= viewportTop) { first = index; break }
    offset += height
  }
  let visibleOffset = offset
  for (let index = first; index < filteredPosts.value.length; index++) {
    visibleOffset += (timelineHeights.value[filteredPosts.value[index].id] || estimatedPostHeight) + 15
    if (visibleOffset >= viewportBottom) { last = index + 1; break }
  }
  const overscan = phonePortrait.value ? 2 : timelineOverscan
  timelineStart.value = Math.max(0, first - overscan)
  timelineEnd.value = Math.min(filteredPosts.value.length, last + overscan)
}
function scheduleTimelineWindow() {
  if (!timelineFrame) timelineFrame = window.requestAnimationFrame(updateTimelineWindow)
}
function resetTimelineWindow() {
  timelineStart.value = 0
  timelineEnd.value = Math.min(filteredPosts.value.length, phonePortrait.value ? 7 : 15)
  masonryViewportTop.value = 0
  masonryViewportBottom.value = typeof window === 'undefined' ? 900 : window.innerHeight
  lastTimelineScrollY = typeof window === 'undefined' ? 0 : window.scrollY
  masonryScrollDirection = 'idle'
  animatedMasonryPostIds.clear()
  nextTick(() => { updateMasonryMetrics(); scheduleTimelineWindow() })
}
function isPhonePortraitScreen() {
  return window.matchMedia('(max-width: 760px)').matches
}
function updatePhonePortrait() {
  const nextPhonePortrait = isPhonePortraitScreen()
  if (phonePortrait.value === nextPhonePortrait) return
  phonePortrait.value = nextPhonePortrait
  mobileMenuOpen.value = false
  if (lightbox.value.open && !nextPhonePortrait) showLightboxDock(true)
  if (nextPhonePortrait && masonryDetailPost.value && !window.location.pathname.startsWith('/post/')) {
    mobilePostReturnPath.value = window.location.pathname
    updateRoute(`/post/${encodeURIComponent(masonryDetailPost.value.id)}`)
  }
  if (nextPhonePortrait) openPhoneDefaultTimeline()
  resetTimelineWindow()
}
function openPhoneDefaultTimeline() {
  if (window.location.pathname.startsWith('/post/')) return
  showSettings.value = false
  activeNav.value = 'all'
  activeSource.value = 'all'
  selectedAuthor.value = null
  selectedTag.value = ''
  updateRoute('/', true)
}
function openAuthor(post) {
  masonryDetailPost.value = null
  authorReturnState.value = { nav: activeNav.value, source: activeSource.value }
  showSettings.value = false
  activeNav.value = 'author'
  activeSource.value = post.source
  selectedAuthor.value = { name: post.author, source: post.source, avatar: post.avatar }
  updateRoute(`/author/${post.source}/${encodeURIComponent(post.author)}`)
}
function closeAuthor() {
  navigateTo(authorReturnState.value.nav, authorReturnState.value.source)
}
function closeSettingsPage() { navigateTo(activeSource.value === 'all' ? 'all' : 'source') }
function updateRoute(path, replace = false) {
  if (window.location.pathname === path) return
  window.history[replace ? 'replaceState' : 'pushState']({}, '', path)
}
function applyRoute() {
  const segments = window.location.pathname.split('/').filter(Boolean).map(segment => decodeURIComponent(segment))
  showSettings.value = false
  masonryDetailPost.value = null
  selectedAuthor.value = null
  selectedTag.value = ''
  pendingPostId.value = ''
  if (segments[0] === 'post' && segments[1]) {
    activeNav.value = 'all'; activeSource.value = 'all'; pendingPostId.value = segments.slice(1).join('/')
    resolveRoutedPost()
    return
  }
  if (segments[0] === 'source' && validSources.has(segments[1])) {
    activeNav.value = 'source'; activeSource.value = segments[1]
    return
  }
  if (segments[0] === 'liked' || segments[0] === 'favorites') {
    activeNav.value = 'liked'; activeSource.value = 'all'
    return
  }
  if (segments[0] === 'tag' && segments[1]) {
    activeNav.value = 'tag'; activeSource.value = 'all'; selectedTag.value = segments.slice(1).join('/')
    return
  }
  if (segments[0] === 'pulls') {
    activeNav.value = 'pulls'; activeSource.value = 'all'
    void Promise.all([loadFeeds(), loadPlatformAccounts()])
    return
  }
  if (segments[0] === 'settings') {
    activeNav.value = 'settings'; showSettings.value = true; settingsTab.value = 'settings'
    openSettings('settings', false)
    return
  }
  if (segments[0] === 'author' && validSources.has(segments[1]) && segments[2]) {
    activeNav.value = 'author'; activeSource.value = segments[1]; selectedAuthor.value = { source: segments[1], name: segments.slice(2).join('/'), avatar: '' }
    return
  }
  activeNav.value = 'all'; activeSource.value = 'all'
  if (segments.length) updateRoute('/', true)
}
function resolveRoutedPost() {
  if (!pendingPostId.value) return
  const post = posts.value.find(item => String(item.id) === pendingPostId.value)
  if (!post) return
  masonryDetailPost.value = post
  mobileDetailIndex.value = 0
}
function formatFans(count) { return count >= 10000 ? `${(count / 10000).toFixed(1)}万` : count }
function platformEmptyMessage(platformKey) {
  if (platformKey === 'bilibili') return '点击“添加 UP 主”开始订阅图文与专栏。'
  if (platformKey === 'weibo') return '点击“添加博主”开始订阅微博动态。'
  if (platformKey === 'pixiv') return '添加画师或账号收藏来源，作品将以原图归档。'
  if (platformKey === 'twitter') return '推特账号连接与作者采集器尚未开放。'
  return '作者订阅连接器将在后续版本开放。'
}
async function checkSession(refreshData = true) {
  try {
    const response = await fetch('/api/session', { credentials: 'same-origin' })
    if (!response.ok) throw new Error('session unavailable')
    const session = await response.json()
    if (!session.authenticated) {
      authenticated.value = false
      showBrandMenu.value = false
      return
    }
    authenticated.value = true
    if (refreshData) {
      await loadData()
      await loadPlatformAccounts()
    }
  } catch {
    loginError.value = '暂时无法连接服务，请稍后刷新页面'
  } finally {
    sessionChecked.value = true
  }
}
watch(filteredPosts, resetTimelineWindow)
watch(posts, prunePostCaches)
watch(timelineView, value => localStorage.setItem('lumic-timeline-view', value))
watch(effectiveTimelineView, resetTimelineWindow)
watch(isDark, value => { if (value) startNightMeteorLoop(); else stopNightMeteorLoop() })
watch(platformCards, cards => {
  if (!credentialPlatform.value) return
  credentialPlatform.value = cards.find(platform => platform.key === credentialPlatform.value.key) || null
})
watch(platformCards, cards => {
  if (!selectedPlatform.value) return
  selectedPlatform.value = cards.find(platform => platform.key === selectedPlatform.value.key) || null
})
onMounted(() => { isDark.value = localStorage.getItem('lumic-theme') === 'dark'; timelineView.value = localStorage.getItem('lumic-timeline-view') === 'masonry' ? 'masonry' : 'list'; document.querySelector('meta[name="theme-color"]')?.setAttribute('content', isDark.value ? '#080a0e' : '#fbf7ea'); phonePortraitQuery = window.matchMedia('(max-width: 760px)'); phonePortrait.value = isPhonePortraitScreen(); phonePortraitQuery.addEventListener('change', updatePhonePortrait); window.addEventListener('orientationchange', updatePhonePortrait); postResizeObserver = new ResizeObserver(entries => { for (const entry of entries) { const post = filteredPosts.value.find(item => String(item.id) === entry.target.dataset.postId); if (post) measurePostElement(post, entry.target) }; scheduleTimelineWindow() }); applyRoute(); if (phonePortrait.value && window.location.pathname === '/') openPhoneDefaultTimeline(); checkSession(); sessionPollTimer = window.setInterval(() => checkSession(false), 60_000); window.addEventListener('keydown', handleGlobalKeydown); window.addEventListener('popstate', applyRoute); window.addEventListener('scroll', scheduleTimelineWindow, { passive: true }); window.addEventListener('resize', handleWindowResize) })
onUnmounted(() => { stopWeiboPolling(); stopBilibiliPolling(); stopNightMeteorLoop(); if (sessionPollTimer) window.clearInterval(sessionPollTimer); phonePortraitQuery?.removeEventListener('change', updatePhonePortrait); window.removeEventListener('orientationchange', updatePhonePortrait); postResizeObserver?.disconnect(); observedPostElements.clear(); transientTimers.forEach(timer => window.clearTimeout(timer)); transientTimers.clear(); closeLightbox(); closeContextMenu(); window.removeEventListener('keydown', handleGlobalKeydown); window.removeEventListener('popstate', applyRoute); window.removeEventListener('scroll', scheduleTimelineWindow); window.removeEventListener('resize', handleWindowResize); if (timelineFrame) window.cancelAnimationFrame(timelineFrame); if (confirmResolver) closeConfirmDialog(false) })
</script>

<template>
  <div v-if="!sessionChecked" class="session-loading" aria-label="正在恢复登录状态">
    <span></span>
  </div>
  <div v-else-if="!authenticated" :class="['login-shell', { 'phone-ui-login': phonePortrait }]">
    <div class="login-panel">
      <div class="login-brand">
<img class="login-logo" src="/lumic-logo.png" alt="Lumic Logo">
<strong>Lumic</strong>
<small>拾光</small>
</div>
      <p class="login-project-note">拾起散落在时光里的片刻，珍藏每一次心动。</p>
      <form class="login-form" @submit.prevent="login" autocomplete="off">
        <label for="username">账号</label>
        <input id="username" v-model="credentials.username" type="text" name="username" autocomplete="username" required autofocus>
        <label for="password">密码</label>
        <input id="password" v-model="credentials.password" type="password" name="password" autocomplete="current-password" required>
        <p v-if="loginError" class="login-error" role="alert">{{ loginError }}</p>
        <button class="login-button" type="submit" :disabled="loginBusy">{{ loginBusy ? '验证中…' : '登录拾光' }}</button>
      </form>
    </div>
  </div>
  <div v-else class="app-shell" :class="{ dark: isDark, 'lightbox-active': lightbox.open, 'phone-ui': phonePortrait, 'timeline-search-focused': timelineSearchFocused }" @click="showBrandMenu = false">
    <button v-if="phonePortrait && !timelineSearchFocused && !masonryDetailPost" class="mobile-menu-toggle" type="button" :class="{ open: mobileMenuOpen }" :aria-expanded="mobileMenuOpen" :title="mobileMenuOpen ? '关闭导航' : '打开导航'" :aria-label="mobileMenuOpen ? '关闭导航' : '打开导航'" @pointerdown.stop @click.stop="mobileMenuOpen = !mobileMenuOpen">
      <svg viewBox="0 0 24 24" aria-hidden="true"><path class="menu-line menu-line-top" d="M5 7h14"/><path class="menu-line menu-line-middle" d="M5 12h14"/><path class="menu-line menu-line-bottom" d="M5 17h14"/></svg>
    </button>
    <button v-if="mobileMenuOpen" class="mobile-menu-scrim" type="button" aria-label="关闭导航" @click="mobileMenuOpen = false"></button>
    <aside class="sidebar" :class="{ 'mobile-open': mobileMenuOpen }">
      <div class="brand" @click.stop>
        <button class="brand-mark-button" type="button" :aria-expanded="showBrandMenu" title="账号菜单" aria-label="打开账号菜单" @click="showBrandMenu = !showBrandMenu"><span class="brand-mark">✦</span></button>
        <span>Lumic</span>
        <small>拾光</small>
        <div v-if="showBrandMenu" class="brand-account-popover">
          <button type="button" @click="logout">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M10 5H6.5A2.5 2.5 0 0 0 4 7.5v9A2.5 2.5 0 0 0 6.5 19H10M14 8l4 4-4 4M18 12H9"/></svg>
            <span>退出登录</span>
          </button>
        </div>
      </div>
      <nav class="main-nav">
        <div class="source-nav-group">
          <div class="source-nav-heading">
            <button class="source-nav-main" :class="{ active: activeNav === 'all' }" @click="navigateTo('all', 'all')"><span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${timelineNavIcon})` }" aria-hidden="true"></span> 全部动态</button>
            <button class="source-nav-toggle" type="button" :title="sourcesExpanded ? '收起平台来源' : '展开平台来源'" :aria-expanded="sourcesExpanded" @click="sourcesExpanded = !sourcesExpanded"><svg :class="{ collapsed: !sourcesExpanded }" viewBox="0 0 24 24" aria-hidden="true"><path d="m4 8.5 8 4.3 8-4.3"/></svg></button>
          </div>
          <Transition name="source-nav-collapse">
            <div v-show="sourcesExpanded" class="source-nav-children">
              <button v-for="(meta, key) in sourceMeta" :key="key" :class="{ active: activeNav === 'source' && activeSource === key }" @click="navigateTo('source', key)"><img :class="['sidebar-source-icon', `sidebar-${key}-icon`]" :src="meta.lineImage" :alt="`${meta.label}线条图标`">{{ meta.label }}</button>
            </div>
          </Transition>
        </div>
        <button :class="{ active: activeNav === 'liked' }" @click="navigateTo('liked', 'all')">
<span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${favoriteNavIcon})` }" aria-hidden="true"></span> 收藏
</button>
        <button :class="{ active: activeNav === 'pulls' }" @click="navigateTo('pulls')">
<span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${subscriptionsNavIcon})` }" aria-hidden="true"></span> 订阅平台
</button>
      </nav>
      <div class="sidebar-bottom">
        <button :class="{ active: activeNav === 'settings' }" @click="openSettings()"><span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${settingsNavIcon})` }" aria-hidden="true"></span> 设置</button>
        <button class="sidebar-theme-button" type="button" @click="setDarkMode(!isDark); mobileMenuOpen = false" :title="isDark ? '当前为夜间主题' : '当前为日间主题'" :aria-label="isDark ? '当前为夜间主题，点击切换日间主题' : '当前为日间主题，点击切换夜间主题'"><span class="theme-mask-symbol" :style="{ '--nav-mask': `url(${isDark ? nightThemeIcon : dayThemeIcon})` }" aria-hidden="true"></span></button>
      </div>
    </aside>

    <main v-if="!showSettings && activeNav !== 'pulls' && !(phonePortrait && masonryDetailPost)" :class="['content', { 'liked-page': activeNav === 'liked' }]" @click="closeContextMenu" @contextmenu.prevent="openContextMenu($event)">
      <div v-if="!authorProfile && activeNav !== 'liked'" class="night-sky-decor" aria-hidden="true"><i class="night-haze"></i><i class="night-moon"></i><i class="night-star star-one"></i><i class="night-star star-two"></i><i class="night-star star-three"></i><i class="night-star star-four"></i><i class="night-star star-five"></i><i class="night-star star-six"></i><i class="night-star star-seven"></i><i class="night-star star-eight"></i></div>
      <div v-if="!authorProfile && activeNav !== 'liked'" class="night-meteor-layer" aria-hidden="true"><i v-for="meteor in meteorBurst" :key="meteor.id" class="night-meteor" :style="meteor.style"></i></div>
      <div v-if="!authorProfile && activeNav !== 'liked'" class="seasonal-decor" :class="`season-${localSeason}`" aria-hidden="true">
        <template v-if="localSeason === 'spring'"><i class="spring-branch"></i><i class="spring-stem stem-one"></i><i class="spring-stem stem-two"></i><i class="spring-stem stem-three"></i><i class="spring-leaf spring-leaf-one"></i><i class="spring-leaf spring-leaf-two"></i><i class="spring-leaf spring-leaf-three"></i><i class="spring-blossom blossom-one"></i><i class="spring-blossom blossom-two"></i><i class="spring-blossom blossom-three"></i><i class="spring-blossom blossom-four"></i><i class="spring-petal petal-one"></i><i class="spring-petal petal-two"></i></template>
        <template v-else-if="localSeason === 'summer'"><i class="summer-sun"></i><i class="summer-pond"></i><i class="summer-stem stem-one"></i><i class="summer-stem stem-two"></i><i class="lotus-leaf leaf-one"></i><i class="lotus-leaf leaf-two"></i><i class="summer-lotus"></i><i class="summer-lotus-bud"></i><i class="summer-dragonfly"></i><i class="summer-frog"><span></span></i><i class="summer-fish fish-one"></i><i class="summer-fish fish-two"></i></template>
        <template v-else-if="localSeason === 'autumn'"><i class="autumn-glow"></i><i class="autumn-branch"></i><i class="autumn-leaf leaf-one"></i><i class="autumn-leaf leaf-two"></i><i class="autumn-leaf leaf-three"></i><i class="autumn-leaf leaf-four"></i><i class="autumn-leaf leaf-five"></i></template>
        <template v-else><i class="winter-haze"></i><i class="winter-branch"></i><i class="winter-plum plum-one"></i><i class="winter-plum plum-two"></i><i class="winter-plum plum-three"></i><i class="winter-snowman"></i><i class="winter-ice"></i><i class="winter-snowflake flake-one"></i><i class="winter-snowflake flake-two"></i><i class="winter-snowflake flake-three"></i><i class="winter-snowflake flake-four"></i><i class="winter-snowflake flake-five"></i></template>
      </div>
      <header v-if="authorProfile" class="topbar author-page-header">
        <div class="author-profile-main">
          <button class="author-back-button" type="button" title="返回时间线" aria-label="返回时间线" @click="closeAuthor">←</button>
          <img :src="authorProfile.avatar || sourceIconFor(authorProfile.source)" :alt="authorProfile.name">
          <div><p class="eyebrow">AUTHOR TIMELINE · {{ sourceMeta[authorProfile.source].label }}</p><h1>{{ authorProfile.name }}</h1><p class="subtitle">共 {{ authorProfile.count }} 条已拉取动态</p></div>
        </div>
        <div class="header-actions"><button class="danger-outline-button" :disabled="postActionBusy !== '' || !authorProfile.count" @click="deleteAuthorPosts(authorProfile.source, authorProfile.name)">删除全部动态</button></div>
      </header>
      <header v-else-if="activeNav !== 'liked'" class="topbar timeline-hero">
<div class="timeline-hero-copy">
<p class="eyebrow">SAVED MOMENTS · {{ new Date().toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' }) }}</p>
<h1>{{ selectedTag ? `#${selectedTag}` : `${localGreeting}，拾光者` }}</h1>
<p class="subtitle">{{ selectedTag ? `这里汇总了所有带有 #${selectedTag} 的动态。` : '这里有你关注的世界，和刚刚发生的一切。' }}</p>
</div>
</header>
      <section v-if="!authorProfile" class="stats">
<div class="stat-card">
<div class="stat-icon mint"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="5" width="16" height="14" rx="2"/><path d="M8 9h8M8 13h5"/></svg></div>
<div>
<span>全部动态</span>
<strong>{{ totalStatsCount }}</strong>
</div>
<small>{{ activeSource === 'all' ? '全部平台' : sourceMeta[activeSource]?.label }}</small>
</div>
<div class="stat-card">
<div class="stat-icon sand"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8"/><path d="M12 7v5l3 2"/></svg></div>
<div>
<span>今日动态</span>
<strong>{{ todayStatsCount }}</strong>
</div>
<small>本地日期</small>
</div>
<div class="stat-card">
<div class="stat-icon rose"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 20.5S4.5 16.2 4.5 10.2A4.2 4.2 0 0 1 12 7.6a4.2 4.2 0 0 1 7.5 2.6c0 6-7.5 10.3-7.5 10.3Z"/></svg></div>
<div>
<span>收藏动态</span>
<strong>{{ favoriteStatsCount }}</strong>
</div>
<small>{{ activeSource === 'all' ? '全部平台' : sourceMeta[activeSource]?.label }}</small>
</div>
</section>
      <div class="section-heading">
<div class="filters">
<button v-if="!authorProfile" :class="{ selected: activeSource === 'all' }" @click="activeSource = 'all'"><span>✦</span>全部</button>
<button class="timeline-sort-button" type="button" :title="timelineSort === 'newest' ? '最新' : '最旧'" :aria-label="timelineSort === 'newest' ? '当前按最新排序，点击切换为最旧' : '当前按最旧排序，点击切换为最新'" @click="timelineSort = timelineSort === 'newest' ? 'oldest' : 'newest'">
  <span class="timeline-sort-symbol" :style="{ '--nav-mask': `url(${timelineSort === 'newest' ? newestSortIcon : oldestSortIcon})` }" aria-hidden="true"></span>
</button>
<button class="timeline-view-button" type="button" :title="isMasonryView ? '当前：瀑布流；点击切换为列表' : '当前：列表；点击切换为瀑布流'" :aria-label="isMasonryView ? '当前为瀑布流视图，点击切换为列表' : '当前为列表视图，点击切换为瀑布流'" @click="toggleTimelineView">
  <span :class="['timeline-view-symbol', { 'list-view-symbol': !isMasonryView }]" :style="{ '--nav-mask': `url(${isMasonryView ? masonryViewIcon : listViewIcon})` }" aria-hidden="true"></span>
</button>
</div>
<div class="timeline-tools">
  <label class="timeline-search" @pointerdown.stop="closeMobileNavigationOnInputFocus" @click.stop><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m16 16 5 5"/></svg><input v-model="timelineSearch" type="search" placeholder="搜索内容、作者或标签" aria-label="搜索动态内容、作者或标签" @focus="closeMobileNavigationOnInputFocus" @input="closeMobileNavigationOnInputFocus" @blur="releaseTimelineSearchFocus"></label>
</div>
</div>
      <p v-if="timelineMessage" class="timeline-message">{{ timelineMessage }}</p>
      <section ref="feedListElement" :class="['feed-list', { 'masonry-feed': isMasonryView }]" :style="masonryFeedStyle">
<template v-if="!isMasonryView">
<div v-if="timelineTopSpace" class="timeline-spacer" :style="{ height: `${timelineTopSpace}px` }" aria-hidden="true"></div>
<article v-for="post in visiblePosts" :key="post.id" :ref="element => setPostCard(post, element, 'list')" :class="['post-card', { selected: selectedPostIds.includes(post.id), selectable: selectionMode }]" :data-post-id="post.id" @click.capture="handlePostSelectionClick($event, post)" @contextmenu.stop.prevent="openContextMenu($event, post)">
<label v-if="selectionMode" class="post-select-control" :title="`选择 ${post.author} 的这条动态`" @click.prevent><input type="checkbox" :checked="selectedPostIds.includes(post.id)" tabindex="-1"><span></span></label>
<div class="post-head">
<button class="post-author-avatar" type="button" :title="`查看 ${post.author} 的动态`" @click="openAuthor(post)"><img :src="post.avatar" :alt="post.author"></button>
<div class="author">
<button class="post-author-name" type="button" :title="`查看 ${post.author} 的动态`" @click="openAuthor(post)"><strong>{{ post.author }}</strong></button>
<span>{{ postDateTime(post.published) }}</span>
</div>
<span :class="['source-pill', 'post-source-pill', sourceMeta[post.source].color]">
<img :class="['source-icon', { 'twitter-night-icon': post.source === 'twitter' && isDark }]" :src="sourceIconFor(post.source)" :alt="`${sourceMeta[post.source].label}图标`">{{ sourceMeta[post.source].label }}</span>
</div>
<p v-if="post.caption" class="caption">{{ post.caption }}</p>
<div v-if="post.media?.length" :class="['media-grid', `media-count-${Math.min(post.media.length, 9)}`]">
<button v-for="(media, mediaIndex) in post.media.slice(0, 9)" :key="media" :class="['media-frame', mediaShape(post, mediaIndex)]" type="button" :aria-label="`查看 ${post.author} 的第 ${mediaIndex + 1} 张图片`" @click="openLightbox(post, mediaIndex)"><img :src="previewMedia(media)" alt="" loading="lazy" decoding="async" fetchpriority="low" @load="setMediaShape(post, mediaIndex, $event); scheduleTimelineWindow()"><span v-if="mediaIndex === 8 && post.media.length > 9" class="media-more-count">+{{ post.media.length - 9 }}</span></button>
</div>
<div v-if="primaryVideo(post)" class="post-video-list">
<div :class="['post-video-frame', postVideoFrameClass(post)]" :style="postVideoFrameStyle(post)"><video :src="primaryVideo(post).url" :poster="primaryVideo(post).poster ? previewMedia(primaryVideo(post).poster) : undefined" controls playsinline preload="metadata" @loadedmetadata="setPostVideoRatio(post, $event)"></video></div>
</div>
<div class="post-foot">
<div class="tag-row"><button v-for="tag in post.tags" :key="tag" type="button" :class="{ active: selectedTag === tag }" @click="openTag(tag)">#{{ tag }}</button></div>
<div class="post-foot-actions">
<button :class="['post-like-button', { liked: post.liked }]" :disabled="postActionBusy === `like:${post.id}`" :title="post.liked ? '取消收藏' : '收藏'" :aria-label="post.liked ? '取消收藏' : '收藏'" @click="togglePostLike(post)"><span class="post-action-mask post-favorite-symbol" :style="{ '--post-action-mask': `url(${favoriteNavIcon})` }" aria-hidden="true"></span></button>
<button class="post-visit-button" :disabled="!post.originalUrl" :title="post.originalUrl ? '访问原动态' : '旧动态暂无原始链接，请重新拉取'" aria-label="访问原动态" @click="openOriginalPost(post)"><span class="post-action-mask post-visit-symbol" :style="{ '--post-action-mask': `url(${visitPostIcon})` }" aria-hidden="true"></span></button>
</div>
</div>
</article>
<div v-if="timelineBottomSpace" class="timeline-spacer" :style="{ height: `${timelineBottomSpace}px` }" aria-hidden="true"></div>
</template>
<template v-else>
<article v-for="item in visibleMasonryItems" :key="item.post.id" :ref="element => setPostCard(item.post, element, 'masonry')" :class="['masonry-card', { selected: selectedPostIds.includes(item.post.id), selectable: selectionMode, 'text-only': !masonryCover(item.post) && !item.post.videos?.length }]" :style="masonryItemStyle(item)" :data-post-id="item.post.id" tabindex="0" @click.capture="handlePostSelectionClick($event, item.post)" @click="openMasonryPost(item.post)" @keydown.enter.prevent="openMasonryPost(item.post)" @contextmenu.stop.prevent="openContextMenu($event, item.post)">
  <label v-if="selectionMode" class="post-select-control masonry-select-control" :title="`选择 ${item.post.author} 的这条动态`" @click.prevent><input type="checkbox" :checked="selectedPostIds.includes(item.post.id)" tabindex="-1"><span></span></label>
  <div v-if="masonryCover(item.post)" class="masonry-cover">
    <img :src="previewMedia(masonryCover(item.post))" alt="" loading="lazy" decoding="async" fetchpriority="low" @load="setMasonryCoverRatio(item.post, $event, masonryCoverIsVideo(item.post))">
    <span v-if="item.post.media?.length > 1" class="masonry-media-count">1 / {{ item.post.media.length }}</span>
    <span v-if="masonryPostHasVideo(item.post)" class="masonry-video-mark" aria-label="视频动态"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9 7 8 5-8 5Z"/></svg></span>
  </div>
  <div v-else-if="primaryVideo(item.post)" class="masonry-cover masonry-video-cover">
    <video :src="primaryVideo(item.post).url" muted playsinline preload="metadata" @loadedmetadata="setMasonryCoverRatio(item.post, $event, true)"></video>
    <span class="masonry-video-mark" aria-label="视频动态"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9 7 8 5-8 5Z"/></svg></span>
  </div>
  <div class="masonry-card-body">
    <p v-if="item.post.caption" class="masonry-caption">{{ item.post.caption }}</p>
    <div v-if="item.post.tags?.length" class="masonry-tags"><button v-for="tag in item.post.tags.slice(0, 2)" :key="tag" type="button" @click.stop="openTag(tag)">#{{ tag }}</button><span v-if="item.post.tags.length > 2">+{{ item.post.tags.length - 2 }}</span></div>
    <footer class="masonry-meta">
      <button class="masonry-author" type="button" :title="`查看 ${item.post.author} 的动态`" @click.stop="openAuthor(item.post)"><img :src="item.post.avatar" :alt="item.post.author"><span><strong>{{ item.post.author }}</strong><small>{{ postDateTime(item.post.published) }}</small></span></button>
      <button :class="['masonry-like-button', { liked: item.post.liked }]" type="button" :disabled="postActionBusy === `like:${item.post.id}`" :title="item.post.liked ? '取消收藏' : '收藏'" @click.stop="togglePostLike(item.post)"><span class="post-action-mask post-favorite-symbol" :style="{ '--post-action-mask': `url(${favoriteNavIcon})` }" aria-hidden="true"></span></button>
    </footer>
  </div>
</article>
</template>
<div v-if="!filteredPosts.length" class="empty">{{ authorProfile ? '还没有拉取到这个作者的动态' : activeNav === 'liked' ? '还没有收藏的动态' : '还没有这个来源的动态' }}</div>
</section>
    </main>
    <main v-if="!showSettings && activeNav === 'pulls'" class="content pulls-page">
      <p v-if="timelineMessage" class="timeline-message">{{ timelineMessage }}</p>
      <section class="subscription-platforms">
        <div class="section-heading"><div><h2>平台来源</h2><p>查看账号连接状态与订阅数量。</p></div><div class="platform-heading-actions"><span>{{ platformCards.filter(platform => platform.configured).length }} / 4 已连接</span><button class="sync-latest-icon" type="button" :disabled="syncing" title="全部拉取最新动态" aria-label="全部拉取最新动态" @click="runFullSync"><svg :class="{ spin: syncing }" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 9a7 7 0 0 1 11.4-2.5L20 9M4 15l2.5 2.5A7 7 0 0 0 17.9 15"/></svg></button></div></div>
        <div class="platform-source-grid">
          <article v-for="platform in platformCards" :key="platform.key" :class="['platform-source-card', platform.key]" @contextmenu.prevent="openPlatformSettings(platform)">
            <div class="platform-card-head"><img :class="['source-icon', { 'twitter-night-icon': platform.key === 'twitter' && isDark }]" :src="platform.image" :alt="`${platform.label}图标`"><span :class="['connection-dot', { online: platform.configured }]">{{ platform.configured ? '已连接' : '未连接' }}</span></div>
            <h4>{{ platform.label }}</h4>
            <dl><div><dt>账号</dt><dd>{{ platform.account }}</dd></div><div><dt>订阅来源</dt><dd>{{ platform.feeds.length }} 个</dd></div></dl>
            <button @click="openPlatformSettings(platform)">管理平台 <span>→</span></button>
          </article>
        </div>
      </section>
      <div class="section-heading subscription-list-heading"><div><h2>订阅作者</h2><p>查看同步状态并手动拉取内容。</p></div><span>{{ feeds.length }} 个来源</span></div>
      <section class="pull-list">
        <article v-for="feed in feeds" :key="feed.id" class="pull-card">
          <img :key="`${feed.id}:${sourceAvatar(feed, platformCardForSource(feed.source))}`" class="pull-avatar" :src="sourceAvatar(feed, platformCardForSource(feed.source))" data-fallback-index="0" :alt="feed.name" referrerpolicy="no-referrer" @error="handleSourceAvatarError($event, feed, platformCardForSource(feed.source))">
          <div class="pull-info"><div class="pull-title"><strong>{{ feed.name }}</strong><span :class="['source-pill', sourceMeta[feed.source].color]"><img :class="['source-icon', { 'twitter-night-icon': feed.source === 'twitter' && isDark }]" :src="sourceIconFor(feed.source)" :alt="sourceMeta[feed.source].label">{{ sourceMeta[feed.source].label }}</span></div><span class="pull-handle">{{ feed.handle }}</span><small>{{ feed.lastSyncMessage || (feed.lastSyncedAt ? `上次拉取：${relativeTime(feed.lastSyncedAt)}` : '尚未拉取') }}</small></div>
          <div class="pull-status"><i :class="['pull-dot', feed.lastSyncStatus]"></i><span>{{ feed.lastSyncStatus === 'success' ? `新增 ${feed.lastSyncCount || 0} 条` : feed.lastSyncStatus === 'failed' ? '拉取失败' : '待拉取' }}</span></div>
          <div class="pull-actions"><button class="pull-action" :disabled="sourceActionBusy !== ''" :title="sourceActionBusy === `sync:${feed.id}` ? '正在同步' : '立即同步'" @click="syncSource(feed)"><svg :class="{ spin: sourceActionBusy === `sync:${feed.id}` }" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 9a7 7 0 0 1 11.4-2.5L20 9M4 15l2.5 2.5A7 7 0 0 0 17.9 15"/></svg><span>{{ sourceActionBusy === `sync:${feed.id}` ? '同步中' : '立即同步' }}</span></button></div>
        </article>
        <div v-if="!feeds.length" class="empty">还没有订阅作者，请先添加 UP 主或微博博主。</div>
      </section>
    </main>
    <main v-if="phonePortrait && masonryDetailPost" class="content mobile-post-detail-page">
      <header class="mobile-post-detail-head">
        <button class="mobile-post-back" type="button" title="返回动态页" aria-label="返回动态页" @click="closePostDetail"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m15 5-7 7 7 7"/></svg></button>
        <button class="mobile-post-author" type="button" @click="openAuthor(masonryDetailPost)"><img :src="masonryDetailPost.avatar" :alt="masonryDetailPost.author"><strong>{{ masonryDetailPost.author }}</strong></button>
        <span :class="['source-pill', 'mobile-post-source', sourceMeta[masonryDetailPost.source].color]"><img :class="['source-icon', { 'twitter-night-icon': masonryDetailPost.source === 'twitter' && isDark }]" :src="sourceIconFor(masonryDetailPost.source)" :alt="`${sourceMeta[masonryDetailPost.source].label}图标`">{{ sourceMeta[masonryDetailPost.source].label }}</span>
      </header>
      <section v-if="mobileDetailCurrentMedia" :class="['mobile-post-media-stage', { 'video-media': mobileDetailCurrentMedia.type === 'video' }, mobileDetailCurrentMedia.type === 'video' ? postVideoFrameClass(masonryDetailPost) : '']" :style="mobileDetailCurrentMedia.type === 'video' ? postVideoFrameStyle(masonryDetailPost) : undefined" @touchstart.passive="beginMobileDetailSwipe" @touchend.passive="finishMobileDetailSwipe">
        <img v-if="mobileDetailCurrentMedia.type === 'image'" :key="`${mobileDetailCurrentMedia.key}:${mobileDetailMotion}`" :class="`mobile-detail-motion-${mobileDetailMotion}`" :src="mobileDetailCurrentMedia.src" :alt="`${masonryDetailPost.author} 的第 ${mobileDetailIndex + 1} 张图片`" @click="openMobileDetailImage">
        <video v-else :key="`${mobileDetailCurrentMedia.key}:${mobileDetailMotion}`" :class="`mobile-detail-motion-${mobileDetailMotion}`" :src="mobileDetailCurrentMedia.src" :poster="mobileDetailCurrentMedia.poster || undefined" controls playsinline autoplay muted preload="auto" @loadedmetadata="setPostVideoRatio(masonryDetailPost, $event)"></video>
        <div v-if="mobileDetailMedia.length > 1" class="mobile-post-media-dots" aria-label="媒体分页"><button v-for="(media, index) in mobileDetailMedia" :key="media.key" type="button" :class="{ active: index === mobileDetailIndex }" :aria-label="`查看第 ${index + 1} 项媒体`" @click="selectMobileDetailMedia(index)"></button></div>
      </section>
      <section class="mobile-post-copy">
        <p v-if="masonryDetailPost.caption" class="caption">{{ masonryDetailPost.caption }}</p>
        <div v-if="masonryDetailPost.tags?.length" class="tag-row"><button v-for="tag in masonryDetailPost.tags" :key="tag" type="button" @click="openTag(tag)">#{{ tag }}</button></div>
      </section>
      <footer class="mobile-post-detail-foot">
        <time :datetime="masonryDetailPost.published">{{ postDateTime(masonryDetailPost.published) }}</time>
        <div class="mobile-post-detail-actions">
          <button :class="['post-like-button', { liked: masonryDetailPost.liked }]" type="button" :disabled="postActionBusy === `like:${masonryDetailPost.id}`" :title="masonryDetailPost.liked ? '取消收藏' : '收藏'" :aria-label="masonryDetailPost.liked ? '取消收藏' : '收藏'" @click="togglePostLike(masonryDetailPost)"><span class="post-action-mask post-favorite-symbol" :style="{ '--post-action-mask': `url(${favoriteNavIcon})` }" aria-hidden="true"></span></button>
          <button class="post-visit-button" type="button" :disabled="!masonryDetailPost.originalUrl" title="访问原动态" aria-label="访问原动态" @click="openOriginalPost(masonryDetailPost)"><span class="post-action-mask post-visit-symbol" :style="{ '--post-action-mask': `url(${visitPostIcon})` }" aria-hidden="true"></span></button>
        </div>
      </footer>
    </main>
    <div v-if="masonryDetailPost && !phonePortrait" class="modal-backdrop masonry-detail-backdrop" @click.self="closePostDetail">
      <article class="masonry-detail-modal" role="dialog" aria-modal="true" :aria-label="`${masonryDetailPost.author} 的动态详情`">
        <div class="post-head">
          <button class="post-author-avatar" type="button" :title="`查看 ${masonryDetailPost.author} 的动态`" @click="openAuthor(masonryDetailPost)"><img :src="masonryDetailPost.avatar" :alt="masonryDetailPost.author"></button>
          <div class="author"><button class="post-author-name" type="button" @click="openAuthor(masonryDetailPost)"><strong>{{ masonryDetailPost.author }}</strong></button><span>{{ postDateTime(masonryDetailPost.published) }}</span></div>
          <span :class="['source-pill', 'post-source-pill', sourceMeta[masonryDetailPost.source].color]"><img :class="['source-icon', { 'twitter-night-icon': masonryDetailPost.source === 'twitter' && isDark }]" :src="sourceIconFor(masonryDetailPost.source)" :alt="`${sourceMeta[masonryDetailPost.source].label}图标`">{{ sourceMeta[masonryDetailPost.source].label }}</span>
        </div>
        <p v-if="masonryDetailPost.caption" class="caption">{{ masonryDetailPost.caption }}</p>
        <div v-if="masonryDetailPost.media?.length" :class="['media-grid', 'masonry-detail-media-grid', `media-count-${Math.min(masonryDetailPost.media.length, 4)}`]">
          <button v-for="(media, mediaIndex) in masonryDetailPost.media.slice(0, 4)" :key="media" :class="['media-frame', mediaShape(masonryDetailPost, mediaIndex)]" type="button" :aria-label="`查看第 ${mediaIndex + 1} 张图片`" @click="openLightbox(masonryDetailPost, mediaIndex)"><img :src="previewMedia(media)" alt="" loading="lazy" decoding="async" @load="setMediaShape(masonryDetailPost, mediaIndex, $event)"><span v-if="mediaIndex === 3 && masonryDetailPost.media.length > 4" class="media-more-count">+{{ masonryDetailPost.media.length - 4 }}</span></button>
        </div>
        <div v-if="primaryVideo(masonryDetailPost)" class="post-video-list"><div :class="['post-video-frame', postVideoFrameClass(masonryDetailPost)]" :style="postVideoFrameStyle(masonryDetailPost)"><video :src="primaryVideo(masonryDetailPost).url" :poster="primaryVideo(masonryDetailPost).poster ? previewMedia(primaryVideo(masonryDetailPost).poster) : undefined" controls playsinline autoplay muted preload="auto" @loadedmetadata="setPostVideoRatio(masonryDetailPost, $event)"></video></div></div>
        <div class="post-foot masonry-detail-foot"><div class="tag-row"><button v-for="tag in masonryDetailPost.tags" :key="tag" type="button" @click="openTag(tag)">#{{ tag }}</button></div><div class="post-foot-actions"><button :class="['post-like-button', { liked: masonryDetailPost.liked }]" :title="masonryDetailPost.liked ? '取消收藏' : '收藏'" @click="togglePostLike(masonryDetailPost)"><span class="post-action-mask post-favorite-symbol" :style="{ '--post-action-mask': `url(${favoriteNavIcon})` }" aria-hidden="true"></span></button><button class="post-visit-button" :disabled="!masonryDetailPost.originalUrl" title="访问原动态" @click="openOriginalPost(masonryDetailPost)"><span class="post-action-mask post-visit-symbol" :style="{ '--post-action-mask': `url(${visitPostIcon})` }" aria-hidden="true"></span></button></div></div>
      </article>
    </div>
    <div v-if="contextMenu.open" class="timeline-context-menu" :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }" role="menu" @click.stop>
      <button type="button" role="menuitem" @click="startMultiSelectMode"><svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="3"/><path d="m8 12 2.3 2.3L16 8.7"/></svg><span>{{ activeNav === 'liked' ? '多选取消收藏' : '多选删除' }}</span></button>
      <button v-if="contextMenu.post && activeNav === 'liked'" type="button" class="danger" role="menuitem" @click="unfavoriteContextPost"><svg viewBox="0 0 24 24"><path d="M12 20.5S4.5 16.2 4.5 10.2A4.2 4.2 0 0 1 12 7.6a4.2 4.2 0 0 1 7.5 2.6c0 6-7.5 10.3-7.5 10.3Z"/><path d="M8.5 11.5h7"/></svg><span>取消收藏</span></button>
      <button v-else-if="contextMenu.post" type="button" class="danger" role="menuitem" @click="deleteContextPost"><svg viewBox="0 0 24 24"><path d="M4 7h16M9 7V4h6v3M7 7l1 13h8l1-13M10 11v5M14 11v5"/></svg><span>删除这条动态</span></button>
    </div>
    <div v-if="selectionMode" class="selection-dock">
      <button type="button" class="selection-cancel-button" title="取消多选" aria-label="取消多选" @click="stopSelection"><svg viewBox="0 0 24 24"><path d="m6 6 12 12M18 6 6 18"/></svg></button>
      <button v-if="selectionAction === 'unfavorite'" type="button" class="selection-select-all-button" :title="allFilteredPostsSelected ? '取消全选' : '全选'" :aria-label="allFilteredPostsSelected ? '取消全选' : '全选'" @click="toggleSelectAllPosts"><svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="3"/><path v-if="allFilteredPostsSelected" d="M8 12h8"/><path v-else d="m8 12 2.3 2.3L16 8.7"/></svg><b>{{ allFilteredPostsSelected ? '取消全选' : '全选' }}</b></button>
      <span>已选择 {{ selectedPostCount }} 条</span>
      <button v-if="selectionAction === 'unfavorite'" type="button" class="selection-delete-button selection-unfavorite-button" :disabled="!selectedPostCount || postActionBusy === 'batch-unfavorite'" title="取消收藏所选动态" aria-label="取消收藏所选动态" @click="unfavoriteSelectedPosts"><svg viewBox="0 0 24 24"><path d="M12 20.5S4.5 16.2 4.5 10.2A4.2 4.2 0 0 1 12 7.6a4.2 4.2 0 0 1 7.5 2.6c0 6-7.5 10.3-7.5 10.3Z"/><path d="M8.5 11.5h7"/></svg><b>取消收藏</b></button>
      <button v-else type="button" class="selection-delete-button" :disabled="!selectedPostCount || postActionBusy === 'batch-delete'" title="删除所选动态" aria-label="删除所选动态" @click="deleteSelectedPosts"><svg viewBox="0 0 24 24"><path d="M4 7h16M9 7V4h6v3M7 7l1 13h8l1-13M10 11v5M14 11v5"/></svg><b>删除</b></button>
    </div>
    <div v-if="lightbox.open" class="lightbox-layer" role="dialog" aria-modal="true" :aria-label="`${lightbox.author} 的动态媒体`" @click.self="closeLightbox" @wheel.prevent="zoomLightbox">
      <button class="lightbox-close" type="button" title="关闭大图" aria-label="关闭大图" @click="closeLightbox">×</button>
      <figure @click.self="closeLightbox">
        <div :key="`${lightbox.media[lightbox.index]}:${lightbox.motion}`" :class="['lightbox-image-stage', `motion-${lightbox.motion}`, { 'mobile-original-size': phonePortrait && lightboxAtOriginalSize }]" @click.self="closeLightbox" @contextmenu.prevent>
          <img ref="lightboxImageElement" :src="lightbox.media[lightbox.index]" :alt="`${lightbox.author} 的动态图片 ${lightbox.index + 1}`" :class="{ dragging: lightbox.dragging, 'original-size': !lightbox.fit }" :style="{ transform: `translate3d(${lightbox.x}px, ${lightbox.y}px, 0) rotate(${lightbox.rotation}deg) scale(${lightbox.scale})` }" draggable="false" @click.stop @pointerdown.prevent.stop="startLightboxGesture" @pointermove.prevent.stop="moveLightboxGesture" @pointerup.prevent.stop="stopLightboxGesture" @pointercancel.prevent.stop="stopLightboxGesture" @load="scheduleLightboxScaleUpdate">
        </div>
      </figure>
      <div v-if="!phonePortrait" class="lightbox-dock-zone" @pointerenter="showLightboxDock(false)" @pointermove="showLightboxDock(false)" @pointerleave="scheduleLightboxDockHide(650)">
      <div :class="['lightbox-dock', { hidden: !lightboxDockVisible }]" role="toolbar" aria-label="图片查看工具">
        <button type="button" title="上一张" aria-label="上一张" :disabled="lightbox.media.length < 2" @click="moveLightbox(-1)"><svg viewBox="0 0 24 24"><path d="m15 18-6-6 6-6"/></svg></button>
        <span class="lightbox-counter">{{ lightbox.index + 1 }}/{{ lightbox.media.length }}</span>
        <button type="button" title="下一张" aria-label="下一张" :disabled="lightbox.media.length < 2" @click="moveLightbox(1)"><svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg></button>
        <i></i>
        <button type="button" title="缩小" aria-label="缩小" @click="zoomLightboxBy(-0.15)"><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="M8 11h6M16 16l5 5"/></svg></button>
        <span class="lightbox-scale">{{ lightboxScalePercent }}%</span>
        <button type="button" title="放大" aria-label="放大" @click="zoomLightboxBy(0.15)"><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="M8 11h6M11 8v6M16 16l5 5"/></svg></button>
        <button type="button" :title="lightbox.fit ? '原始尺寸' : '适应页面'" :aria-label="lightbox.fit ? '原始尺寸' : '适应页面'" :class="{ active: !lightbox.fit }" @click="toggleLightboxFit">
          <span v-if="lightbox.fit" class="lightbox-tool-mask lightbox-original-size-symbol" :style="{ '--lightbox-tool-mask': `url(${originalSizeIcon})` }" aria-hidden="true"></span>
          <svg v-else class="lightbox-fit-symbol" viewBox="0 0 24 24" aria-hidden="true"><path d="M8 4H6a2 2 0 0 0-2 2v2M16 4h2a2 2 0 0 1 2 2v2M8 20H6a2 2 0 0 1-2-2v-2M16 20h2a2 2 0 0 0 2-2v-2"/><rect x="8" y="8" width="8" height="8" rx="2.2"/></svg>
        </button>
        <i></i>
        <button type="button" title="顺时针旋转" aria-label="顺时针旋转" @click="rotateLightbox"><span class="lightbox-tool-mask lightbox-rotate-symbol" :style="{ '--lightbox-tool-mask': `url(${rotateLightboxIcon})` }" aria-hidden="true"></span></button>
        <button type="button" title="下载原图" aria-label="下载原图" @click="downloadLightboxImage"><span class="lightbox-tool-mask lightbox-download-symbol" :style="{ '--lightbox-tool-mask': `url(${downloadLightboxIcon})` }" aria-hidden="true"></span></button>
      </div>
      </div>
      <div v-if="phonePortrait && mobileLightboxMenu.open" class="mobile-lightbox-menu" role="menu" aria-label="图片操作" @pointerdown.stop @click.stop>
        <button type="button" role="menuitem" @click="saveMobileLightboxImage"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12M7.5 10.5 12 15l4.5-4.5M5 19v2h14v-2"/></svg><span>保存图片</span></button>
      </div>
    </div>
    <button v-if="!showSettings && activeNav === 'pulls'" class="add-fab" @click="showAdd = true">＋ <span>添加来源</span>
</button>
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
<div class="modal">
<button class="modal-close" @click="showAdd = false">×</button>
<p class="eyebrow">NEW SOURCE</p>
<h2>添加一个新来源</h2>
<p>连接账号后，Lumic 会帮你把喜欢的内容收进同一条时间线。</p>
<div class="connect-options">
<button @click="openBilibili">
<img class="source-icon" :src="sourceMeta.bilibili.image" alt="哔哩哔哩图标">连接哔哩哔哩</button>
<button @click="openWeibo">
<img class="source-icon" :src="sourceMeta.weibo.image" alt="微博图标">添加微博博主</button>
<button @click="openPixiv">
<img class="source-icon" :src="sourceMeta.pixiv.image" alt="pixiv图标">添加 Pixiv 画师</button>
<button @click="showAdd = false; openSettings('platforms')">
<img :class="['source-icon', { 'twitter-night-icon': isDark }]" :src="sourceIconFor('twitter')" alt="推特图标">连接推特</button>
</div>
</div>
</div>
    <div v-if="showBilibili" class="modal-backdrop" @click.self="showBilibili = false">
      <div class="modal bili-modal">
        <button class="modal-close" @click="showBilibili = false">×</button>
        <p class="eyebrow">BILIBILI SOURCE</p>
        <h2>订阅 UP 主图文</h2>
        <p>仅添加订阅，不会立即拉取动态。历史内容可在订阅平台页面手动获取；专栏可在来源设置中单独开启。</p>
        <div class="bili-account"><span>已连接 B 站账号 · UID {{ biliAccount.userId }}</span><button @click="showBilibili = false; openSettings('platforms')">管理凭证</button></div>
        <form class="bili-search" @submit.prevent="searchBilibili"><input v-model="biliKeyword" placeholder="搜索 UP 主昵称" maxlength="40" required><button :disabled="biliBusy">⌕ 搜索</button></form>
        <label class="subscription-tag-field"><span>作者标签</span><input v-model="biliSubscriptionTags" placeholder="#标签1 #标签2" maxlength="120"><small>订阅搜索结果中的作者时，会同时保存这些标签。</small></label>
        <div class="bili-results">
          <article v-for="user in biliResults" :key="user.userId"><img :src="user.avatar" :alt="user.name"><div><strong>{{ user.name }}</strong><span>UID {{ user.userId }} · 粉丝 {{ formatFans(user.fans) }}</span><p>{{ user.description || '这位 UP 主还没有填写简介' }}</p></div><button @click="subscribeBilibili(user)" :disabled="biliBusy">订阅</button></article>
          <div v-if="!biliResults.length" class="bili-placeholder">输入昵称搜索 UP 主，然后选择订阅</div>
        </div>
        <p v-if="biliError" class="login-error bili-error">{{ biliError }}</p>
      </div>
    </div>
    <div v-if="showWeibo" class="modal-backdrop" @click.self="showWeibo = false">
      <div class="modal bili-modal">
        <button class="modal-close" @click="showWeibo = false">×</button>
        <p class="eyebrow">WEIBO SOURCE</p>
        <h2>添加微博博主</h2>
        <p>搜索并添加微博博主，不会立即拉取动态。历史内容可在订阅平台页面手动获取。</p>
        <div class="bili-account"><span>已连接微博账号 · {{ weiboAccount.userName || `UID ${weiboAccount.userId}` }}</span><button @click="showWeibo = false; openSettings('platforms')">管理凭证</button></div>
        <form class="bili-search" @submit.prevent="searchWeibo"><input v-model="weiboKeyword" placeholder="搜索微博博主昵称" maxlength="40" required><button :disabled="weiboBusy">⌕ 搜索</button></form>
        <label class="subscription-tag-field"><span>作者标签</span><input v-model="weiboSubscriptionTags" placeholder="#标签1 #标签2" maxlength="120"><small>订阅搜索结果中的作者时，会同时保存这些标签。</small></label>
        <div class="bili-results">
          <article v-for="user in weiboResults" :key="user.userId"><img :src="user.avatar" :alt="user.name"><div><strong>{{ user.name }}</strong><span>UID {{ user.userId }} · 粉丝 {{ formatFans(user.fans) }}</span><p>{{ user.description || '这位博主还没有填写简介' }}</p></div><button @click="subscribeWeibo(user)" :disabled="weiboBusy">订阅</button></article>
          <div v-if="!weiboResults.length" class="bili-placeholder">输入昵称搜索微博博主，然后选择订阅</div>
        </div>
        <p v-if="weiboError" class="login-error bili-error">{{ weiboError }}</p>
      </div>
    </div>
    <main v-if="showSettings" class="settings-page">
      <div class="settings-page-inner">
        <section class="settings-pane platform-credentials-pane">
          <div class="pane-heading"><div><h3>平台凭证</h3><p>各平台凭证仅由服务端验证，并加密保存在本地数据目录。</p></div><span>4 个平台</span></div>
          <div class="platform-auth-grid">
            <article v-for="platform in platformCards" :key="platform.key" :class="['platform-auth-card', platform.key]" tabindex="0" @contextmenu.prevent="openCredentialSettings(platform.key)" @click="handleCredentialCardClick(platform.key)" @keydown.enter="openCredentialSettings(platform.key)">
              <header class="platform-auth-head"><img :class="['source-icon', { 'twitter-night-icon': platform.key === 'twitter' && isDark }]" :src="platform.image" :alt="`${platform.label}图标`"><div><h3>{{ platform.label }}</h3><span>{{ platform.key === 'twitter' ? '账号连接与作者采集' : '平台账号凭证' }}</span></div><em :class="['connection-dot', { online: platform.configured }]">{{ platform.key === 'twitter' ? '未开放' : platform.configured ? '已连接' : '未连接' }}</em></header>
              <div class="platform-account-summary"><img :src="platform.avatar || platform.image" :alt="`${platform.account}头像`" @error="$event.target.src = platform.image"><div><span>接入账号</span><strong>{{ platform.account }}</strong></div></div>
            </article>
          </div>
        </section>
        <div class="settings-window-grid">
          <section class="settings-pane compact-settings-pane">
            <div class="pane-heading"><div><h3>网络代理</h3><p>配置后端访问外部平台时使用的代理。</p></div><span>{{ proxyStatus.proxyEnabled ? '已启用' : '未启用' }}</span></div>
            <div v-if="proxyStatus.proxyEnabled" class="setting-status">当前代理：{{ proxyStatus.proxyUrl }}</div>
            <form class="settings-form" @submit.prevent="saveProxy" autocomplete="off"><label>代理地址</label><input v-model="proxyForm.proxyUrl" placeholder="socks5://host.docker.internal:7890"><div class="form-actions"><button type="button" class="secondary-button" @click="testProxy" :disabled="settingsBusy">测试</button><button class="login-button" :disabled="settingsBusy">保存代理</button><button type="button" class="danger-link" @click="proxyForm.proxyUrl = ''; saveProxy()">关闭</button></div></form>
            <p v-if="proxyMessage" class="success-message">{{ proxyMessage }}</p>
          </section>
          <section class="settings-pane backup-settings-pane">
            <div class="pane-heading"><div><h3>备份配置</h3><p>导出或恢复登录、平台、代理和订阅配置。</p></div><span>JSON</span></div>
            <div class="backup-action-grid">
              <article><h4>下载备份</h4><p>备份中包含账号凭证，请妥善保管。</p><button class="secondary-button" type="button" :disabled="settingsBusy" @click="downloadConfigurationBackup">下载备份</button></article>
              <article><h4>恢复配置</h4><p>覆盖当前配置，不改变动态及图片文件。</p><label class="restore-file-button" :class="{ disabled: settingsBusy }"><input type="file" accept="application/json,.json" :disabled="settingsBusy" @change="restoreConfiguration"><span>选择备份文件</span></label></article>
            </div>
          </section>
          <section class="settings-pane compact-settings-pane"><div class="pane-heading"><div><h3>账号管理</h3><p>更新 Lumic 本地管理账号和密码。</p></div><span>密码至少 8 位</span></div><form class="settings-form" @submit.prevent="saveSettings" autocomplete="off"><label>新账号</label><input v-model="settingsForm.username" required minlength="3" autocomplete="off"><label>当前密码</label><input v-model="settingsForm.currentPassword" type="password" required autocomplete="current-password"><label>新密码</label><input v-model="settingsForm.newPassword" type="password" required minlength="8" autocomplete="new-password"><button class="login-button" type="submit" :disabled="settingsBusy">{{ settingsBusy ? '保存中…' : '保存账号' }}</button></form></section>
        </div>
        <p v-if="settingsError" class="login-error settings-page-error">{{ settingsError }}</p>
      </div>
    </main>
    <div v-if="credentialPlatform" class="modal-backdrop credential-modal-backdrop" @click.self="credentialPlatform = null">
      <div class="modal credential-settings-modal">
        <button class="modal-close" type="button" aria-label="关闭平台配置" @click="credentialPlatform = null">×</button>
        <div class="credential-modal-head">
          <img :class="['source-icon', { 'twitter-night-icon': credentialPlatform.key === 'twitter' && isDark }]" :src="credentialPlatform.image" :alt="`${credentialPlatform.label}图标`">
          <div><p class="eyebrow">PLATFORM CREDENTIAL</p><h2>{{ credentialPlatform.label }}</h2><span>{{ credentialPlatform.account }}</span></div>
          <em :class="['connection-dot', { online: credentialPlatform.configured }]">{{ credentialPlatform.key === 'twitter' ? '未开放' : credentialPlatform.configured ? '已连接' : '未连接' }}</em>
        </div>

        <div v-if="credentialPlatform.key === 'bilibili'" class="credential-config-body">
          <div v-if="biliQRImage" class="weibo-qr"><img :src="biliQRImage" alt="哔哩哔哩登录二维码"><span>{{ biliQRStatus }}</span></div>
          <button class="login-button platform-login-button" type="button" @click="startBilibiliQR" :disabled="biliBusy && !biliQR">{{ biliQR ? '刷新二维码' : biliBusy ? '获取中…' : biliAccount.configured ? '扫码切换 B 站账号' : '扫码连接 B 站' }}</button>
          <details class="manual-credential"><summary>手动导入 Cookie</summary><form class="settings-form bili-credentials" @submit.prevent="saveBilibiliAccount" autocomplete="off"><label>完整 Cookie</label><textarea v-model="biliCredentials.cookie" rows="4" placeholder="仅在扫码不可用时使用"></textarea><label>SESSDATA</label><input v-model="biliCredentials.SESSDATA" type="password"><label>bili_jct</label><input v-model="biliCredentials.bili_jct" type="password"><label>buvid3</label><input v-model="biliCredentials.buvid3" type="password"><label>DedeUserID</label><input v-model="biliCredentials.DedeUserID" inputmode="numeric"><button class="login-button" :disabled="biliBusy">验证并保存手动凭证</button></form></details>
          <p v-if="biliError" class="login-error bili-error">{{ biliError }}</p>
        </div>

        <div v-else-if="credentialPlatform.key === 'weibo'" class="credential-config-body">
          <details class="platform-auth-details"><summary>账号密码登录</summary><form class="settings-form platform-auth-form weibo-password-form" @submit.prevent="loginWeiboAccount" autocomplete="off"><label>微博账号</label><input v-model="weiboPasswordCredentials.username" type="text" autocomplete="username" required placeholder="手机号、邮箱或微博账号"><label>微博密码</label><input v-model="weiboPasswordCredentials.password" type="password" autocomplete="current-password" required><button class="login-button" :disabled="weiboBusy">{{ weiboBusy ? '登录中…' : '账号密码登录' }}</button></form></details>
          <div v-if="weiboQR" class="weibo-qr"><img :src="weiboQR.image.startsWith('//') ? `https:${weiboQR.image}` : weiboQR.image" alt="微博登录二维码"><span>请在二维码过期前扫码并确认</span></div>
          <button class="login-button platform-login-button" type="button" @click="startWeiboQR" :disabled="weiboBusy && !weiboQR">{{ weiboQR ? '刷新二维码' : weiboBusy ? '获取中…' : weiboAccount.configured ? '扫码切换微博账号' : '扫码连接微博' }}</button>
          <details class="manual-credential"><summary>手动导入 Cookie</summary><form class="settings-form bili-credentials" @submit.prevent="saveWeiboAccount" autocomplete="off"><label>微博 UID</label><input v-model="weiboCredentials.userId" inputmode="numeric" required placeholder="个人主页地址中的数字 UID"><label>完整 Cookie</label><textarea v-model="weiboCredentials.cookie" rows="4" required placeholder="可粘贴浏览器请求头中的 Cookie: 完整内容"></textarea><p class="credential-note">保存前会验证账号资料，不会回显原始 Cookie。</p><button class="login-button" :disabled="weiboBusy">{{ weiboBusy ? '验证中…' : '验证并保存 Cookie' }}</button></form></details>
          <p v-if="weiboError" class="login-error">{{ weiboError }}</p>
        </div>

        <div v-else-if="credentialPlatform.key === 'pixiv'" class="credential-config-body">
          <form class="settings-form platform-auth-form pixiv-browser-form" @submit.prevent="savePixivAccount">
            <label class="credential-field"><span>浏览器 UA</span><input v-model="pixivCredentials.userAgent" required autocomplete="off" placeholder="Mozilla/5.0 ..."></label>
            <label class="credential-field"><span>用户 ID</span><input v-model="pixivCredentials.userId" required inputmode="numeric" autocomplete="off"></label>
            <label class="credential-field"><span>浏览器 Baggage</span><textarea v-model="pixivCredentials.baggage" rows="2" autocomplete="off"></textarea></label>
            <label class="credential-field"><span>浏览器 Cookie</span><textarea v-model="pixivCredentials.cookie" rows="2" required autocomplete="off"></textarea></label>
            <label class="credential-field"><span>浏览器 Sentry-Trace</span><input v-model="pixivCredentials.sentryTrace" autocomplete="off"></label>
            <label class="credential-field"><span>浏览器 X-CSRF-TOKEN</span><input v-model="pixivCredentials.csrfToken" autocomplete="off"></label>
            <button class="login-button" :disabled="pixivBusy">{{ pixivBusy ? '验证中…' : '验证并保存 Pixiv' }}</button>
          </form>
          <p v-if="pixivError" class="login-error">{{ pixivError }}</p>
        </div>

        <div v-else class="credential-unavailable"><img :class="{ 'twitter-night-icon': isDark }" :src="sourceIconFor('twitter')" alt="推特图标"><strong>连接能力开发中</strong><p>账号授权与作者采集器将在后续版本开放。</p></div>
      </div>
    </div>
    <div v-if="showPixiv" class="modal-backdrop" @click.self="showPixiv = false">
      <div class="modal bili-modal pixiv-source-modal">
        <button class="modal-close" @click="showPixiv = false">×</button>
        <p class="eyebrow">PIXIV SOURCE</p>
        <h2>订阅 Pixiv 画师</h2>
        <p>填写画师主页中的数字用户 ID，仅添加订阅。历史作品可在订阅平台页面手动获取。</p>
        <div class="bili-account"><span>已连接 Pixiv · {{ pixivAccount.userName || `UID ${pixivAccount.userId}` }}</span><button @click="showPixiv = false; openSettings('platforms')">管理凭证</button></div>
        <form class="pixiv-source-form" @submit.prevent="subscribePixiv">
          <div class="bili-search pixiv-subscribe-row"><input v-model="pixivArtistId" required inputmode="numeric" pattern="[0-9]+" placeholder="画师用户 ID，例如 12345678"><button :disabled="pixivBusy">{{ pixivBusy ? '添加中…' : '订阅' }}</button></div>
          <label class="subscription-tag-field"><span>作者标签</span><input v-model="pixivSubscriptionTags" placeholder="#插画 #收藏"></label>
        </form>
        <p v-if="pixivError" class="login-error bili-error">{{ pixivError }}</p>
      </div>
    </div>
    <div v-if="selectedPlatform" class="modal-backdrop platform-detail-backdrop" @click.self="selectedPlatform = null">
      <div class="modal platform-detail-modal">
        <button class="modal-close" @click="selectedPlatform = null">×</button>
        <div class="platform-detail-title"><img :class="['source-icon', { 'twitter-night-icon': selectedPlatform.key === 'twitter' && isDark }]" :src="selectedPlatform.image" alt="平台图标"><div><p class="eyebrow">PLATFORM SOURCE</p><h2>{{ selectedPlatform.label }}</h2></div><span :class="['connection-dot', { online: selectedPlatform.configured }]">{{ selectedPlatform.configured ? '已连接' : '未连接' }}</span></div>
        <div class="platform-detail-summary"><div><span>当前账号</span><strong>{{ selectedPlatform.account }}</strong></div><div><span>内容目录</span><strong>{{ selectedPlatform.path }}</strong></div><div><span>作者来源</span><strong>{{ selectedPlatform.feeds.length }} 个</strong></div></div>
        <div class="platform-detail-actions"><button v-if="selectedPlatform.key !== 'twitter'" class="secondary-button" @click="managePlatformCredentials(selectedPlatform.key)">{{ selectedPlatform.configured ? '管理账号凭证' : '连接平台账号' }}</button><button v-if="selectedPlatform.key === 'weibo' && selectedPlatform.configured && !hasWeiboLikesSource" class="secondary-button" :disabled="sourceActionBusy !== ''" @click="addWeiboLikesSource">添加我的点赞</button><button v-if="selectedPlatform.key === 'pixiv' && selectedPlatform.configured && !hasPixivBookmarksSource" class="secondary-button" :disabled="sourceActionBusy !== ''" @click="addPixivBookmarksSource">添加 P站收藏</button><button v-if="selectedPlatform.key === 'bilibili' && selectedPlatform.configured && !hasBilibiliFavoriteOpusSource" class="secondary-button" :disabled="sourceActionBusy !== ''" @click="addBilibiliFavoriteOpusSource">添加收藏专栏</button><button v-if="selectedPlatform.key === 'bilibili' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openBilibili()">添加 UP 主</button><button v-if="selectedPlatform.key === 'weibo' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openWeibo()">添加博主</button><button v-if="selectedPlatform.key === 'pixiv' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openPixiv()">添加画师</button></div>
        <p v-if="sourceActionMessage" class="success-message source-action-message">{{ sourceActionMessage }}</p><p v-if="settingsError" class="login-error">{{ settingsError }}</p>
        <div class="configured-source-list">
          <div class="configured-source-heading"><h3>已配置作者</h3><span>{{ selectedPlatform.feeds.length }} 个</span></div>
          <article v-for="feed in selectedPlatform.feeds" :key="feed.id">
            <img :key="`${feed.id}:${sourceAvatar(feed, selectedPlatform)}`" class="configured-source-avatar" :src="sourceAvatar(feed, selectedPlatform)" data-fallback-index="0" :alt="`${feed.name}头像`" referrerpolicy="no-referrer" @error="handleSourceAvatarError($event, feed, selectedPlatform)">
            <div><strong>{{ feed.name }}</strong><span>{{ feed.handle }} · {{ feed.schedule }}</span><div v-if="feed.tags?.length" class="source-tag-preview"><b v-for="tag in feed.tags" :key="tag">#{{ tag }}</b></div><small>{{ feed.storagePath || `${selectedPlatform.path}/${feed.name}` }}</small></div>
            <button :class="['source-enabled-toggle', { disabled: !feed.enabled }]" type="button" :disabled="sourceActionBusy !== ''" :title="feed.enabled ? '自动同步已启用，点击停用' : '自动同步已停用，点击启用'" @click="toggleFeedEnabled(feed)"><i></i>{{ sourceActionBusy === `toggle:${feed.id}` ? '保存中' : feed.enabled ? '同步中' : '已停用' }}</button>
            <div class="source-row-actions">
              <button class="source-icon-action" title="立即拉取" aria-label="立即拉取" @click="syncSource(feed)" :disabled="sourceActionBusy !== ''"><svg :class="{ spin: sourceActionBusy === `sync:${feed.id}` }" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 9a7 7 0 0 1 11.4-2.5L20 9M4 15l2.5 2.5A7 7 0 0 0 17.9 15"/></svg></button>
              <button class="source-icon-action" title="获取历史动态" aria-label="获取历史动态" @click="syncSource(feed, true)" :disabled="sourceActionBusy !== ''"><svg :class="{ spin: sourceActionBusy === `resync:${feed.id}` }" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/></svg></button>
              <button class="source-icon-action" title="订阅设置" aria-label="订阅设置" @click="openFeedSettings(feed)"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.6v-.2h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z"/></svg></button>
              <button class="source-icon-action delete-posts-button" title="删除全部动态" aria-label="删除全部动态" @click="deleteAuthorPosts(feed.source, feed.name)" :disabled="sourceActionBusy !== '' || postActionBusy !== ''"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18M8 6V4h8v2M6 6l1 15h10l1-15"/><path d="M10 10v7M14 10v7"/></svg></button>
              <button class="source-icon-action delete-source-button" title="删除订阅" aria-label="删除订阅" @click="deleteSource(feed)" :disabled="sourceActionBusy !== ''"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
            </div>
          </article>
          <div v-if="!selectedPlatform.feeds.length" class="platform-empty"><span>＋</span><strong>还没有作者来源</strong><p>{{ platformEmptyMessage(selectedPlatform.key) }}</p></div>
        </div>
      </div>
    </div>
    <div v-if="showFeedSettings && selectedFeed" class="modal-backdrop feed-detail-backdrop" @click.self="showFeedSettings = false">
      <div class="modal feed-settings-modal">
        <button class="modal-close" @click="showFeedSettings = false">×</button>
        <p class="eyebrow">SOURCE DETAILS</p><h2>{{ selectedFeed.name }}</h2><p>{{ sourceMeta[selectedFeed.source]?.label }} · {{ selectedFeed.handle }}</p>
        <form class="settings-form feed-settings-form" @submit.prevent="saveFeedSettings">
          <div class="feed-field-row">
            <label><span>Cron</span><input v-model="selectedFeed.schedule" class="cron-input" :readonly="!cronEditing" :title="nextCronExecution(selectedFeed.schedule)" required placeholder="0 6 * * *" maxlength="80" @dblclick="cronEditing = true" @blur="cronEditing = false"></label>
            <label><span>作者标签</span><input v-model="selectedFeed.tagInput" placeholder="#绘画 #日常" maxlength="120"></label>
          </div>
          <div class="feed-field-row">
            <label><span>包含关键词</span><input v-model="selectedFeed.includeKeywordInput" placeholder="插画 绘画" maxlength="240"></label>
            <label><span>排除关键词</span><input v-model="selectedFeed.excludeKeywordInput" placeholder="广告 抽奖" maxlength="240"></label>
          </div>
          <div class="feed-toggle-row">
            <label><input v-model="selectedFeed.onlyWithImages" type="checkbox"><span>包含图片</span></label>
            <label v-if="selectedFeed.source === 'weibo' || (selectedFeed.source === 'bilibili' && !selectedFeed.id.startsWith('bili-opus-favorites-'))"><input v-model="selectedFeed.includeVideos" type="checkbox"><span>包含视频</span></label>
            <label><input v-model="selectedFeed.enabled" type="checkbox"><span>启用自动同步</span></label>
          </div>
          <div v-if="selectedFeed.source === 'bilibili' && !selectedFeed.id.startsWith('bili-opus-favorites-')" class="content-scope"><strong>内容范围</strong><span>图文动态（DRAW）</span><label><input v-model="selectedFeed.contentTypes" type="checkbox" value="ARTICLE"><span>专栏（ARTICLE）</span></label><small>专栏默认关闭；转发动态仍过滤。</small></div>
          <p v-if="settingsError" class="login-error">{{ settingsError }}</p><button class="login-button" :disabled="settingsBusy">保存来源设置</button>
        </form>
      </div>
    </div>
    <div v-if="confirmDialog.open" class="confirm-dialog-layer" @click.self="closeConfirmDialog(false)">
      <section class="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-dialog-title">
        <div :class="['confirm-dialog-icon', confirmDialog.tone]">!</div>
        <div class="confirm-dialog-content">
          <p class="eyebrow">PLEASE CONFIRM</p>
          <h3 id="confirm-dialog-title">{{ confirmDialog.title }}</h3>
          <p>{{ confirmDialog.message }}</p>
          <div class="confirm-dialog-actions">
            <button class="secondary-button" type="button" @click="closeConfirmDialog(false)">{{ confirmDialog.cancelText }}</button>
            <button :class="['dialog-confirm-button', confirmDialog.tone]" type="button" @click="closeConfirmDialog(true)">{{ confirmDialog.confirmText }}</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
