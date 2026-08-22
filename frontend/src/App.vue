<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import bilibiliIcon from '../icon/bilibili.png'
import bilibiliLineIcon from '../icon/bilibili-1.png'
import pixivIcon from '../icon/Pixiv.png'
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
import refreshIcon from '../icon/刷新.png'
import originalSizeIcon from '../icon/原始尺寸.png'
import rotateLightboxIcon from '../icon/旋转.png'
import downloadLightboxIcon from '../icon/下载-细线.png'
import closeLightboxIcon from '../icon/取消.png'
import visitPostIcon from '../icon/访问.png'
import scrollTopIcon from '../icon/回到顶部.png'
import deleteIcon from '../icon/删除.png'
import selectAllIcon from '../icon/全选.png'

const authenticated = ref(false)
const sessionChecked = ref(false)
const loginError = ref('')
const showSettings = ref(false)
const settingsTab = ref('settings')
const settingsError = ref('')
const settingsBusy = ref(false)
const settingsForm = ref({ username: '', newPassword: '' })
const proxyForm = ref({ proxyUrl: '' })
const proxyStatus = ref({ proxyEnabled: false, proxyUrl: '' })
const proxyMessage = ref('')
const previewQuality = ref({ desktop: 2, mobile: 3 })
const previewQualitySaved = ref({ desktop: 2, mobile: 3 })
const selectedFeed = ref(null)
const cronEditing = ref(false)
const showFeedSettings = ref(false)
const showStartDatePicker = ref(false)
const startDatePickerView = ref({ year: new Date().getFullYear(), month: new Date().getMonth() })
const selectedPlatform = ref(null)
const credentialPlatform = ref(null)
const sourceActionBusy = ref('')
const sourceActionMessage = ref('')
const confirmDialog = ref({ open: false, title: '', message: '', confirmText: '确认', cancelText: '取消', tone: 'danger' })
let confirmResolver = null
const loginBusy = ref(false)
const credentials = ref({ username: '', password: '' })
const rememberPassword = ref(false)
const passwordVisible = ref(false)
const activeNav = ref('all')
const activeSource = ref('all')
const sourcesExpanded = ref(true)
const mobileMenuOpen = ref(false)
const mobileSourcesOpen = ref(false)
const mobilePageSwitching = ref(false)
const mobileTimelineIconSwitching = ref(false)
const showBrandMenu = ref(false)
const phonePortrait = ref(false)
const selectedAuthor = ref(null)
const selectedTag = ref('')
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
const weiboAccount = ref({ configured: false, cookieConfigured: false, passwordConfigured: false, userId: '', userName: '', avatar: '' })
const weiboCredentials = ref({ cookie: '', userId: '' })
const weiboPasswordCredentials = ref({ username: '', password: '' })
const weiboQR = ref(null)
const weiboBusy = ref(false)
const weiboError = ref('')
let weiboPollTimer = null
const twitterAccount = ref({ configured: false, userId: '', userName: '', avatar: '' })
const twitterCredentials = ref({ apiKey: '', username: '' })
const twitterBusy = ref(false)
const twitterError = ref('')
const syncing = ref(false)
const posts = ref([])
const postStats = ref(null)
const postsFullyLoaded = ref(false)
const feeds = ref([])
const resolvedAuthorAvatars = ref({})
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
const subscriptionPage = ref(1)
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
const masonryMetricsReady = ref(false)
const masonryViewportTop = ref(0)
const masonryViewportBottom = ref(900)
const masonryDetailPost = ref(null)
const mobileDetailIndex = ref(0)
const mobileDetailDragX = ref(0)
const mobileDetailTrackShift = ref(0)
const mobileDetailDragging = ref(false)
const mobileDetailAnimating = ref(false)
const mobileDetailPageDragX = ref(0)
const mobileDetailPageDragY = ref(0)
const mobileDetailPageDragging = ref(false)
const mobileDetailPageAnimating = ref(false)
const mobileDetailPageTransitionMs = ref(320)
const mobileDetailPageTransitionEasing = ref('cubic-bezier(.2, .78, .18, 1)')
const showScrollTop = ref(false)
const mobileControlsVisible = ref(true)
const mobilePostReturnPath = ref('/')
const mobilePostReturnScrollY = ref(0)
const mobileAuthorDetailState = ref(null)
const mobileAuthorPageDragX = ref(0)
const mobileAuthorPageDragging = ref(false)
const mobileAuthorPageAnimating = ref(false)
const mobileAuthorPreviewPost = ref(null)
const mobileAuthorPreviewSnapshot = ref({ active: false, items: [], height: 0, scrollY: 0 })
const mobileAuthorPreviewHandoff = ref(false)
const mobileAuthorPreviewFading = ref(false)
const mobileDetailReturnHandoff = ref(false)
const mobileDetailReturnFading = ref(false)
const mobileTimelineReturnPreviewPost = ref(null)
const mobileTimelineReturnHandoff = ref(false)
const mobileTimelineReturnFading = ref(false)
const pendingPostId = ref('')
const feedListElement = ref(null)
const estimatedPostHeight = 560
const timelineOverscan = 5
let phonePortraitQuery = null
let timelineFrame = 0
let feedListDocumentTop = 0
let feedListTopValid = false
let mobileAuthorScrollY = 0
let mobileControlsLastActivity = 0
let masonryAssignmentColumnCount = 0
let postResizeObserver = null
let feedListResizeObserver = null
let masonryMetricsFrame = 0
let pendingMasonryWidth = 0
let sessionPollTimer = null
const observedPostElements = new Map()
const masonryColumnAssignments = new Map()
const preloadedPreviewUrls = new Set()
const transientTimers = new Set()
const lightbox = ref({ open: false, media: [], index: 0, author: '', scale: 1, rotation: 0, fit: true, x: 0, y: 0, dragging: false, motion: 'enter' })
const lightboxImageElement = ref(null)
const lightboxScalePercent = ref(100)
const lightboxAtOriginalSize = ref(false)
const lightboxDockVisible = ref(true)
const desktopLightboxHoverTarget = ref('')
const lightboxClosing = ref(false)
const lightboxTransitioning = ref(false)
const lightboxDisplaySource = ref('')
const lightboxOriginalLoaded = ref(false)
const mobileLightboxDragX = ref(0)
const mobileLightboxTrackShift = ref(0)
const mobileLightboxDragging = ref(false)
const mobileLightboxAnimating = ref(false)
const mobileLightboxExitY = ref(0)
const mobileLightboxExitDragging = ref(false)
const mobileLightboxExitAnimating = ref(false)
const mobileLightboxEntering = ref(false)
const mobileLightboxDotsVisible = ref(false)
const lightboxZoomAnimating = ref(false)
const mobileLightboxMenu = ref({ open: false, x: 0, y: 0 })
const meteorBurst = ref([])
const lightboxPointers = new Map()
let lightboxGesture = null
let lightboxGestureHadPinch = false
let lightboxScaleFrame = 0
let lightboxLastTap = { time: 0, x: 0, y: 0 }
let lightboxDockTimer = 0
let lightboxLongPressTimer = 0
let lightboxSingleTapTimer = 0
let lightboxLoadSequence = 0
let lightboxHistoryActive = false
let lightboxHistoryPopPending = false
let phoneOverlayHistoryActive = ''
let phoneOverlayHistoryClosing = false
let phoneOverlayDismissInProgress = false
let meteorBurstTimer = 0
let meteorCleanupTimer = 0
let meteorBurstSequence = 0
let mobileDetailTouch = null
let mobileDetailSwipeClickBlocked = false
let mobileDetailAnimationTimer = 0
let mobileDetailAnimationFrame = 0
let mobileDetailTransition = null
let mobileDetailPageTouch = null
let mobileDetailPageTimer = 0
let mobileAuthorPageTouch = null
let mobileAuthorPageTimer = 0
let mobileAuthorHandoffTimer = 0
let mobileDetailReturnHandoffTimer = 0
let mobileTimelineReturnHandoffTimer = 0
let mobilePostOriginVisual = null
let mobileDetailRouteExitLayer = null
let mobileDetailRouteExitTarget = null
let mobileDetailGestureReturnPending = false
let postLoadGeneration = 0
let mobileLightboxAnimationTimer = 0
let mobileLightboxAnimationFrame = 0
let mobileLightboxTransitionStep = 0
let mobileLightboxDotsTimer = 0
let mobileLightboxInertiaFrame = 0
let lightboxZoomTimer = 0
let mobileControlsTimer = 0

const sourceMeta = {
  bilibili: { label: '哔哩哔哩', icon: 'bl', image: bilibiliIcon, lineImage: bilibiliLineIcon, color: 'blue' },
  weibo: { label: '微博', icon: 'wb', image: weiboIcon, lineImage: weiboLineIcon, color: 'coral' },
  pixiv: { label: 'Pixiv', icon: 'px', image: pixivIcon, lineImage: pixivLineIcon, color: 'violet' },
  twitter: { label: '推特', icon: 'tw', image: twitterIcon, lineImage: twitterLineIcon, nightImage: twitterLineIcon, color: 'twitter' }
}
const validSources = new Set(Object.keys(sourceMeta))
const startDatePickerWeekdays = ['日', '一', '二', '三', '四', '五', '六']
const startDatePickerLabel = computed(() => `${startDatePickerView.value.year}年${startDatePickerView.value.month + 1}月`)
const startDatePickerDays = computed(() => {
  const { year, month } = startDatePickerView.value
  const firstDay = new Date(year, month, 1).getDay()
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(year, month, index - firstDay + 1)
    const dayYear = date.getFullYear()
    const dayMonth = date.getMonth()
    const day = date.getDate()
    return {
      key: `${dayYear}-${String(dayMonth + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`,
      day,
      current: dayMonth === month
    }
  })
})
function sourceIconFor(source) {
  const meta = sourceMeta[source]
  return isDark.value && meta?.nightImage ? meta.nightImage : meta?.image
}
function startDateParts(value) {
  const match = String(value || '').match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2]) - 1
  const day = Number(match[3])
  const date = new Date(year, month, day)
  return date.getFullYear() === year && date.getMonth() === month && date.getDate() === day ? { year, month, day } : null
}
function startDateTodayKey() {
  const today = new Date()
  return `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
}
function formatStartDate(value) {
  const parts = startDateParts(value)
  return parts ? `${parts.year}年${String(parts.month + 1).padStart(2, '0')}月${String(parts.day).padStart(2, '0')}日` : ''
}
function openStartDatePicker() {
  const parts = startDateParts(selectedFeed.value?.startDate) || startDateParts(startDateTodayKey())
  startDatePickerView.value = { year: parts.year, month: parts.month }
  showStartDatePicker.value = true
}
function changeStartDatePickerMonth(offset) {
  const current = startDatePickerView.value
  const next = new Date(current.year, current.month + offset, 1)
  startDatePickerView.value = { year: next.getFullYear(), month: next.getMonth() }
}
function selectStartDate(date) {
  if (!selectedFeed.value) return
  selectedFeed.value.startDate = date.key
  showStartDatePicker.value = false
}
function clearStartDate() {
  if (selectedFeed.value) selectedFeed.value.startDate = ''
  showStartDatePicker.value = false
}
function selectStartDateToday() {
  const parts = startDateParts(startDateTodayKey())
  startDatePickerView.value = { year: parts.year, month: parts.month }
  selectStartDate({ key: startDateTodayKey() })
}
const statsPosts = computed(() => {
  const allPosts = Array.isArray(posts.value) ? posts.value : []
  return activeSource.value === 'all' ? allPosts : allPosts.filter(post => post.source === activeSource.value)
})
const serverStats = computed(() => {
  if (!postStats.value) return null
  return activeSource.value === 'all' ? postStats.value.all : postStats.value.bySource?.[activeSource.value]
})
const totalStatsCount = computed(() => !postsFullyLoaded.value && serverStats.value ? serverStats.value.total : statsPosts.value.length)
const todayStatsCount = computed(() => {
  if (!postsFullyLoaded.value && serverStats.value) return serverStats.value.today
  const today = new Date()
  return statsPosts.value.filter(post => {
    const published = new Date(post.published)
    return !Number.isNaN(published.getTime()) && published.getFullYear() === today.getFullYear() && published.getMonth() === today.getMonth() && published.getDate() === today.getDate()
  }).length
})
const favoriteStatsCount = computed(() => !postsFullyLoaded.value && serverStats.value ? serverStats.value.favorites : statsPosts.value.filter(post => post.liked).length)
const selectedPostCount = computed(() => selectedPostIds.value.length)
const mobileDetailMedia = computed(() => postDetailMedia(masonryDetailPost.value))
const mobileDetailCurrentMedia = computed(() => mobileDetailMedia.value[mobileDetailIndex.value] || null)
const mobileDetailSlides = computed(() => {
  const media = mobileDetailMedia.value
  if (!media.length) return []
  if (media.length === 1) return [{ media: media[0], index: 0, position: 0 }]
  return [-1, 0, 1].map(position => {
    const index = (mobileDetailIndex.value + position + media.length) % media.length
    return { media: media[index], index, position }
  })
})
const mobileDetailTrackStyle = computed(() => {
  const baseShift = mobileDetailMedia.value.length > 1 ? -100 : 0
  return {
    transform: `translate3d(calc(${baseShift + mobileDetailTrackShift.value}% + ${mobileDetailDragX.value}px), 0, 0)`,
    transition: mobileDetailDragging.value ? 'none' : mobileDetailAnimating.value ? 'transform .26s cubic-bezier(.18, .84, .22, 1)' : 'none'
  }
})
const mobileLightboxSlides = computed(() => {
  const media = lightbox.value.media || []
  if (!media.length) return []
  if (media.length === 1) return [{ source: lightboxDisplaySource.value || media[0], media: media[0], index: 0, position: 0 }]
  return [-1, 0, 1].map(position => {
    const index = (lightbox.value.index + position + media.length) % media.length
    const source = position === 0 ? (lightboxDisplaySource.value || previewMedia(media[index])) : previewMedia(media[index])
    return { source, media: media[index], index, position }
  })
})
const mobileLightboxTrackStyle = computed(() => {
  const baseShift = lightbox.value.media.length > 1 ? -100 : 0
  return {
    transform: `translate3d(calc(${baseShift + mobileLightboxTrackShift.value}% + ${mobileLightboxDragX.value}px), 0, 0)`,
    transition: mobileLightboxDragging.value ? 'none' : mobileLightboxAnimating.value ? 'transform .24s cubic-bezier(.18, .86, .22, 1)' : 'none'
  }
})
const mobileDetailPageStyle = computed(() => {
  const width = typeof window === 'undefined' ? 390 : Math.max(1, window.innerWidth)
  const height = typeof window === 'undefined' ? 844 : Math.max(1, window.innerHeight)
  const horizontalProgress = Math.min(1, Math.abs(mobileDetailPageDragX.value) / width)
  const verticalProgress = Math.min(1, Math.abs(mobileDetailPageDragY.value) / height)
  const progress = Math.max(horizontalProgress, verticalProgress)
  const scale = 1 - verticalProgress * .018
  return {
    transform: `translate3d(${mobileDetailPageDragX.value}px, ${mobileDetailPageDragY.value}px, 0) scale(${scale})`,
    opacity: String(1 - progress * .08),
    transition: mobileDetailPageDragging.value ? 'none' : mobileDetailPageAnimating.value ? `transform ${mobileDetailPageTransitionMs.value}ms ${mobileDetailPageTransitionEasing.value}, opacity ${Math.min(260, mobileDetailPageTransitionMs.value)}ms ease-out` : 'none'
  }
})
const mobileAuthorPreviewDisplayPost = computed(() => masonryDetailPost.value || mobileAuthorPreviewPost.value)
const mobileAuthorTimelinePosts = computed(() => {
  const post = mobileAuthorPreviewDisplayPost.value
  if (!post) return []
  let result = posts.value.filter(item => item.source === post.source && item.author === post.author)
  const keywords = timelineSearch.value.trim().normalize('NFKC').toLocaleLowerCase().split(/\s+/).filter(Boolean)
  if (keywords.length) result = result.filter(item => {
    const tags = Array.isArray(item.tags) ? item.tags : []
    const searchable = [item.caption, item.author, ...tags, ...tags.map(tag => `#${tag}`)]
      .map(value => String(value || '').normalize('NFKC').toLocaleLowerCase())
    return keywords.every(keyword => searchable.some(value => value.includes(keyword)))
  })
  return [...result].sort((left, right) => {
    const difference = new Date(right.published).getTime() - new Date(left.published).getTime()
    return timelineSort.value === 'newest' ? difference : -difference
  })
})
function buildMobilePreviewMasonrySnapshot(items, scrollY = 0) {
  const gap = 8
  const viewportWidth = typeof window === 'undefined' ? 390 : Math.max(1, window.innerWidth)
  const columnWidth = Math.max(1, (viewportWidth - 24 - gap) / 2)
  const heights = [0, 0]
  const allItems = items.map((post, index) => {
    const column = heights[1] < heights[0] ? 1 : 0
    const height = masonryHeights.value[post.id] || estimateMasonryPostHeight(post)
    const item = { post, index, column, x: column * (columnWidth + gap), y: heights[column], height, width: columnWidth }
    heights[column] += height + gap
    return item
  })
  const height = Math.max(0, ...heights) - (allItems.length ? gap : 0)
  const feedOffset = 142
  const viewportTop = Math.max(0, scrollY - feedOffset - 720)
  const viewportBottom = scrollY - feedOffset + (typeof window === 'undefined' ? 844 : window.innerHeight) + 720
  return {
    active: true,
    items: allItems.filter(item => item.y + item.height >= viewportTop && item.y <= viewportBottom),
    height,
    scrollY: Math.max(0, scrollY)
  }
}
function captureMobileAuthorPreviewSnapshot(scrollY = 0) {
  mobileAuthorPreviewSnapshot.value = buildMobilePreviewMasonrySnapshot(mobileAuthorTimelinePosts.value, scrollY)
}
function mobilePreviewMasonryItemStyle(item) {
  return { left: `${item.x}px`, top: `${item.y}px`, width: `${item.width}px` }
}
const mobileAuthorPreviewPosts = computed(() => mobileAuthorTimelinePosts.value.slice(0, 8))
const mobileAuthorPreviewFallback = computed(() => buildMobilePreviewMasonrySnapshot(mobileAuthorTimelinePosts.value, mobileAuthorDetailState.value?.authorScrollY || 0))
const mobileAuthorPreviewLayout = computed(() => mobileAuthorPreviewSnapshot.value.active ? mobileAuthorPreviewSnapshot.value : mobileAuthorPreviewFallback.value)
const mobileAuthorPreviewItems = computed(() => mobileAuthorPreviewLayout.value.items)
const mobileAuthorPreviewFeedStyle = computed(() => ({ height: `${Math.ceil(mobileAuthorPreviewLayout.value.height)}px` }))
const mobileAuthorPreviewContentStyle = computed(() => ({ transform: `translate3d(0, ${-mobileAuthorPreviewLayout.value.scrollY}px, 0)` }))
const mobileAuthorPreviewStyle = computed(() => {
  if (mobileAuthorPreviewHandoff.value) return {
    zIndex: 70,
    opacity: mobileAuthorPreviewFading.value ? '0' : '1',
    transform: 'translate3d(0, 0, 0)',
    transition: mobileAuthorPreviewFading.value ? 'opacity .18s ease-out' : 'none'
  }
  const width = typeof window === 'undefined' ? 390 : Math.max(1, window.innerWidth)
  const progress = Math.min(1, Math.max(0, -mobileDetailPageDragX.value) / width)
  return {
    opacity: String(.7 + progress * .3),
    transform: `translate3d(${100 - progress * 100}%, 0, 0)`,
    transition: mobileDetailPageDragging.value ? 'none' : mobileDetailPageAnimating.value ? 'transform .32s cubic-bezier(.16,.88,.22,1)' : 'none'
  }
})
const mobileAuthorReturnsToTimeline = computed(() => Boolean(authorProfile.value && mobileAuthorDetailState.value?.returnToDetail === false))
const mobilePagedReturnsToTimeline = computed(() => Boolean((authorProfile.value || selectedTag.value) && mobileAuthorDetailState.value?.returnToDetail === false))
const mobileReturnTimelinePosts = computed(() => {
  const state = mobileAuthorDetailState.value
  if (!mobileAuthorReturnsToTimeline.value || !Array.isArray(state?.returnPostIds)) return filteredPosts.value
  const byId = new Map(posts.value.map(post => [String(post.id), post]))
  return state.returnPostIds.map(id => byId.get(String(id))).filter(Boolean)
})
const mobileReturnPreviewPosts = computed(() => mobileReturnTimelinePosts.value.slice(0, 8))
const mobileReturnPreviewLayout = computed(() => buildMobilePreviewMasonrySnapshot(
  mobileReturnTimelinePosts.value,
  mobileAuthorReturnsToTimeline.value ? mobileAuthorDetailState.value?.detailScrollY : mobilePostReturnScrollY.value
))
const mobileReturnPreviewItems = computed(() => mobileReturnPreviewLayout.value.items)
const mobileReturnPreviewFeedStyle = computed(() => ({ height: `${Math.ceil(mobileReturnPreviewLayout.value.height)}px` }))
const mobileReturnPreviewDisplayPost = computed(() => masonryDetailPost.value || mobileTimelineReturnPreviewPost.value || (mobileAuthorReturnsToTimeline.value ? mobileAuthorDetailState.value?.post : null))
const mobileReturnPreviewTitle = computed(() => mobileAuthorReturnsToTimeline.value ? (mobileAuthorDetailState.value?.returnTitle || '全部动态') : mobileTimelineTitle.value)
const mobileDetailReturnContentStyle = computed(() => ({ transform: `translate3d(0, ${-(mobileAuthorDetailState.value?.detailScrollY || 0)}px, 0)` }))
const mobileReturnPreviewStyle = computed(() => {
  if (mobileTimelineReturnHandoff.value) return {
    zIndex: 70,
    opacity: mobileTimelineReturnFading.value ? '0' : '1',
    transform: 'translate3d(0, 0, 0)',
    transition: mobileTimelineReturnFading.value ? 'opacity .18s ease-out' : 'none'
  }
  const width = typeof window === 'undefined' ? 390 : Math.max(1, window.innerWidth)
  const returningFromAuthor = mobilePagedReturnsToTimeline.value
  const dragX = returningFromAuthor ? mobileAuthorPageDragX.value : mobileDetailPageDragX.value
  const dragging = returningFromAuthor ? mobileAuthorPageDragging.value : mobileDetailPageDragging.value
  const animating = returningFromAuthor ? mobileAuthorPageAnimating.value : mobileDetailPageAnimating.value
  const progress = Math.min(1, Math.max(0, dragX) / width)
  return {
    opacity: String(.7 + progress * .3),
    transform: `translate3d(${-100 + progress * 100}%, 0, 0)`,
    transition: dragging ? 'none' : animating ? 'transform .32s cubic-bezier(.16,.88,.22,1)' : 'none'
  }
})
const mobilePreviousDetailPreviewStyle = computed(() => {
  const height = typeof window === 'undefined' ? 844 : Math.max(1, window.innerHeight)
  const dragY = Math.max(0, mobileDetailPageDragY.value)
  const progress = Math.min(1, dragY / height)
  return {
    opacity: String(.82 + progress * .18),
    transform: `translate3d(0, ${-height + dragY}px, 0) scale(${.985 + progress * .015})`,
    transition: mobileDetailPageDragging.value ? 'none' : mobileDetailPageAnimating.value ? `transform ${mobileDetailPageTransitionMs.value}ms ${mobileDetailPageTransitionEasing.value}, opacity ${Math.min(260, mobileDetailPageTransitionMs.value)}ms ease-out` : 'none'
  }
})
const mobileNextDetailPreviewStyle = computed(() => {
  const height = typeof window === 'undefined' ? 844 : Math.max(1, window.innerHeight)
  const dragY = Math.min(0, mobileDetailPageDragY.value)
  const progress = Math.min(1, Math.abs(dragY) / height)
  return {
    opacity: String(.82 + progress * .18),
    transform: `translate3d(0, ${height + dragY}px, 0) scale(${.985 + progress * .015})`,
    transition: mobileDetailPageDragging.value ? 'none' : mobileDetailPageAnimating.value ? `transform ${mobileDetailPageTransitionMs.value}ms ${mobileDetailPageTransitionEasing.value}, opacity ${Math.min(260, mobileDetailPageTransitionMs.value)}ms ease-out` : 'none'
  }
})
const mobileAuthorPageStyle = computed(() => {
  const width = typeof window === 'undefined' ? 390 : Math.max(1, window.innerWidth)
  const progress = Math.min(1, Math.max(0, mobileAuthorPageDragX.value) / width)
  return {
    transform: `translate3d(${mobileAuthorPageDragX.value}px, 0, 0)`,
    opacity: String(1 - progress * .08),
    transition: mobileAuthorPageDragging.value ? 'none' : mobileAuthorPageAnimating.value ? 'transform .32s cubic-bezier(.16,.88,.22,1), opacity .26s ease' : 'none'
  }
})
const mobileDetailReturnPreviewStyle = computed(() => {
  if (mobileDetailReturnHandoff.value) return {
    zIndex: 70,
    opacity: mobileDetailReturnFading.value ? '0' : '1',
    transform: 'translate3d(0, 0, 0)',
    transition: mobileDetailReturnFading.value ? 'opacity .18s ease-out' : 'none'
  }
  const width = typeof window === 'undefined' ? 390 : Math.max(1, window.innerWidth)
  const progress = Math.min(1, Math.max(0, mobileAuthorPageDragX.value) / width)
  return {
    opacity: String(.7 + progress * .3),
    transform: `translate3d(${-100 + progress * 100}%, 0, 0)`,
    transition: mobileAuthorPageDragging.value ? 'none' : mobileAuthorPageAnimating.value ? 'transform .32s cubic-bezier(.16,.88,.22,1)' : 'none'
  }
})
const mobileLightboxLayerStyle = computed(() => ({
  transform: `translate3d(0, ${mobileLightboxExitY.value}px, 0)`,
  opacity: '1',
  transition: mobileLightboxExitDragging.value ? 'none' : mobileLightboxExitAnimating.value ? 'transform .34s cubic-bezier(.16,.88,.22,1)' : undefined
}))
const filteredPosts = computed(() => {
  const allPosts = Array.isArray(posts.value) ? posts.value : []
  const timeline = activeNav.value === 'liked' ? allPosts.filter(post => post.liked) : allPosts
  const sourceTimeline = activeSource.value === 'all' ? timeline : timeline.filter(post => post.source === activeSource.value)
  let result = sourceTimeline
  if (selectedTag.value) result = result.filter(post => post.tags?.includes(selectedTag.value))
  else if (selectedAuthor.value?.feedId) result = result.filter(post => post.source === selectedAuthor.value.source && post.feedIds?.includes(selectedAuthor.value.feedId))
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
const postById = computed(() => new Map(posts.value.map(post => [String(post.id), post])))
const effectiveTimelineView = computed(() => timelineView.value)
const isMasonryView = computed(() => effectiveTimelineView.value === 'masonry')
const mobileDetailTimelineIndex = computed(() => {
  if (!masonryDetailPost.value) return -1
  return filteredPosts.value.findIndex(post => String(post.id) === String(masonryDetailPost.value.id))
})
const mobileDetailPreviousPost = computed(() => {
  const index = mobileDetailTimelineIndex.value
  return index > 0 ? filteredPosts.value[index - 1] : null
})
const mobileDetailNextPost = computed(() => {
  const index = mobileDetailTimelineIndex.value
  return index >= 0 && index < filteredPosts.value.length - 1 ? filteredPosts.value[index + 1] : null
})
const mobileAtAllTimeline = computed(() => activeNav.value === 'all' && activeSource.value === 'all' && !showSettings.value && !masonryDetailPost.value)
const mobileTimelineMeta = computed(() => activeNav.value === 'source' && sourceMeta[activeSource.value] ? sourceMeta[activeSource.value] : null)
const mobileTimelineTitle = computed(() => mobileTimelineMeta.value ? `${mobileTimelineMeta.value.label}动态` : '全部动态')
const phoneOverlayKey = computed(() => {
  if (!phonePortrait.value) return ''
  if (mobileLightboxMenu.value.open) return 'lightbox-menu'
  if (lightbox.value.open) return ''
  if (confirmDialog.value.open) return 'confirm'
  if (showFeedSettings.value) return 'feed-settings'
  if (credentialPlatform.value) return 'credential-platform'
  if (selectedPlatform.value) return 'platform'
  if (showBilibili.value) return 'bilibili'
  if (showWeibo.value) return 'weibo'
  if (showPixiv.value) return 'pixiv'
  if (showAdd.value) return 'add-source'
  if (contextMenu.value.open) return 'context-menu'
  if (mobileMenuOpen.value) return 'mobile-menu'
  if (mobileSourcesOpen.value) return 'mobile-sources'
  return ''
})
const visiblePosts = computed(() => filteredPosts.value.slice(timelineStart.value, timelineEnd.value))
function estimateMasonryPostHeight(post) {
  const width = masonryColumnWidth.value
  const inset = phonePortrait.value ? 9 : 12
  let height = inset * 2 + 40
  const firstMedia = masonryCover(post)
  const firstVideo = primaryVideo(post)
  const firstPoster = firstVideo?.poster
  if (firstMedia || firstVideo) {
    const ratioKey = `${post.id}:${masonryCoverIsVideo(post) ? 'video' : 0}`
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
  if (masonryAssignmentColumnCount !== count) {
    masonryAssignmentColumnCount = count
    masonryColumnAssignments.clear()
  }
  const heights = Array.from({ length: count }, () => 0)
  const items = filteredPosts.value.map((post, index) => {
    const id = String(post.id)
    let column = masonryColumnAssignments.get(id)
    if (!Number.isInteger(column) || column < 0 || column >= count) {
      column = 0
      for (let candidate = 1; candidate < count; candidate++) {
        if (heights[candidate] < heights[column]) column = candidate
      }
      masonryColumnAssignments.set(id, column)
    }
    const height = masonryHeights.value[post.id] || estimateMasonryPostHeight(post)
    const item = { post, index, column, x: column * (masonryColumnWidth.value + masonryGap.value), y: heights[column], height }
    heights[column] += height + masonryGap.value
    return item
  })
  return { items, height: Math.max(0, ...heights) - (items.length ? masonryGap.value : 0) }
})
const visibleMasonryItems = computed(() => {
  if (!masonryMetricsReady.value) return []
  const overscan = phonePortrait.value ? 700 : 1000
  const top = Math.max(0, masonryViewportTop.value - overscan)
  const bottom = masonryViewportBottom.value + overscan
  return masonryLayout.value.items.filter(item => item.y + item.height >= top && item.y <= bottom)
})
const masonryFeedStyle = computed(() => isMasonryView.value && filteredPosts.value.length && masonryMetricsReady.value ? { height: `${Math.ceil(masonryLayout.value.height)}px` } : undefined)
const loadedSelectionPosts = computed(() => isMasonryView.value ? visibleMasonryItems.value.map(item => item.post) : visiblePosts.value)
const allLoadedPostsSelected = computed(() => loadedSelectionPosts.value.length > 0 && loadedSelectionPosts.value.every(post => selectedPostIds.value.includes(post.id)))
const timelineOffsets = computed(() => {
  const offsets = [0]
  for (const post of filteredPosts.value) offsets.push(offsets[offsets.length - 1] + (timelineHeights.value[post.id] || estimatedPostHeight) + 15)
  return offsets
})
const timelineTopSpace = computed(() => timelineOffsets.value[Math.min(timelineStart.value, timelineOffsets.value.length - 1)] || 0)
const timelineBottomSpace = computed(() => {
  const offsets = timelineOffsets.value
  return Math.max(0, offsets[offsets.length - 1] - (offsets[Math.min(timelineEnd.value, offsets.length - 1)] || 0))
})
const authorProfile = computed(() => {
  if (!selectedAuthor.value) return null
  const authorPosts = selectedAuthor.value.feedId
    ? posts.value.filter(post => post.source === selectedAuthor.value.source && post.feedIds?.includes(selectedAuthor.value.feedId))
    : posts.value.filter(post => post.source === selectedAuthor.value.source && post.author === selectedAuthor.value.name)
  const latest = authorPosts[0]
  return { ...selectedAuthor.value, avatar: selectedAuthor.value.feedId ? selectedAuthor.value.avatar : (latest?.avatar || selectedAuthor.value.avatar), count: authorPosts.length }
})
const isTimelinePage = computed(() => !showSettings.value && activeNav.value !== 'pulls' && !masonryDetailPost.value)
const platformCards = computed(() => [
  { key: 'bilibili', label: '哔哩哔哩', short: '哔', ...sourceMeta.bilibili, configured: biliAccount.value.configured, account: biliAccount.value.configured ? (biliAccount.value.userName || `UID ${biliAccount.value.userId}`) : '尚未连接', avatar: biliAccount.value.avatar, path: '/flow/bilibili', description: 'UP 主动态、专栏与账号收藏', feeds: feeds.value.filter(feed => feed.source === 'bilibili') },
  { key: 'weibo', label: '微博', short: '微', ...sourceMeta.weibo, configured: weiboAccount.value.configured, account: weiboAccount.value.configured ? (weiboAccount.value.userName || `UID ${weiboAccount.value.userId}`) : '尚未连接', avatar: weiboAccount.value.avatar, path: '/flow/weibo', description: '博主动态与图文媒体', feeds: feeds.value.filter(feed => feed.source === 'weibo') },
  { key: 'pixiv', label: 'Pixiv', short: 'P', ...sourceMeta.pixiv, configured: pixivAccount.value.configured, account: pixivAccount.value.configured ? (pixivAccount.value.userName || `UID ${pixivAccount.value.userId}`) : '尚未连接', avatar: pixivAccount.value.avatar, path: '/flow/pixiv', description: '画师作品与插画媒体', feeds: feeds.value.filter(feed => feed.source === 'pixiv') },
  { key: 'twitter', label: '推特', short: '推', ...sourceMeta.twitter, image: sourceIconFor('twitter'), configured: twitterAccount.value.configured, account: twitterAccount.value.configured ? (`@${String(twitterAccount.value.userName || twitterAccount.value.userId).replace(/^@/, '')}`) : '尚未连接', avatar: twitterAccount.value.avatar, path: '/flow/twitter', description: '账号点赞的推文与媒体', feeds: feeds.value.filter(feed => feed.source === 'twitter') }
])
const subscriptionPageSize = 10
const subscriptionPageCount = computed(() => Math.max(1, Math.ceil(feeds.value.length / subscriptionPageSize)))
const pagedFeeds = computed(() => {
  const page = Math.min(Math.max(1, subscriptionPage.value), subscriptionPageCount.value)
  const start = (page - 1) * subscriptionPageSize
  return feeds.value.slice(start, start + subscriptionPageSize)
})
const hasWeiboLikesSource = computed(() => feeds.value.some(feed => feed.id?.startsWith('weibo-likes-')))
const hasPixivBookmarksSource = computed(() => feeds.value.some(feed => feed.id?.startsWith('pixiv-bookmarks-')))
const hasBilibiliFavoriteOpusSource = computed(() => feeds.value.some(feed => feed.id?.startsWith('bili-opus-favorites-')))
const hasTwitterLikesSource = computed(() => feeds.value.some(feed => feed.id?.startsWith('twitter-likes-')))
function isAccountCollectionFeed(feed) {
  return feed?.id?.startsWith('weibo-likes-') || feed?.id?.startsWith('pixiv-bookmarks-') || feed?.id?.startsWith('bili-opus-favorites-') || feed?.id?.startsWith('twitter-likes-')
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
function authorAvatarKey(post) {
  const source = String(post?.source || '').trim()
  const author = String(post?.author || post?.name || '').trim()
  return source && author ? `${source}\u0000${author}` : ''
}
function uniqueAvatarCandidates(values) {
  const unique = [...new Set(values.map(value => String(value || '').trim()).filter(Boolean))]
  return unique.sort((left, right) => Number(!left.startsWith('/flow/')) - Number(!right.startsWith('/flow/')))
}
const authorAvatarPool = computed(() => {
  const pool = new Map()
  const add = (source, author, avatar) => {
    const key = authorAvatarKey({ source, author })
    if (!key || !avatar) return
    const candidates = pool.get(key) || []
    candidates.push(avatar)
    pool.set(key, candidates)
  }
  for (const post of posts.value) add(post?.source, post?.author, post?.avatar)
  for (const feed of feeds.value) add(feed?.source, feed?.name, feed?.avatar)
  for (const [key, candidates] of pool) pool.set(key, uniqueAvatarCandidates(candidates))
  return pool
})
function postAvatarCandidates(post) {
  const key = authorAvatarKey(post)
  const author = post?.author || post?.name
  const candidates = [resolvedAuthorAvatars.value[key], ...(authorAvatarPool.value.get(key) || []), post?.avatar]
  const matchingFeed = feeds.value.find(feed => feed.source === post?.source && feed.name === author)
  if (matchingFeed?.avatar) candidates.push(matchingFeed.avatar)
  candidates.push(sourceIconFor(post?.source))
  return uniqueAvatarCandidates(candidates)
}
function postAvatar(post) {
  return postAvatarCandidates(post)[0] || ''
}
function rememberPostAvatar(post, value) {
  const key = authorAvatarKey(post)
  const avatar = String(value || '').trim()
  if (!key || !avatar || resolvedAuthorAvatars.value[key] === avatar) return
  resolvedAuthorAvatars.value = { ...resolvedAuthorAvatars.value, [key]: avatar }
}
function handlePostAvatarLoad(event, post) {
  const image = event.currentTarget
  if (image?.naturalWidth > 0) rememberPostAvatar(post, image.currentSrc || image.src)
}
function handlePostAvatarError(event, post) {
  const image = event.currentTarget
  const candidates = postAvatarCandidates(post)
  const currentIndex = Math.max(Number(image.dataset.fallbackIndex || 0), candidates.indexOf(image.getAttribute('src') || image.src))
  const nextIndex = currentIndex + 1
  image.dataset.fallbackIndex = String(nextIndex)
  if (candidates[nextIndex]) image.src = candidates[nextIndex]
}
function preloadPostAvatar(post) {
  const key = authorAvatarKey(post)
  if (!key || resolvedAuthorAvatars.value[key]) return Promise.resolve(resolvedAuthorAvatars.value[key] || '')
  const candidates = postAvatarCandidates(post)
  return new Promise(resolve => {
    const tryCandidate = index => {
      const candidate = candidates[index]
      if (!candidate) {
        resolve('')
        return
      }
      const image = new Image()
      image.decoding = 'async'
      image.onload = () => {
        rememberPostAvatar(post, candidate)
        resolve(candidate)
      }
      image.onerror = () => tryCandidate(index + 1)
      image.src = candidate
    }
    tryCandidate(0)
  })
}
function warmPostAvatars(items, limit = 48) {
  const unique = new Map()
  for (const post of items || []) {
    const key = authorAvatarKey(post)
    if (key && !unique.has(key)) unique.set(key, post)
    if (unique.size >= limit) break
  }
  unique.forEach(post => { void preloadPostAvatar(post) })
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
function feedLastSyncText(feed) {
  if (feed?.lastSyncMessage) return feed.lastSyncMessage
  const timestamp = new Date(feed?.lastSyncedAt || '').getTime()
  return Number.isFinite(timestamp) && timestamp > 0 && new Date(timestamp).getFullYear() > 1900 ? `上次拉取：${relativeTime(timestamp)}` : '尚未拉取'
}
async function login() {
  loginError.value = ''
  loginBusy.value = true
  try {
    const response = await fetch('/api/login', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(credentials.value) })
    if (!response.ok) throw new Error('账号或密码不正确')
    if (rememberPassword.value) localStorage.setItem('lumic-remembered-login', JSON.stringify(credentials.value))
    else localStorage.removeItem('lumic-remembered-login')
    authenticated.value = true
    await loadData()
    await loadPlatformAccounts()
  } catch (error) { loginError.value = error.message }
  finally { loginBusy.value = false }
}
async function logout() {
  clearPhoneOverlayHistoryForNavigation()
  try {
    await fetch('/api/logout', { method: 'POST', credentials: 'same-origin' })
  } finally {
    passwordVisible.value = false
    showBrandMenu.value = false
    mobileMenuOpen.value = false
    mobileSourcesOpen.value = false
    authenticated.value = false
    posts.value = []
    postStats.value = null
    postsFullyLoaded.value = false
    feeds.value = []
    selectedPlatform.value = null
    credentialPlatform.value = null
    stopSelection()
  }
}
function loadRememberedLogin() {
  try {
    const saved = JSON.parse(localStorage.getItem('lumic-remembered-login') || 'null')
    if (!saved?.username || !saved?.password) return
    credentials.value = { username: String(saved.username), password: String(saved.password) }
    rememberPassword.value = true
  } catch {
    localStorage.removeItem('lumic-remembered-login')
  }
}
function publishPostPage(items, preserveLocalState = true) {
  if (!preserveLocalState) {
    posts.value = items
    warmPostAvatars(items)
    resolveRoutedPost()
    return
  }
  const current = new Map(posts.value.map(post => [String(post.id), post]))
  posts.value = items.map(post => {
    const existing = current.get(String(post.id))
    return existing ? { ...post, liked: existing.liked, favoriteExplicit: existing.favoriteExplicit ?? post.favoriteExplicit } : post
  })
  warmPostAvatars(items)
  resolveRoutedPost()
}
async function loadRemainingPostPages(generation, firstPage) {
  const loaded = [...firstPage.items]
  let cursor = firstPage.nextCursor || ''
  let hasMore = Boolean(firstPage.hasMore && cursor)
  let pagesSincePublish = 0
  while (hasMore && generation === postLoadGeneration) {
    await new Promise(resolve => window.setTimeout(resolve, 70))
    if (generation !== postLoadGeneration) return
    try {
      const response = await fetch(`/api/v1/posts?limit=100&order=newest&cursor=${encodeURIComponent(cursor)}`, { cache: 'no-store' })
      if (!response.ok) return
      const page = await response.json()
      if (generation !== postLoadGeneration || !Array.isArray(page.items)) return
      loaded.push(...page.items)
      cursor = page.nextCursor || ''
      hasMore = Boolean(page.hasMore && cursor)
      pagesSincePublish += 1
      if (pagesSincePublish >= 4 || !hasMore) {
        publishPostPage(loaded)
        pagesSincePublish = 0
      }
    } catch {
      return
    }
  }
  if (generation === postLoadGeneration) postsFullyLoaded.value = true
}
async function loadPostsIncrementally() {
  const generation = ++postLoadGeneration
  postsFullyLoaded.value = false
  try {
    const statsDate = startDateTodayKey()
    const timezoneOffset = new Date().getTimezoneOffset()
    const response = await fetch(`/api/v1/posts?limit=100&order=newest&statsDate=${encodeURIComponent(statsDate)}&tzOffset=${timezoneOffset}`, { cache: 'no-store' })
    if (!response.ok) throw new Error('api unavailable')
    const page = await response.json()
    if (generation !== postLoadGeneration) return false
    const items = Array.isArray(page.items) ? page.items : []
    postStats.value = page.stats || null
    publishPostPage(items, false)
    if (page.hasMore && page.nextCursor) void loadRemainingPostPages(generation, { ...page, items })
    else postsFullyLoaded.value = true
    return true
  } catch {
    if (generation === postLoadGeneration) {
      posts.value = []
      postStats.value = null
      postsFullyLoaded.value = true
    }
    return false
  }
}
async function loadData() {
  await loadPostsIncrementally()
  await Promise.all([loadFeeds(), loadProjectSettings()])
}
async function loadProjectSettings() {
  try {
    const response = await fetch('/api/project/settings', { cache: 'no-store' })
    if (!response.ok) return
    const project = await response.json()
    proxyStatus.value = project
    proxyForm.value = { proxyUrl: project.proxyUrl || '' }
    previewQuality.value = {
      desktop: normalizePreviewQualityLevel(project.previewDesktopLevel, 2),
      mobile: normalizePreviewQualityLevel(project.previewMobileLevel, 3)
    }
    previewQualitySaved.value = { ...previewQuality.value }
  } catch {}
}
async function loadFeeds(fallbackFeed = null) {
  const endpoints = ['/api/feeds', '/api/bilibili/subscriptions', '/api/weibo/subscriptions', '/api/pixiv/subscriptions', '/api/twitter/subscriptions']
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
  const [biliResponse, weiboResponse, pixivResponse, twitterResponse] = await Promise.allSettled([
    fetch('/api/bilibili/account', { cache: 'no-store' }),
    fetch('/api/weibo/account', { cache: 'no-store' }),
    fetch('/api/pixiv/account', { cache: 'no-store' }),
    fetch('/api/twitter/account', { cache: 'no-store' })
  ])
  if (biliResponse.status === 'fulfilled' && biliResponse.value.ok) biliAccount.value = await biliResponse.value.json()
  if (weiboResponse.status === 'fulfilled' && weiboResponse.value.ok) weiboAccount.value = await weiboResponse.value.json()
  if (pixivResponse.status === 'fulfilled' && pixivResponse.value.ok) pixivAccount.value = await pixivResponse.value.json()
  if (twitterResponse.status === 'fulfilled' && twitterResponse.value.ok) twitterAccount.value = await twitterResponse.value.json()
}
function setDarkMode(value) {
  isDark.value = Boolean(value)
  localStorage.setItem('lumic-theme', isDark.value ? 'dark' : 'light')
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', isDark.value ? '#080a0e' : '#fbf7ea')
}
function useRoundedTooltip(event) {
  if (phonePortrait.value || !(event.target instanceof Element)) return
  const button = event.target.closest('button[title]')
  if (!button || button.dataset.tooltipDisabled === 'true') return
  const label = button.getAttribute('title')
  if (!label) return
  button.dataset.tooltip = label
  button.removeAttribute('title')
  let bubble = button.querySelector(':scope > .app-tooltip')
  if (!bubble) {
    bubble = document.createElement('span')
    bubble.className = 'app-tooltip'
    bubble.setAttribute('aria-hidden', 'true')
    button.appendChild(bubble)
  }
  bubble.textContent = label
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
      // 保留原有中速和稍慢节奏，并加入更缓慢的一档，避免出现过快的流星。
      const speedProfile = [
        [6.4, 1.6],
        [4.25, 1.15],
        [2.8, 1.5]
      ][Math.floor(Math.random() * 3)]
      const duration = speedProfile[0] + Math.random() * speedProfile[1]
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
  clearPhoneOverlayHistoryForNavigation()
  mobileMenuOpen.value = false
  mobileSourcesOpen.value = false
  showBrandMenu.value = false
  settingsTab.value = 'settings'; showSettings.value = true; activeNav.value = 'settings'; settingsError.value = ''; proxyMessage.value = ''; pixivError.value = ''; weiboError.value = ''; twitterError.value = ''
  if (updateHistory) updateRoute('/settings')
  try {
    const [projectResponse, settingsResponse, biliResponse, pixivResponse, weiboResponse, twitterResponse] = await Promise.all([fetch('/api/project/settings'), fetch('/api/settings'), fetch('/api/bilibili/account'), fetch('/api/pixiv/account'), fetch('/api/weibo/account'), fetch('/api/twitter/account')])
    if (projectResponse.ok) {
      const project = await projectResponse.json()
      proxyStatus.value = project
      proxyForm.value = { proxyUrl: project.proxyUrl || '' }
      previewQuality.value = {
        desktop: normalizePreviewQualityLevel(project.previewDesktopLevel, 2),
        mobile: normalizePreviewQualityLevel(project.previewMobileLevel, 3)
      }
      previewQualitySaved.value = { ...previewQuality.value }
    }
    if (settingsResponse.ok) {
      const settings = await settingsResponse.json()
      settingsForm.value = { username: settings.username || '', newPassword: '' }
    }
    if (biliResponse.ok) biliAccount.value = await biliResponse.json()
    if (pixivResponse.ok) pixivAccount.value = await pixivResponse.json()
    if (weiboResponse.ok) weiboAccount.value = await weiboResponse.json()
    if (twitterResponse.ok) twitterAccount.value = await twitterResponse.json()
  } catch { settingsError.value = '无法读取项目设置' }
}
function normalizePreviewQualityLevel(value, fallback) {
  const numeric = Number(value)
  return Number.isInteger(numeric) && numeric >= 0 && numeric <= 5 ? numeric : fallback
}
function previewQualityLabel(level) {
  return ['原图', '轻度', '均衡', '较高', '最高', '自适应'][normalizePreviewQualityLevel(level, 2)]
}
function previewNetworkHint() {
  const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection
  if (connection?.saveData || connection?.effectiveType === 'slow-2g' || connection?.effectiveType === '2g') return 'slow'
  if (connection?.effectiveType === '3g' || (Number(connection?.downlink) > 0 && Number(connection.downlink) < 3)) return 'medium'
  return 'fast'
}
async function savePreviewQuality() {
  const next = {
    desktop: normalizePreviewQualityLevel(previewQuality.value.desktop, 2),
    mobile: normalizePreviewQualityLevel(previewQuality.value.mobile, 3)
  }
  if (next.desktop === previewQualitySaved.value.desktop && next.mobile === previewQualitySaved.value.mobile) return
  settingsBusy.value = true
  settingsError.value = ''
  try {
    const response = await fetch('/api/project/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ previewDesktopLevel: next.desktop, previewMobileLevel: next.mobile })
    })
    if (!response.ok) throw new Error(await responseError(response, '画质设置保存失败'))
    const project = await response.json()
    previewQuality.value = {
      desktop: normalizePreviewQualityLevel(project.previewDesktopLevel, next.desktop),
      mobile: normalizePreviewQualityLevel(project.previewMobileLevel, next.mobile)
    }
    previewQualitySaved.value = { ...previewQuality.value }
  } catch (error) {
    previewQuality.value = { ...previewQualitySaved.value }
    settingsError.value = error.message
  } finally {
    settingsBusy.value = false
  }
}
async function saveProxy() {
  settingsBusy.value = true; settingsError.value = ''; proxyMessage.value = ''
  try {
    const response = await fetch('/api/project/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(proxyForm.value) })
    if (!response.ok) throw new Error(await responseError(response, '代理保存失败'))
    proxyStatus.value = await response.json(); proxyForm.value.proxyUrl = proxyStatus.value.proxyUrl || ''; proxyMessage.value = proxyStatus.value.proxyEnabled ? '代理已保存' : '已关闭项目代理'
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
  const confirmed = await askConfirm({ title: '保存账号设置', message: '将更新 Lumic 的登录账号和密码，确定继续吗？', confirmText: '保存' })
  if (!confirmed) return
  settingsBusy.value = true
  try {
    const response = await fetch('/api/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(settingsForm.value) })
    if (!response.ok) throw new Error('保存失败，请检查输入')
    settingsForm.value = { username: settingsForm.value.username, newPassword: '' }
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
  // 已有本地连接状态时先展示窗口，账号资料的网络补全不应阻塞页面进入。
  if (pixivAccount.value.configured) {
    selectedPlatform.value = null; showPixiv.value = true
    fetch('/api/pixiv/account').then(async response => {
      if (response.ok) pixivAccount.value = await response.json()
    }).catch(() => {})
    return
  }
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
    if (result.status === 'connected') { weiboAccount.value = { ...weiboAccount.value, configured: true, cookieConfigured: true, userId: result.userId, userName: result.userName, avatar: result.avatar || '' }; weiboQR.value = null; weiboBusy.value = false; return }
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
async function saveTwitterAccount() {
  twitterBusy.value = true; twitterError.value = ''
  try {
	const response = await fetch('/api/twitter/account', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ apiKey: twitterCredentials.value.apiKey.trim(), username: twitterCredentials.value.username.trim() }) })
	if (!response.ok) throw new Error(await responseError(response, 'twitterapi.io 凭证验证失败'))
    twitterAccount.value = await response.json()
    twitterCredentials.value = { apiKey: '', username: '' }
    await loadFeeds()
  } catch (error) { twitterError.value = error.message } finally { twitterBusy.value = false }
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
    const response = await fetch('/api/weibo/account', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: weiboPasswordCredentials.value.username.trim(), password: weiboPasswordCredentials.value.password, savePasswordOnly: weiboAccount.value.cookieConfigured }) })
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
  if (syncing.value) return
  syncing.value = true; timelineMessage.value = ''
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 45_000)
  try {
    const response = await fetch('/api/sync', { method: 'POST', signal: controller.signal })
    const payload = await response.text()
    let result = {}
    try { result = payload ? JSON.parse(payload) : {} } catch {}
    if (!response.ok && !result.message) throw new Error(result.error || '同步失败')
    timelineMessage.value = result.message || '拉取任务已完成'
    void loadData()
  } catch (error) { timelineMessage.value = error?.name === 'AbortError' ? '同步请求超时，后台任务仍可能继续执行。' : error.message } finally { window.clearTimeout(timeout); syncing.value = false }
}
function refreshTimeline(event) {
  if (syncing.value) return
  event?.currentTarget?.querySelector?.('.timeline-refresh-symbol')?.animate(
    [{ transform: 'rotate(0deg)' }, { transform: 'rotate(360deg)' }],
    { duration: 620, easing: 'cubic-bezier(.2,.72,.28,1)' }
  )
  void runFullSync()
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
  twitterError.value = ''
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
  showStartDatePicker.value = false
  const contentTypes = feed.source === 'bilibili' && (!feed.contentTypes || feed.contentTypes.length === 0) ? ['DRAW', 'ARTICLE'] : [...(feed.contentTypes || [])]
  selectedFeed.value = { ...feed, startDate: feed.startDate || '', includeVideos: Boolean(feed.includeVideos), contentTypes, tags: [...(feed.tags || [])], tagInput: formatTagInput(feed.tags), includeKeywordInput: formatKeywordInput(feed.includeKeywords), excludeKeywordInput: formatKeywordInput(feed.excludeKeywords) }
  showFeedSettings.value = true
}
async function saveFeedSettings() {
  if (!selectedFeed.value) return
  settingsBusy.value = true; settingsError.value = ''
  try {
    selectedFeed.value.tags = parseTagInput(selectedFeed.value.tagInput)
    selectedFeed.value.includeKeywords = parseKeywordInput(selectedFeed.value.includeKeywordInput)
    selectedFeed.value.excludeKeywords = parseKeywordInput(selectedFeed.value.excludeKeywordInput)
    if ((selectedFeed.value.source === 'bilibili' && selectedFeed.value.id.startsWith('bili-')) || (selectedFeed.value.source === 'weibo' && selectedFeed.value.id.startsWith('weibo-')) || (selectedFeed.value.source === 'pixiv' && selectedFeed.value.id.startsWith('pixiv-')) || (selectedFeed.value.source === 'twitter' && selectedFeed.value.id.startsWith('twitter-'))) {
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
    showStartDatePicker.value = false
    cronEditing.value = false
  } catch (error) { settingsError.value = error.message } finally { settingsBusy.value = false }
}
function sourceOperationEndpoint(feed) {
  if (feed.source === 'bilibili' && feed.id.startsWith('bili-')) return '/api/bilibili/subscriptions'
  if (feed.source === 'weibo' && feed.id.startsWith('weibo-')) return '/api/weibo/subscriptions'
  if (feed.source === 'pixiv' && feed.id.startsWith('pixiv-')) return '/api/pixiv/subscriptions'
  if (feed.source === 'twitter' && feed.id.startsWith('twitter-')) return '/api/twitter/subscriptions'
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
    if (result.status !== 'failed') void loadPostsIncrementally()
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
function clearMobileLightboxAnimation() {
  if (mobileLightboxAnimationTimer) window.clearTimeout(mobileLightboxAnimationTimer)
  if (mobileLightboxAnimationFrame) window.cancelAnimationFrame(mobileLightboxAnimationFrame)
  mobileLightboxAnimationTimer = 0
  mobileLightboxAnimationFrame = 0
}
function resetMobileLightboxTrack() {
  clearMobileLightboxAnimation()
  clearMobileLightboxInertia()
  mobileLightboxDragX.value = 0
  mobileLightboxTrackShift.value = 0
  mobileLightboxDragging.value = false
  mobileLightboxAnimating.value = false
  mobileLightboxTransitionStep = 0
}
function resetLightboxView() {
  resetMobileLightboxTrack()
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
  clearLightboxSingleTap()
  clearLightboxLongPress()
  scheduleLightboxScaleUpdate()
}
function openLightbox(post, index) {
  if (phonePortrait.value && !lightboxHistoryActive) {
    window.history.pushState({ ...(window.history.state || {}), lumicLightbox: true }, '', window.location.href)
    lightboxHistoryActive = true
  }
  lightbox.value = { open: true, media: post.media || [], index, author: post.author, scale: 1, rotation: 0, fit: true, x: 0, y: 0, dragging: false, motion: 'enter' }
  lightboxClosing.value = false
  mobileLightboxEntering.value = phonePortrait.value
  lightboxTransitioning.value = false
  lightboxScalePercent.value = 100
  lightboxAtOriginalSize.value = false
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  showLightboxDock(true)
  prepareLightboxSource()
  scheduleLightboxScaleUpdate()
  if (phonePortrait.value) scheduleTransient(() => { mobileLightboxEntering.value = false }, 520)
}
function resetLightboxState() {
  resetMobileLightboxTrack()
  lightbox.value = { open: false, media: [], index: 0, author: '', scale: 1, rotation: 0, fit: true, x: 0, y: 0, dragging: false, motion: 'enter' }
  lightboxPointers.clear()
  lightboxGesture = null
  lightboxGestureHadPinch = false
  lightboxAtOriginalSize.value = false
  lightboxLastTap = { time: 0, x: 0, y: 0 }
  lightboxDisplaySource.value = ''
  lightboxOriginalLoaded.value = false
  lightboxClosing.value = false
  desktopLightboxHoverTarget.value = ''
  lightboxTransitioning.value = false
  mobileLightboxExitY.value = 0
  mobileLightboxExitDragging.value = false
  mobileLightboxExitAnimating.value = false
  mobileLightboxEntering.value = false
  mobileLightboxDotsVisible.value = false
  lightboxZoomAnimating.value = false
  lightboxLoadSequence++
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  clearLightboxLongPress()
  clearLightboxSingleTap()
  clearLightboxDockTimer()
  if (mobileLightboxDotsTimer) window.clearTimeout(mobileLightboxDotsTimer)
  mobileLightboxDotsTimer = 0
  if (lightboxZoomTimer) window.clearTimeout(lightboxZoomTimer)
  lightboxZoomTimer = 0
  if (lightboxScaleFrame) window.cancelAnimationFrame(lightboxScaleFrame)
  lightboxScaleFrame = 0
}
function closeLightbox(fromHistory = false, delay = 320) {
  if (lightboxClosing.value) return
  lightboxClosing.value = true
  clearLightboxSingleTap()
  const hadHistory = lightboxHistoryActive
  scheduleTransient(() => {
    lightboxHistoryActive = false
    resetLightboxState()
    if (!fromHistory && hadHistory) {
      lightboxHistoryPopPending = true
      window.history.back()
    }
  }, delay)
}
function updateDesktopLightboxHover(event) {
  if (phonePortrait.value || !lightbox.value.open) return
  const layer = event.currentTarget
  const width = layer?.clientWidth || window.innerWidth
  const edgeWidth = Math.min(118, Math.max(72, width * .085))
  if (event.clientX >= width - 92 && event.clientY <= 92) desktopLightboxHoverTarget.value = 'close'
  else if (event.clientX <= edgeWidth) desktopLightboxHoverTarget.value = 'previous'
  else if (event.clientX >= width - edgeWidth) desktopLightboxHoverTarget.value = 'next'
  else desktopLightboxHoverTarget.value = ''
}
function showDesktopLightboxClose() {
  if (!phonePortrait.value && lightbox.value.open) desktopLightboxHoverTarget.value = 'close'
}
function hideDesktopLightboxClose() {
  if (!phonePortrait.value && desktopLightboxHoverTarget.value === 'close') desktopLightboxHoverTarget.value = ''
}
function clearDesktopLightboxHover() {
  if (!phonePortrait.value) desktopLightboxHoverTarget.value = ''
}
function showMobileLightboxDots(duration = 1500) {
  if (!phonePortrait.value || lightbox.value.media.length < 2) return
  mobileLightboxDotsVisible.value = true
  if (mobileLightboxDotsTimer) window.clearTimeout(mobileLightboxDotsTimer)
  mobileLightboxDotsTimer = window.setTimeout(() => {
    mobileLightboxDotsVisible.value = false
    mobileLightboxDotsTimer = 0
  }, duration)
}
function animateMobileLightboxExit(direction) {
  if (!phonePortrait.value || lightboxClosing.value) return
  mobileLightboxExitDragging.value = false
  mobileLightboxExitAnimating.value = true
  lightboxClosing.value = true
  mobileLightboxExitY.value = (direction < 0 ? -1 : 1) * Math.max(window.innerHeight + 48, 768)
  const hadHistory = lightboxHistoryActive
  scheduleTransient(() => {
    lightboxHistoryActive = false
    resetLightboxState()
    if (hadHistory) {
      lightboxHistoryPopPending = true
      window.history.back()
    }
  }, 340)
}
function finishMobileLightboxTransition(commit = true) {
  const step = mobileLightboxTransitionStep
  clearMobileLightboxAnimation()
  mobileLightboxAnimating.value = false
  mobileLightboxDragging.value = false
  mobileLightboxDragX.value = 0
  mobileLightboxTrackShift.value = 0
  mobileLightboxTransitionStep = 0
  if (!commit || !step || lightbox.value.media.length < 2) return
  lightbox.value.index = (lightbox.value.index + step + lightbox.value.media.length) % lightbox.value.media.length
  resetLightboxView()
  lightbox.value.motion = 'idle'
  prepareLightboxSource()
}
function animateMobileLightbox(step) {
  const total = lightbox.value.media.length
  if (total < 2) return
  if (mobileLightboxAnimating.value) finishMobileLightboxTransition(true)
  mobileLightboxTransitionStep = step > 0 ? 1 : -1
  showMobileLightboxDots(1500)
  mobileLightboxDragging.value = false
  mobileLightboxAnimating.value = true
  mobileLightboxAnimationFrame = window.requestAnimationFrame(() => {
    mobileLightboxAnimationFrame = 0
    mobileLightboxDragX.value = 0
    mobileLightboxTrackShift.value = mobileLightboxTransitionStep > 0 ? -100 : 100
  })
  mobileLightboxAnimationTimer = window.setTimeout(() => finishMobileLightboxTransition(true), 245)
}
function snapMobileLightboxTrack() {
  if (!mobileLightboxDragX.value) {
    resetMobileLightboxTrack()
    return
  }
  clearMobileLightboxAnimation()
  mobileLightboxDragging.value = false
  mobileLightboxAnimating.value = true
  mobileLightboxAnimationFrame = window.requestAnimationFrame(() => {
    mobileLightboxAnimationFrame = 0
    mobileLightboxDragX.value = 0
  })
  mobileLightboxAnimationTimer = window.setTimeout(resetMobileLightboxTrack, 205)
}
function moveLightbox(step) {
  const total = lightbox.value.media.length
  if (total < 2) return
  if (phonePortrait.value) {
    animateMobileLightbox(step)
    return
  }
  if (lightboxTransitioning.value) return
  lightboxTransitioning.value = true
  lightbox.value.motion = step > 0 ? 'leave-next' : 'leave-previous'
  scheduleTransient(() => {
    lightbox.value.index = (lightbox.value.index + step + total) % total
    resetLightboxView()
    lightbox.value.motion = step > 0 ? 'enter-next' : 'enter-previous'
    prepareLightboxSource()
    scheduleTransient(() => {
      lightbox.value.motion = 'idle'
      lightboxTransitioning.value = false
    }, 290)
  }, 150)
}
function prepareLightboxSource() {
  const source = lightbox.value.media[lightbox.value.index] || ''
  const preview = phonePortrait.value ? previewMedia(source) : source
  const sequence = ++lightboxLoadSequence
  lightboxDisplaySource.value = preview
  lightboxOriginalLoaded.value = preview === source
  if (phonePortrait.value && lightbox.value.media.length > 1) {
    for (const offset of [-1, 1]) {
      const index = (lightbox.value.index + offset + lightbox.value.media.length) % lightbox.value.media.length
      const preload = new Image()
      preload.src = previewMedia(lightbox.value.media[index])
    }
  }
  // Mobile keeps the preview mounted while the full image is fetched by its
  // overlay layer. Replacing the display source here would rebuild the slide.
  if (phonePortrait.value) return
  if (!source || preview === source) return
  const original = new Image()
  original.decoding = 'async'
  original.onload = () => {
    if (sequence !== lightboxLoadSequence || !lightbox.value.open) return
    lightboxDisplaySource.value = source
    lightboxOriginalLoaded.value = true
  }
  original.src = source
}
function handleLightboxImageLoad() {
  if (phonePortrait.value) {
    const sequence = lightboxLoadSequence
    // Keep one painted frame of the preview before the original fades over it.
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => {
        if (sequence === lightboxLoadSequence && lightbox.value.open) lightboxOriginalLoaded.value = true
      })
    })
    return
  }
  lightboxOriginalLoaded.value = lightboxDisplaySource.value === lightbox.value.media[lightbox.value.index]
  scheduleLightboxScaleUpdate()
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
  if (window.Lumir?.saveFile) {
    const downloadUrl = new URL(source, window.location.href).href
    window.Lumir.saveFile(downloadUrl, decodeURIComponent(source.split('/').pop()?.split('?')[0] || fallbackName))
    return
  }
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
function clearLightboxSingleTap() {
  if (lightboxSingleTapTimer) window.clearTimeout(lightboxSingleTapTimer)
  lightboxSingleTapTimer = 0
}
function scheduleLightboxSingleTapClose() {
  clearLightboxSingleTap()
  lightboxSingleTapTimer = window.setTimeout(() => {
    lightboxSingleTapTimer = 0
    closeLightbox()
  }, 390)
}
function scheduleLightboxLongPress(event) {
  clearLightboxLongPress()
  const imageTarget = event.target instanceof HTMLImageElement ? event.target : null
  const isCurrentImage = Boolean(imageTarget?.closest?.('.mobile-lightbox-slide.current'))
  if (!phonePortrait.value || event.pointerType !== 'touch' || !isCurrentImage) return
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
function dismissMobileLightboxMenu() {
  clearLightboxSingleTap()
  mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
}
function scheduleLightboxScaleUpdate() {
  if (typeof window === 'undefined') return
  if (lightboxScaleFrame) window.cancelAnimationFrame(lightboxScaleFrame)
  nextTick(() => {
    lightboxScaleFrame = window.requestAnimationFrame(() => {
      lightboxScaleFrame = 0
      const image = currentLightboxImage()
      if (!image?.naturalWidth) return
      const baseScale = lightbox.value.fit ? image.clientWidth / image.naturalWidth : 1
      const renderedScale = baseScale * lightbox.value.scale
      lightboxScalePercent.value = Math.max(1, Math.round(renderedScale * 100))
      lightboxAtOriginalSize.value = Math.abs(renderedScale - 1) < 0.015
    })
  })
}
function currentLightboxImage() {
  const candidates = Array.isArray(lightboxImageElement.value) ? lightboxImageElement.value : [lightboxImageElement.value]
  return candidates.find(image => image?.closest?.('.mobile-lightbox-slide.current'))
    || candidates.find(Boolean)
    || document.querySelector('.mobile-lightbox-slide.current .mobile-lightbox-preview')
    || null
}
function lightboxBaseScale() {
  const image = currentLightboxImage()
  if (!image?.naturalWidth || !lightbox.value.fit) return 1
  return Math.max(0.01, image.clientWidth / image.naturalWidth)
}
function lightboxMaximumScale() {
  return Math.min(20, Math.max(5, 2 / lightboxBaseScale()))
}
function isLightboxOriginalSize() {
  return Math.abs(lightboxBaseScale() * lightbox.value.scale - 1) < 0.015
}
function isMobileLightboxSwipeView() {
  const centered = Math.abs(lightbox.value.x) < 3 && Math.abs(lightbox.value.y) < 3
  const defaultFit = lightbox.value.fit && Math.abs(lightbox.value.scale - 1) < 0.02
  return centered && (defaultFit || isLightboxOriginalSize())
}
function isMobileLightboxDefaultView() {
  return lightbox.value.fit && Math.abs(lightbox.value.scale - 1) < 0.02 && Math.abs(lightbox.value.x) < 3 && Math.abs(lightbox.value.y) < 3
}
function lightboxPointerDistance(points) {
  return Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y)
}
function lightboxPointerCenter(points) {
  return { x: (points[0].x + points[1].x) / 2, y: (points[0].y + points[1].y) / 2 }
}
function clearMobileLightboxInertia() {
  if (mobileLightboxInertiaFrame) window.cancelAnimationFrame(mobileLightboxInertiaFrame)
  mobileLightboxInertiaFrame = 0
}
function mobileLightboxBounds() {
  const image = currentLightboxImage()
  if (!image) return { x: 0, y: 0 }
  const scale = Math.max(.01, lightbox.value.scale)
  return {
    x: Math.max(0, (image.clientWidth * scale - window.innerWidth) / 2),
    y: Math.max(0, (image.clientHeight * scale - window.innerHeight) / 2)
  }
}
function dampBeyond(value, limit, resistance = .24) {
  if (limit <= 0) return value * resistance
  if (value > limit) return limit + (value - limit) * resistance
  if (value < -limit) return -limit + (value + limit) * resistance
  return value
}
function clampMobileLightboxPosition(withResistance = false) {
  const bounds = mobileLightboxBounds()
  const clamp = (value, limit) => Math.max(-limit, Math.min(limit, value))
  lightbox.value.x = withResistance ? dampBeyond(lightbox.value.x, bounds.x) : clamp(lightbox.value.x, bounds.x)
  lightbox.value.y = withResistance ? dampBeyond(lightbox.value.y, bounds.y) : clamp(lightbox.value.y, bounds.y)
  return bounds
}
function settleMobileLightboxPan() {
  clearMobileLightboxInertia()
  lightbox.value.dragging = false
  clampMobileLightboxPosition(false)
}
function startMobileLightboxInertia(velocityX, velocityY) {
  clearMobileLightboxInertia()
  let vx = Math.max(-2.5, Math.min(2.5, velocityX))
  let vy = Math.max(-2.5, Math.min(2.5, velocityY))
  if (Math.hypot(vx, vy) < .18) return settleMobileLightboxPan()
  lightbox.value.dragging = true
  let previous = performance.now()
  const tick = now => {
    const elapsed = Math.min(32, Math.max(8, now - previous))
    previous = now
    const bounds = mobileLightboxBounds()
    lightbox.value.x += vx * elapsed
    lightbox.value.y += vy * elapsed
    const settleAxis = (value, velocity, limit) => {
      if (limit <= 0) return { value: value * .72, velocity: velocity * .72 }
      if (value > limit) {
        const excess = value - limit
        return { value: limit + excess * .22, velocity: -Math.abs(velocity) * .12 - excess * .018 }
      }
      if (value < -limit) {
        const excess = value + limit
        return { value: -limit + excess * .22, velocity: Math.abs(velocity) * .12 - excess * .018 }
      }
      return { value, velocity }
    }
    const settledX = settleAxis(lightbox.value.x, vx, bounds.x)
    const settledY = settleAxis(lightbox.value.y, vy, bounds.y)
    lightbox.value.x = settledX.value
    lightbox.value.y = settledY.value
    vx = settledX.velocity * Math.pow(.91, elapsed / 16)
    vy = settledY.velocity * Math.pow(.91, elapsed / 16)
    if (Math.hypot(vx, vy) < .025 && Math.abs(lightbox.value.x) <= bounds.x + .8 && Math.abs(lightbox.value.y) <= bounds.y + .8) {
      mobileLightboxInertiaFrame = 0
      settleMobileLightboxPan()
      return
    }
    mobileLightboxInertiaFrame = window.requestAnimationFrame(tick)
  }
  mobileLightboxInertiaFrame = window.requestAnimationFrame(tick)
}
function mobileLightboxEdgeOverscroll(rawX, bounds) {
  if (rawX > bounds.x) return rawX - bounds.x
  if (rawX < -bounds.x) return rawX + bounds.x
  return 0
}
function beginLightboxPan(pointer) {
  lightboxGesture = {
    type: 'pan', pointerId: pointer.id, startX: pointer.x, startY: pointer.y,
    startTime: pointer.time, imageX: lightbox.value.x, imageY: lightbox.value.y,
    startScale: lightbox.value.scale, startFit: lightbox.value.fit,
    startSwipeView: isMobileLightboxSwipeView(), startDefaultView: isMobileLightboxDefaultView(), startedOnImage: pointer.startedOnImage, longPressed: false,
    axis: '', edgeOverscroll: 0, prevX: pointer.x, prevY: pointer.y, prevTime: pointer.time, lastX: pointer.x, lastY: pointer.y, lastTime: pointer.time
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
  if (phonePortrait.value && mobileLightboxAnimating.value) finishMobileLightboxTransition(true)
  if (phonePortrait.value) clearMobileLightboxInertia()
  if (mobileLightboxMenu.value.open) mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  clearLightboxSingleTap()
  if (lightboxZoomTimer) window.clearTimeout(lightboxZoomTimer)
  lightboxZoomTimer = 0
  lightboxZoomAnimating.value = false
  event.currentTarget.setPointerCapture(event.pointerId)
  const startedOnImage = event.target instanceof HTMLImageElement && Boolean(event.target.closest('.mobile-lightbox-slide.current'))
  lightboxPointers.set(event.pointerId, { id: event.pointerId, x: event.clientX, y: event.clientY, time: event.timeStamp, startedOnImage })
  lightbox.value.dragging = phonePortrait.value ? !isMobileLightboxDefaultView() : true
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
  if (phonePortrait.value) {
    const dx = event.clientX - lightboxGesture.startX
    const dy = event.clientY - lightboxGesture.startY
    if (!lightboxGesture.axis && Math.max(Math.abs(dx), Math.abs(dy)) > 5) {
      lightboxGesture.axis = Math.abs(dx) > Math.abs(dy) * 1.04 ? 'horizontal' : 'vertical'
    }
    lightboxGesture.prevX = lightboxGesture.lastX
    lightboxGesture.prevY = lightboxGesture.lastY
    lightboxGesture.prevTime = lightboxGesture.lastTime
    lightboxGesture.lastX = event.clientX
    lightboxGesture.lastY = event.clientY
    lightboxGesture.lastTime = event.timeStamp
    const defaultView = lightboxGesture.startDefaultView
    if (!defaultView) {
      clearLightboxLongPress()
      lightbox.value.dragging = true
      const rawX = lightboxGesture.imageX + dx
      const rawY = lightboxGesture.imageY + dy
      const bounds = mobileLightboxBounds()
      const edgeOverscroll = lightboxGesture.axis === 'horizontal' ? mobileLightboxEdgeOverscroll(rawX, bounds) : 0
      lightboxGesture.edgeOverscroll = edgeOverscroll
      lightbox.value.x = dampBeyond(rawX, bounds.x)
      lightbox.value.y = dampBeyond(rawY, bounds.y)
      if (Math.abs(edgeOverscroll) > 28 && lightbox.value.media.length > 1) {
        mobileLightboxDragging.value = true
        mobileLightboxDragX.value = edgeOverscroll * .32
        showMobileLightboxDots(1800)
      } else {
        mobileLightboxDragging.value = false
        mobileLightboxDragX.value = 0
      }
      return
    }
    if (lightboxGesture.axis === 'vertical') {
      clearLightboxLongPress()
      lightbox.value.dragging = false
      mobileLightboxExitDragging.value = true
      mobileLightboxExitAnimating.value = false
      const threshold = Math.min(116, window.innerHeight * .15)
      const capped = Math.min(threshold * .92, Math.abs(dy) * .62)
      mobileLightboxExitY.value = Math.sign(dy || 1) * capped
      return
    }
    if (lightboxGesture.axis !== 'horizontal') return
    clearLightboxLongPress()
    lightbox.value.dragging = false
    mobileLightboxDragging.value = true
    showMobileLightboxDots(1500)
    const limit = Math.max(120, window.innerWidth * .96)
    mobileLightboxDragX.value = Math.max(-limit, Math.min(limit, dx))
    return
  }
  lightbox.value.x = lightboxGesture.imageX + event.clientX - lightboxGesture.startX
  lightbox.value.y = lightboxGesture.imageY + event.clientY - lightboxGesture.startY
}
function toggleLightboxDoubleTap(event) {
  lightboxZoomAnimating.value = true
  if (lightboxZoomTimer) window.clearTimeout(lightboxZoomTimer)
  lightboxZoomTimer = window.setTimeout(() => {
    lightboxZoomAnimating.value = false
    lightboxZoomTimer = 0
  }, 500)
  if (phonePortrait.value) {
    const atDefaultFit = lightbox.value.fit && Math.abs(lightbox.value.scale - 1) < 0.02 && Math.abs(lightbox.value.x) < 2 && Math.abs(lightbox.value.y) < 2
    lightbox.value.fit = true
    if (atDefaultFit) {
      const previousScale = lightbox.value.scale
      const targetScale = Math.min(lightboxMaximumScale(), lightboxBaseScale() > .82 ? 1.92 : 2.22)
      const scaleRatio = targetScale / previousScale
      const image = currentLightboxImage()
      const rect = image?.getBoundingClientRect()
      const focusX = event.clientX - (rect ? rect.left + rect.width / 2 : window.innerWidth / 2)
      const focusY = event.clientY - (rect ? rect.top + rect.height / 2 : window.innerHeight / 2)
      lightbox.value.scale = targetScale
      lightbox.value.x -= focusX * (scaleRatio - 1)
      lightbox.value.y -= focusY * (scaleRatio - 1)
      nextTick(() => clampMobileLightboxPosition(false))
    } else {
      lightbox.value.scale = 1
      lightbox.value.x = 0
      lightbox.value.y = 0
    }
  } else {
    lightbox.value.fit = !lightbox.value.fit || Math.abs(lightbox.value.scale - 1) > 0.02 || Math.abs(lightbox.value.x) > 2 || Math.abs(lightbox.value.y) > 2
    lightbox.value.scale = 1
  }
  if (!phonePortrait.value) {
    lightbox.value.x = 0
    lightbox.value.y = 0
  }
  scheduleLightboxScaleUpdate()
}
function stopLightboxGesture(event) {
  const pointer = lightboxPointers.get(event.pointerId)
  const gesture = lightboxGesture
  if (!pointer) return
  const dx = event.clientX - (gesture?.startX ?? event.clientX)
  const dy = event.clientY - (gesture?.startY ?? event.clientY)
  const duration = event.timeStamp - (gesture?.startTime ?? event.timeStamp)
  const velocityDuration = Math.max(1, (gesture?.lastTime ?? event.timeStamp) - (gesture?.prevTime ?? gesture?.startTime ?? event.timeStamp))
  const velocityX = ((gesture?.lastX ?? event.clientX) - (gesture?.prevX ?? gesture?.startX ?? event.clientX)) / velocityDuration
  const velocityY = ((gesture?.lastY ?? event.clientY) - (gesture?.prevY ?? gesture?.startY ?? event.clientY)) / velocityDuration
  const wasSinglePan = gesture?.type === 'pan' && gesture.pointerId === event.pointerId && lightboxPointers.size === 1
  const wasCancelled = event.type === 'pointercancel'
  const canSwipe = phonePortrait.value ? gesture?.startDefaultView : gesture?.startFit && gesture?.startScale <= 1.02 && Math.abs(gesture?.imageX || 0) < 3 && Math.abs(gesture?.imageY || 0) < 3
  const swipeDistance = Math.min(52, window.innerWidth * .13)
  const edgeSwipeDistance = Math.min(128, window.innerWidth * .3)
  const isDefaultSwipe = !wasCancelled && !lightboxGestureHadPinch && wasSinglePan && !gesture.longPressed && canSwipe && duration < 850 && (Math.abs(dx) > swipeDistance || Math.abs(velocityX) > .42) && Math.abs(dx) > Math.abs(dy) * 1.12
  const isZoomEdgeSwipe = phonePortrait.value && !wasCancelled && !lightboxGestureHadPinch && wasSinglePan && !gesture.longPressed && !gesture?.startDefaultView && gesture?.axis === 'horizontal' && (Math.abs(gesture?.edgeOverscroll || 0) > edgeSwipeDistance || (Math.abs(gesture?.edgeOverscroll || 0) > 38 && Math.abs(velocityX) > .8))
  const isSwipe = isDefaultSwipe || isZoomEdgeSwipe
  const verticalExitThresholdMet = gesture?.startDefaultView
    ? (Math.abs(dy) > Math.min(116, window.innerHeight * .15) || (Math.abs(dy) > 42 && Math.abs(velocityY) > .72))
    : false
  const isVerticalExit = phonePortrait.value && !wasCancelled && !lightboxGestureHadPinch && wasSinglePan && !gesture.longPressed && gesture?.startDefaultView && duration < 780 && verticalExitThresholdMet && Math.abs(dy) > Math.abs(dx) * 1.08
  const isTap = !wasCancelled && !lightboxGestureHadPinch && wasSinglePan && !gesture.longPressed && duration < 280 && Math.hypot(dx, dy) < 10

  clearLightboxLongPress()
  if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  lightboxPointers.delete(event.pointerId)
  if (isVerticalExit) {
    animateMobileLightboxExit(dy < 0 ? -1 : 1)
  } else if (isSwipe && lightbox.value.media.length > 1) {
    const swipeOffset = isZoomEdgeSwipe ? gesture.edgeOverscroll : dx
    moveLightbox(swipeOffset < 0 ? 1 : -1)
  } else if (isTap) {
    const sinceLastTap = event.timeStamp - lightboxLastTap.time
    const nearLastTap = Math.hypot(event.clientX - lightboxLastTap.x, event.clientY - lightboxLastTap.y) < 34
    if (sinceLastTap > 0 && sinceLastTap < 340 && nearLastTap) {
      clearLightboxSingleTap()
      toggleLightboxDoubleTap(event)
      lightboxLastTap = { time: 0, x: 0, y: 0 }
    } else {
      lightboxLastTap = { time: event.timeStamp, x: event.clientX, y: event.clientY }
      scheduleLightboxSingleTapClose()
    }
  } else if (phonePortrait.value && gesture?.startDefaultView) {
    snapMobileLightboxTrack()
    mobileLightboxExitDragging.value = false
    mobileLightboxExitAnimating.value = true
    mobileLightboxExitY.value = 0
    scheduleTransient(() => { mobileLightboxExitAnimating.value = false }, 280)
  } else if (phonePortrait.value && wasSinglePan && !lightboxGestureHadPinch) {
    if (mobileLightboxDragX.value) snapMobileLightboxTrack()
    startMobileLightboxInertia(velocityX, velocityY)
  }

  if (lightboxPointers.size >= 2) beginLightboxPinch()
  else if (lightboxPointers.size === 1) beginLightboxPan([...lightboxPointers.values()][0])
  else {
    lightboxGesture = null
    lightboxGestureHadPinch = false
    if (phonePortrait.value && isMobileLightboxDefaultView()) {
      lightbox.value.dragging = false
      lightbox.value.x = 0
      lightbox.value.y = 0
    } else if (!mobileLightboxInertiaFrame) {
      settleMobileLightboxPan()
    }
  }
}
function closeMobileNavigationOnInputFocus() {
  timelineSearchFocused.value = true
  if (phonePortrait.value) {
    mobileMenuOpen.value = false
    mobileSourcesOpen.value = false
  }
}
function toggleMobileTimelineShortcut() {
  mobileMenuOpen.value = false
  mobileSourcesOpen.value = !mobileSourcesOpen.value
}
function navigateToMobileSource(source) {
  mobilePageSwitching.value = true
  mobileTimelineIconSwitching.value = true
  mobileSourcesOpen.value = false
  navigateTo('source', source)
  scheduleTransient(() => { mobilePageSwitching.value = false; mobileTimelineIconSwitching.value = false }, 360)
}
function navigateToMobileAll() {
  mobilePageSwitching.value = true
  mobileTimelineIconSwitching.value = true
  mobileSourcesOpen.value = false
  navigateTo('all', 'all')
  scheduleTransient(() => { mobilePageSwitching.value = false; mobileTimelineIconSwitching.value = false }, 360)
}
function releaseTimelineSearchFocus() {
  timelineSearchFocused.value = false
}
function handleWindowResize() {
  feedListTopValid = false
  if (isMasonryView.value) masonryMetricsReady.value = false
  nextTick(() => { updateFeedListDocumentTop(); scheduleMasonryMetrics(); scheduleTimelineWindow() })
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
function clearPhoneOverlayHistoryForNavigation() {
  if (!phonePortrait.value || !phoneOverlayHistoryActive) return
  const state = { ...(window.history.state || {}) }
  delete state.lumicOverlay
  window.history.replaceState(state, '', window.location.href)
  phoneOverlayHistoryActive = ''
  phoneOverlayHistoryClosing = false
}
function dismissPhoneOverlay(kind = phoneOverlayKey.value) {
  phoneOverlayDismissInProgress = true
  if (kind === 'lightbox-menu') mobileLightboxMenu.value = { open: false, x: 0, y: 0 }
  else if (kind === 'confirm') closeConfirmDialog(false)
  else if (kind === 'feed-settings') showFeedSettings.value = false
  else if (kind === 'credential-platform') credentialPlatform.value = null
  else if (kind === 'platform') selectedPlatform.value = null
  else if (kind === 'bilibili') showBilibili.value = false
  else if (kind === 'weibo') showWeibo.value = false
  else if (kind === 'pixiv') showPixiv.value = false
  else if (kind === 'add-source') showAdd.value = false
  else if (kind === 'context-menu') closeContextMenu()
  else if (kind === 'mobile-menu') mobileMenuOpen.value = false
  else if (kind === 'mobile-sources') mobileSourcesOpen.value = false
  nextTick(() => { phoneOverlayDismissInProgress = false })
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
function toggleSelectAllLoadedPosts() {
  const loadedIDs = loadedSelectionPosts.value.map(post => post.id)
  if (!loadedIDs.length) return
  if (allLoadedPostsSelected.value) {
    const loadedIDSet = new Set(loadedIDs)
    selectedPostIds.value = selectedPostIds.value.filter(id => !loadedIDSet.has(id))
    return
  }
  selectedPostIds.value = [...new Set([...selectedPostIds.value, ...loadedIDs])]
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
async function addTwitterLikesSource() {
  sourceActionBusy.value = 'add:twitter-likes'; sourceActionMessage.value = ''; settingsError.value = ''
  try {
    const response = await fetch('/api/twitter/subscriptions?type=likes', { method: 'POST' })
    if (!response.ok) throw new Error(await responseError(response, response.status === 409 ? '推特账号点赞来源已添加' : '添加推特账号点赞来源失败'))
    const feed = await response.json(); await loadFeeds(feed)
    if (selectedPlatform.value?.key === 'twitter') selectedPlatform.value.feeds = feeds.value.filter(item => item.source === 'twitter')
    sourceActionMessage.value = '已添加“推特点赞”，可设置启停、Cron 和过滤规则'
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
  return primaryVideo(post) ? '' : (post.media?.[0] || '')
}
function masonryCoverIsVideo(post) {
  return false
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
  if (timelineView.value !== 'masonry') masonryMetricsReady.value = false
  timelineView.value = timelineView.value === 'list' ? 'masonry' : 'list'
  if (event?.detail > 0) event.currentTarget?.blur()
}
function openMasonryPost(post, event) {
  if (selectionMode.value) return
  resetMobileDetailPageSwipe()
  mobileMenuOpen.value = false
  mobileSourcesOpen.value = false
  if (phonePortrait.value) {
    const card = event?.currentTarget?.closest?.('.masonry-card') || event?.currentTarget
    const origin = card?.querySelector?.('.masonry-cover') || card
    const rect = origin?.getBoundingClientRect?.()
    mobilePostOriginVisual = rect?.width && rect?.height ? {
      postId: String(post.id),
      rect: { left: rect.left, top: rect.top, width: rect.width, height: rect.height },
      borderRadius: window.getComputedStyle(origin).borderRadius || '9px'
    } : null
  }
  masonryDetailPost.value = post
  mobileDetailIndex.value = 0
  resetMobileDetailTrack()
  if (phonePortrait.value) {
    const returnPath = `${window.location.pathname}${window.location.search}`
    mobilePostReturnPath.value = window.location.pathname.startsWith('/post/') ? '/' : returnPath
    mobilePostReturnScrollY.value = window.scrollY
    if (!returnPath.startsWith('/author/')) mobileAuthorDetailState.value = null
    updateRoute(`/post/${encodeURIComponent(post.id)}`, false, { lumicMobileDetail: true, lumicReturnPath: returnPath })
    window.scrollTo({ top: 0, behavior: 'auto' })
  }
}
function restoreMobileScroll(top = 0) {
  const target = Math.max(0, Number(top) || 0)
  nextTick(() => {
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => window.scrollTo({ top: target, behavior: 'auto' }))
    })
  })
}
function cleanupMobileDetailRouteExit() {
  if (mobileDetailRouteExitTarget) mobileDetailRouteExitTarget.style.visibility = ''
  mobileDetailRouteExitTarget = null
  mobileDetailRouteExitLayer?.remove()
  mobileDetailRouteExitLayer = null
}
function captureMobileDetailRouteExit(post) {
  if (!phonePortrait.value || !post) return null
  const page = document.querySelector('.mobile-post-detail-page')
  const stage = page?.querySelector('.mobile-post-media-stage')
  const mediaElement = stage?.querySelector('.mobile-post-media-slide.current img, .mobile-post-media-slide.current video')
  if (!page || !stage || !mediaElement) return null
  cleanupMobileDetailRouteExit()
  const pageRect = page.getBoundingClientRect()
  const stageRect = stage.getBoundingClientRect()
  const offsetX = mobileDetailPageDragX.value || 0
  const offsetY = mobileDetailPageDragY.value || 0
  const sourceRect = {
    left: stageRect.left - offsetX,
    top: stageRect.top - offsetY,
    width: stageRect.width,
    height: stageRect.height
  }
  const layer = document.createElement('div')
  layer.className = `mobile-detail-route-exit-layer phone-ui${isDark.value ? ' dark' : ''}`
  layer.setAttribute('aria-hidden', 'true')
  layer.inert = true
  const shell = document.createElement('div')
  shell.className = 'mobile-detail-route-exit-shell'
  shell.style.setProperty('--mobile-detail-exit-page-top', `${pageRect.top - offsetY}px`)
  const pageClone = page.cloneNode(true)
  pageClone.style.transform = 'none'
  pageClone.style.opacity = '1'
  pageClone.style.transition = 'none'
  const clonedStage = pageClone.querySelector('.mobile-post-media-stage')
  if (clonedStage) clonedStage.style.visibility = 'hidden'
  pageClone.querySelectorAll('video').forEach(video => { video.autoplay = false; video.controls = false; video.removeAttribute('src') })
  shell.appendChild(pageClone)
  const media = document.createElement('div')
  media.className = 'mobile-detail-route-exit-media'
  Object.assign(media.style, {
    left: `${sourceRect.left}px`,
    top: `${sourceRect.top}px`,
    width: `${sourceRect.width}px`,
    height: `${sourceRect.height}px`,
    background: window.getComputedStyle(stage).backgroundColor
  })
  let mediaClone
  if (mediaElement.tagName === 'VIDEO' && mediaElement.poster) {
    mediaClone = document.createElement('img')
    mediaClone.src = mediaElement.poster
    mediaClone.alt = ''
  } else {
    mediaClone = mediaElement.cloneNode(true)
    if (mediaClone.tagName === 'VIDEO') {
      mediaClone.autoplay = false
      mediaClone.controls = false
      mediaClone.muted = true
    }
  }
  media.appendChild(mediaClone)
  layer.append(shell, media)
  document.body.appendChild(layer)
  mobileDetailRouteExitLayer = layer
  return { layer, shell, media, postId: String(post.id), sourceRect }
}
function findMobileDetailReturnTarget(postId) {
  const card = [...document.querySelectorAll('.masonry-card[data-post-id], .post-card[data-post-id]')].find(element => element.dataset.postId === String(postId))
  if (!card) return null
  const element = card.querySelector('.masonry-cover, .media-grid') || card
  const rect = element.getBoundingClientRect()
  if (!rect.width || !rect.height) return null
  return { element, rect, borderRadius: window.getComputedStyle(element).borderRadius || '9px' }
}
function runMobileDetailRouteExit(snapshot) {
  if (!snapshot?.layer?.isConnected) return
  let attempts = 0
  let stableFrames = 0
  let previousSignature = ''
  const animate = () => {
    if (!snapshot.layer.isConnected) return
    const foundTarget = findMobileDetailReturnTarget(snapshot.postId)
    const storedTarget = mobilePostOriginVisual?.postId === snapshot.postId ? mobilePostOriginVisual : null
    const expectedScroll = Math.max(0, Number(mobilePostReturnScrollY.value) || 0)
    const scrollReady = Math.abs(window.scrollY - expectedScroll) < 3
    const targetRect = foundTarget?.rect || storedTarget?.rect || null
    const signature = targetRect && scrollReady
      ? [window.scrollY, targetRect.left, targetRect.top, targetRect.width, targetRect.height].map(value => Math.round(value * 10) / 10).join(':')
      : ''
    stableFrames = signature && signature === previousSignature ? stableFrames + 1 : 0
    previousSignature = signature
    if ((!targetRect || !scrollReady || stableFrames < 2) && attempts < 36) {
      attempts += 1
      window.requestAnimationFrame(animate)
      return
    }
    const finalTargetRect = targetRect || snapshot.sourceRect
    const targetRadius = foundTarget?.borderRadius || storedTarget?.borderRadius || '9px'
    const translateX = finalTargetRect.left - snapshot.sourceRect.left
    const translateY = finalTargetRect.top - snapshot.sourceRect.top
    const scaleX = finalTargetRect.width / Math.max(1, snapshot.sourceRect.width)
    const scaleY = finalTargetRect.height / Math.max(1, snapshot.sourceRect.height)
    if (foundTarget?.element) {
      mobileDetailRouteExitTarget = foundTarget.element
      foundTarget.element.style.visibility = 'hidden'
    }
    if (typeof snapshot.media.animate !== 'function' || typeof snapshot.shell.animate !== 'function') {
      cleanupMobileDetailRouteExit()
      return
    }
    const easing = 'cubic-bezier(.22,.61,.36,1)'
    const targetRadiusValue = Number.parseFloat(targetRadius) || 9
    const shellAnimation = snapshot.shell.animate([
      { opacity: 1, transform: 'scale(1)' },
      { opacity: 1, transform: 'scale(1)', offset: .72 },
      { opacity: 0, transform: 'scale(.998)' }
    ], { duration: 300, easing, fill: 'forwards' })
    const mediaAnimation = snapshot.media.animate([
      { transform: 'translate3d(0,0,0) scale(1,1)', borderRadius: '0px', boxShadow: '0 0 0 rgba(0,0,0,0)' },
      { transform: `translate3d(${translateX}px,${translateY}px,0) scale(${scaleX},${scaleY})`, borderRadius: targetRadius || `${targetRadiusValue}px`, boxShadow: '0 8px 24px rgba(0,0,0,.14)' }
    ], { duration: 300, easing, fill: 'forwards' })
    Promise.allSettled([shellAnimation.finished, mediaAnimation.finished]).then(() => {
      cleanupMobileDetailRouteExit()
    })
  }
  nextTick(() => window.requestAnimationFrame(() => window.requestAnimationFrame(animate)))
}
function closePostDetail() {
  if (phonePortrait.value && window.location.pathname.startsWith('/post/') && window.history.state?.lumicMobileDetail) {
    pendingPostId.value = ''
    window.history.back()
    return
  }
  mobileDetailGestureReturnPending = false
  resetMobileDetailPageSwipe()
  masonryDetailPost.value = null
  mobileDetailIndex.value = 0
  resetMobileDetailTrack()
  pendingPostId.value = ''
  if (window.location.pathname.startsWith('/post/')) {
    updateRoute(mobilePostReturnPath.value || '/', true)
    restoreMobileScroll(mobilePostReturnScrollY.value)
  }
}
function startMobileAuthorPreviewHandoff(post) {
  if (mobileAuthorHandoffTimer) window.clearTimeout(mobileAuthorHandoffTimer)
  void preloadPostAvatar(post)
  warmPostAvatars(mobileAuthorTimelinePosts.value, 24)
  mobileAuthorPreviewPost.value = post
  mobileAuthorPreviewHandoff.value = true
  mobileAuthorPreviewFading.value = false
  let attempts = 0
  let stableFrames = 0
  let previousSignature = ''
  const targetScrollY = Math.max(0, mobileAuthorDetailState.value?.authorScrollY || 0)
  const waitForAuthorPage = () => {
    const cards = [...document.querySelectorAll('.mobile-author-detail-page .masonry-card')]
    const viewportCards = cards.filter(card => card.getBoundingClientRect().top < window.innerHeight + 80)
    const authorCardsReady = masonryMetricsReady.value
      && viewportCards.length > 0
      && viewportCards.every(card => !masonryMediaPending(card))
      && Math.abs(window.scrollY - targetScrollY) < 2
    const headerAvatar = document.querySelector('.mobile-author-detail-page .author-page-header .author-profile-main > img')
    const avatarReady = !headerAvatar || (headerAvatar.complete && headerAvatar.naturalWidth > 0)
    const feed = document.querySelector('.mobile-author-detail-page .masonry-feed')
    const signature = authorCardsReady && avatarReady && feed
      ? [window.scrollY, feed.getBoundingClientRect().height, ...viewportCards.slice(0, 8).flatMap(card => {
          const rect = card.getBoundingClientRect()
          return [rect.left, rect.top, rect.width, rect.height].map(value => Math.round(value * 10) / 10)
        })].join(':')
      : ''
    if (signature && signature === previousSignature) stableFrames += 1
    else stableFrames = 0
    previousSignature = signature
    if ((!authorCardsReady || !avatarReady || stableFrames < 3) && attempts < 72) {
      attempts += 1
      window.requestAnimationFrame(waitForAuthorPage)
      return
    }
    window.requestAnimationFrame(() => {
      mobileAuthorPreviewHandoff.value = false
      mobileAuthorPreviewFading.value = false
      mobileAuthorPreviewPost.value = null
      mobileAuthorPreviewSnapshot.value = { active: false, items: [], height: 0, scrollY: 0 }
    })
  }
  nextTick(() => window.requestAnimationFrame(waitForAuthorPage))
}
function startMobileDetailReturnHandoff() {
  if (mobileDetailReturnHandoffTimer) window.clearTimeout(mobileDetailReturnHandoffTimer)
  mobileDetailReturnHandoff.value = true
  mobileDetailReturnFading.value = false
  let attempts = 0
  let stableFrames = 0
  let previousSignature = ''
  const targetScrollY = Math.max(0, mobileAuthorDetailState.value?.detailScrollY || 0)
  const waitForDetailPage = () => {
    const page = document.querySelector('.mobile-post-detail-page')
    const currentMedia = page?.querySelector('.mobile-post-media-slide.current img, .mobile-post-media-slide.current video')
    const mediaReady = !currentMedia
      || (currentMedia.tagName === 'IMG' ? currentMedia.complete && currentMedia.naturalWidth > 0 : currentMedia.readyState >= 1)
    const head = page?.querySelector('.mobile-post-detail-head')?.getBoundingClientRect()
    const stage = page?.querySelector('.mobile-post-media-stage')?.getBoundingClientRect()
    const signature = page && mediaReady && Math.abs(window.scrollY - targetScrollY) < 2
      ? [window.scrollY, page.scrollHeight, head?.height || 0, stage?.top || 0, stage?.width || 0, stage?.height || 0]
          .map(value => Math.round(value * 10) / 10).join(':')
      : ''
    if (signature && signature === previousSignature) stableFrames += 1
    else stableFrames = 0
    previousSignature = signature
    if ((!page || !mediaReady || stableFrames < 3) && attempts < 72) {
      attempts += 1
      window.requestAnimationFrame(waitForDetailPage)
      return
    }
    window.requestAnimationFrame(() => {
      mobileDetailReturnHandoff.value = false
      mobileDetailReturnFading.value = false
    })
  }
  nextTick(() => window.requestAnimationFrame(waitForDetailPage))
}
function startMobileTimelineReturnHandoff(post) {
  if (!post) return
  const clearAuthorState = mobileAuthorDetailState.value?.returnToDetail === false
  if (mobileTimelineReturnHandoffTimer) window.clearTimeout(mobileTimelineReturnHandoffTimer)
  mobileTimelineReturnPreviewPost.value = post
  mobileTimelineReturnHandoff.value = true
  mobileTimelineReturnFading.value = false
  nextTick(() => window.requestAnimationFrame(() => window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
    mobileTimelineReturnFading.value = true
    mobileTimelineReturnHandoffTimer = window.setTimeout(() => {
      mobileTimelineReturnHandoffTimer = 0
      mobileTimelineReturnHandoff.value = false
      mobileTimelineReturnFading.value = false
      mobileTimelineReturnPreviewPost.value = null
      if (clearAuthorState) mobileAuthorDetailState.value = null
    }, 190)
  }))))
}
function openMobileDetailAuthor(post) {
  if (!post) return
  const query = ''
  const authorPath = `/author/${post.source}/${encodeURIComponent(post.author)}${query}`
  const previous = mobileAuthorDetailState.value
  const state = previous?.post?.id === post.id
    ? previous
    : { post, authorPath, authorScrollY: 0, detailScrollY: 0, returnToDetail: true }
  state.returnToDetail = true
  state.detailScrollY = window.scrollY
  mobileAuthorDetailState.value = state
  mobileAuthorScrollY = state.authorScrollY
  captureMobileAuthorPreviewSnapshot(state.authorScrollY)
  startMobileAuthorPreviewHandoff(post)
  openAuthorPage(post.author, post.source, post.avatar, '', true)
  restoreMobileScroll(state.authorScrollY)
}
function openMobileDetailFromAuthor() {
  const state = mobileAuthorDetailState.value
  if (!state?.post) return
  const post = posts.value.find(item => item.id === state.post.id) || state.post
  resetMobileDetailPageSwipe()
  masonryDetailPost.value = post
  mobileDetailIndex.value = 0
  resetMobileDetailTrack()
  activeNav.value = 'all'
  activeSource.value = 'all'
  selectedAuthor.value = null
  updateRoute(`/post/${encodeURIComponent(post.id)}`, true, { lumicMobileDetail: true, lumicMobileAuthor: false, lumicReturnPath: mobilePostReturnPath.value })
  restoreMobileScroll(state.detailScrollY)
}
function returnToMobileDetailFromAuthor() {
  if (window.history.state?.lumicMobileAuthor || window.history.state?.lumicMobileTag) {
    window.history.back()
    return
  }
  openMobileDetailFromAuthor()
}
function clearMobileDetailAnimation() {
  if (mobileDetailAnimationTimer) window.clearTimeout(mobileDetailAnimationTimer)
  if (mobileDetailAnimationFrame) window.cancelAnimationFrame(mobileDetailAnimationFrame)
  mobileDetailAnimationTimer = 0
  mobileDetailAnimationFrame = 0
}
function finishMobileDetailTransition(commit = true) {
  const transition = mobileDetailTransition
  clearMobileDetailAnimation()
  mobileDetailTransition = null
  mobileDetailAnimating.value = false
  mobileDetailDragging.value = false
  mobileDetailDragX.value = 0
  mobileDetailTrackShift.value = 0
  if (commit && transition && mobileDetailMedia.value.length > 1) {
    const count = mobileDetailMedia.value.length
    mobileDetailIndex.value = transition.targetIndex == null
      ? (mobileDetailIndex.value + transition.step + count) % count
      : transition.targetIndex
  }
  mobileDetailSwipeClickBlocked = false
}
function resetMobileDetailTrack() {
  clearMobileDetailAnimation()
  mobileDetailDragX.value = 0
  mobileDetailTrackShift.value = 0
  mobileDetailDragging.value = false
  mobileDetailAnimating.value = false
  mobileDetailTransition = null
  mobileDetailTouch = null
}
function moveMobileDetailMedia(direction, targetIndex = null) {
  const count = mobileDetailMedia.value.length
  if (count < 2) return
  if (mobileDetailAnimating.value) finishMobileDetailTransition(true)
  const step = direction > 0 ? 1 : -1
  mobileDetailSwipeClickBlocked = true
  mobileDetailDragging.value = false
  mobileDetailAnimating.value = true
  mobileDetailTransition = { step, targetIndex }
  mobileDetailAnimationFrame = window.requestAnimationFrame(() => {
    mobileDetailAnimationFrame = 0
    mobileDetailDragX.value = 0
    mobileDetailTrackShift.value = step > 0 ? -100 : 100
  })
  mobileDetailAnimationTimer = window.setTimeout(() => finishMobileDetailTransition(true), 265)
}
function selectMobileDetailMedia(index) {
  if (mobileDetailAnimating.value) finishMobileDetailTransition(true)
  if (index === mobileDetailIndex.value) return
  const count = mobileDetailMedia.value.length
  const forwardDistance = (index - mobileDetailIndex.value + count) % count
  const backwardDistance = (mobileDetailIndex.value - index + count) % count
  moveMobileDetailMedia(forwardDistance <= backwardDistance ? 1 : -1, index)
}
function openMobileDetailImage() {
  if (mobileDetailSwipeClickBlocked || mobileDetailCurrentMedia.value?.type !== 'image') {
    mobileDetailSwipeClickBlocked = false
    return
  }
  openLightbox(masonryDetailPost.value, mobileDetailIndex.value)
}
function mobileGesturePoint(event) {
  const touch = event.touches?.[0] || event.changedTouches?.[0]
  if (touch) return touch
  return event.pointerType === 'touch' ? event : null
}
function beginMobileDetailSwipe(event) {
  const touch = mobileGesturePoint(event)
  if (!touch || mobileDetailMedia.value.length < 2) return
  if (mobileDetailAnimating.value) finishMobileDetailTransition(true)
  mobileDetailSwipeClickBlocked = false
  mobileDetailDragX.value = 0
  mobileDetailTrackShift.value = 0
  mobileDetailDragging.value = true
  mobileDetailTouch = { x: touch.clientX, y: touch.clientY, time: event.timeStamp, prevX: touch.clientX, prevTime: event.timeStamp, lastX: touch.clientX, lastTime: event.timeStamp, axis: '' }
}
function updateMobileDetailSwipe(event) {
  const touch = mobileGesturePoint(event)
  if (!touch || !mobileDetailTouch) return
  const deltaX = touch.clientX - mobileDetailTouch.x
  const deltaY = touch.clientY - mobileDetailTouch.y
  if (!mobileDetailTouch.axis && Math.max(Math.abs(deltaX), Math.abs(deltaY)) > 7) {
    mobileDetailTouch.axis = Math.abs(deltaX) > Math.abs(deltaY) * 1.08 ? 'horizontal' : 'vertical'
  }
  if (mobileDetailTouch.axis !== 'horizontal') return
  mobileDetailTouch.prevX = mobileDetailTouch.lastX
  mobileDetailTouch.prevTime = mobileDetailTouch.lastTime
  mobileDetailTouch.lastX = touch.clientX
  mobileDetailTouch.lastTime = event.timeStamp
  mobileDetailSwipeClickBlocked = Math.abs(deltaX) > 7
  const limit = Math.max(90, window.innerWidth * .82)
  mobileDetailDragX.value = Math.max(-limit, Math.min(limit, deltaX))
}
function finishMobileDetailSwipe(event) {
  const touch = mobileGesturePoint(event)
  if (!touch || !mobileDetailTouch) return
  const deltaX = touch.clientX - mobileDetailTouch.x
  const deltaY = touch.clientY - mobileDetailTouch.y
  const duration = event.timeStamp - mobileDetailTouch.time
  const velocityDuration = Math.max(1, mobileDetailTouch.lastTime - mobileDetailTouch.prevTime)
  const velocityX = (mobileDetailTouch.lastX - mobileDetailTouch.prevX) / velocityDuration
  const horizontal = mobileDetailTouch.axis === 'horizontal'
  mobileDetailTouch = null
  mobileDetailDragging.value = false
  if (horizontal && duration < 900 && (Math.abs(deltaX) >= Math.min(54, window.innerWidth * .13) || Math.abs(velocityX) > .4) && Math.abs(deltaX) > Math.abs(deltaY) * 1.05) {
    moveMobileDetailMedia(deltaX < 0 ? 1 : -1)
    return
  }
  clearMobileDetailAnimation()
  mobileDetailAnimating.value = true
  mobileDetailDragX.value = 0
  mobileDetailAnimationTimer = window.setTimeout(() => {
    mobileDetailAnimationTimer = 0
    mobileDetailAnimating.value = false
    mobileDetailSwipeClickBlocked = false
  }, 205)
}
function cancelMobileDetailSwipe() {
  if (!mobileDetailTouch) return
  mobileDetailTouch = null
  mobileDetailDragging.value = false
  clearMobileDetailAnimation()
  mobileDetailAnimating.value = true
  mobileDetailDragX.value = 0
  mobileDetailAnimationTimer = window.setTimeout(() => {
    mobileDetailAnimationTimer = 0
    mobileDetailAnimating.value = false
    mobileDetailSwipeClickBlocked = false
  }, 205)
}
function resetMobileDetailPageSwipe() {
  if (mobileDetailPageTimer) window.clearTimeout(mobileDetailPageTimer)
  mobileDetailPageTimer = 0
  mobileDetailPageTouch = null
  mobileDetailPageDragX.value = 0
  mobileDetailPageDragY.value = 0
  mobileDetailPageDragging.value = false
  mobileDetailPageAnimating.value = false
  mobileDetailPageTransitionMs.value = 320
  mobileDetailPageTransitionEasing.value = 'cubic-bezier(.2, .78, .18, 1)'
}
function mobileDetailAdjacentPost(step) {
  return step > 0 ? mobileDetailNextPost.value : mobileDetailPreviousPost.value
}
function preloadMobileDetailPost(post) {
  if (!post) return
  const cover = masonryCover(post) || primaryVideo(post)?.poster
  if (!cover) return
  const url = previewMedia(cover)
  if (!url || preloadedPreviewUrls.has(url)) return
  if (preloadedPreviewUrls.size >= 240) preloadedPreviewUrls.clear()
  preloadedPreviewUrls.add(url)
  const image = new Image()
  image.decoding = 'async'
  image.fetchPriority = 'low'
  image.src = url
}
function mobileDetailPreviewMedia(post) {
  return postDetailMedia(post)[0] || null
}
function mobileDetailPreviewCover(post) {
  const media = mobileDetailPreviewMedia(post)
  return media?.type === 'image' ? media.src : (media?.poster || '')
}
function switchMobileDetailPost(post, step) {
  if (!post) return
  mobileDetailPageAnimating.value = false
  mobileDetailPageDragging.value = false
  masonryDetailPost.value = post
  mobileDetailIndex.value = 0
  resetMobileDetailTrack()
  pendingPostId.value = ''
  updateRoute(`/post/${encodeURIComponent(post.id)}`, true)
  window.scrollTo({ top: 0, behavior: 'auto' })
  mobileDetailPageDragX.value = 0
  mobileDetailPageDragY.value = 0
  nextTick(() => {
    window.requestAnimationFrame(() => {
      resetMobileDetailPageSwipe()
      mobileDetailSwipeClickBlocked = false
    })
  })
}
function commitMobileDetailVerticalSwipe(step, velocityY) {
  const target = mobileDetailAdjacentPost(step)
  if (!target) return false
  preloadMobileDetailPost(target)
  mobileDetailPageDragging.value = false
  mobileDetailPageAnimating.value = true
  mobileDetailPageDragX.value = 0
  const viewportHeight = Math.max(1, window.innerHeight)
  const remaining = Math.max(0, 1 - Math.min(1, Math.abs(mobileDetailPageDragY.value) / viewportHeight))
  mobileDetailPageDragY.value = step > 0 ? -window.innerHeight : window.innerHeight
  const duration = Math.max(220, Math.min(340, 185 + remaining * 150 - Math.min(1.5, Math.abs(velocityY)) * 34))
  mobileDetailPageTransitionMs.value = duration
  mobileDetailPageTransitionEasing.value = 'cubic-bezier(.2, .78, .18, 1)'
  mobileDetailPageTimer = window.setTimeout(() => {
    mobileDetailPageTimer = 0
    switchMobileDetailPost(target, step)
  }, duration)
  return true
}
function beginMobileDetailPageSwipe(event) {
  const touch = mobileGesturePoint(event)
  const target = event.target
  if (!touch || !masonryDetailPost.value || target?.closest?.('button, a, input, textarea, select, label')) return
  const systemGestureEdge = Math.max(22, Math.min(30, window.innerWidth * .065))
  if (touch.clientX <= systemGestureEdge || touch.clientX >= window.innerWidth - systemGestureEdge) return
  if (mobileDetailPageTimer) window.clearTimeout(mobileDetailPageTimer)
  mobileDetailPageTimer = 0
  const maxScrollY = Math.max(0, document.documentElement.scrollHeight - window.innerHeight)
  mobileDetailPageTouch = {
    x: touch.clientX,
    y: touch.clientY,
    time: event.timeStamp,
    axis: '',
    targetIsMedia: Boolean(target?.closest?.('.mobile-post-media-stage')),
    verticalActive: false,
    verticalOriginY: touch.clientY,
    maxScrollY,
    prevX: touch.clientX,
    prevY: touch.clientY,
    prevTime: event.timeStamp,
    lastX: touch.clientX,
    lastY: touch.clientY,
    lastTime: event.timeStamp
  }
  mobileDetailPageDragging.value = false
  mobileDetailPageAnimating.value = false
  mobileDetailPageTransitionMs.value = 320
  mobileDetailPageTransitionEasing.value = 'cubic-bezier(.2, .78, .18, 1)'
  captureMobileAuthorPreviewSnapshot(mobileAuthorDetailState.value?.post?.id === masonryDetailPost.value.id ? mobileAuthorDetailState.value.authorScrollY : 0)
  preloadMobileDetailPost(mobileDetailPreviousPost.value)
  preloadMobileDetailPost(mobileDetailNextPost.value)
  mobileAuthorPreviewPosts.value.forEach(preloadMobileDetailPost)
  mobileReturnPreviewPosts.value.forEach(preloadMobileDetailPost)
}
function updateMobileDetailPageSwipe(event) {
  const touch = mobileGesturePoint(event)
  if (!touch || !mobileDetailPageTouch) return
  const dx = touch.clientX - mobileDetailPageTouch.x
  const dy = touch.clientY - mobileDetailPageTouch.y
  if (!mobileDetailPageTouch.axis && Math.max(Math.abs(dx), Math.abs(dy)) > 9) {
    mobileDetailPageTouch.axis = Math.abs(dx) > Math.abs(dy) * 1.12 ? 'horizontal' : 'vertical'
  }
  if (mobileDetailPageTouch.targetIsMedia && mobileDetailPageTouch.axis === 'horizontal') return
  if (mobileDetailPageTouch.axis === 'horizontal') {
    event.preventDefault?.()
    mobileDetailPageDragging.value = true
    mobileDetailPageTouch.prevX = mobileDetailPageTouch.lastX
    mobileDetailPageTouch.prevTime = mobileDetailPageTouch.lastTime
    mobileDetailPageTouch.lastX = touch.clientX
    mobileDetailPageTouch.lastTime = event.timeStamp
    mobileDetailPageDragX.value = dampBeyond(dx, window.innerWidth, .18)
    mobileDetailPageDragY.value = 0
    return
  }
  if (mobileDetailPageTouch.axis !== 'vertical') return
  const atTop = window.scrollY <= 1
  const atBottom = window.scrollY >= mobileDetailPageTouch.maxScrollY - 1
  const pullingPrevious = dy > 0 && atTop
  const pullingNext = dy < 0 && atBottom
  if (!mobileDetailPageTouch.verticalActive) {
    if (!pullingPrevious && !pullingNext) return
    mobileDetailPageTouch.verticalActive = true
    mobileDetailPageTouch.verticalOriginY = touch.clientY
    mobileDetailPageTouch.prevY = touch.clientY
    mobileDetailPageTouch.lastY = touch.clientY
    mobileDetailPageTouch.prevTime = event.timeStamp
    mobileDetailPageTouch.lastTime = event.timeStamp
  }
  const dragY = touch.clientY - mobileDetailPageTouch.verticalOriginY
  if ((dragY > 0 && !atTop) || (dragY < 0 && !atBottom)) return
  event.preventDefault?.()
  mobileDetailPageDragging.value = true
  mobileDetailSwipeClickBlocked = Math.abs(dragY) > 7
  mobileDetailPageTouch.prevY = mobileDetailPageTouch.lastY
  mobileDetailPageTouch.prevTime = mobileDetailPageTouch.lastTime
  mobileDetailPageTouch.lastY = touch.clientY
  mobileDetailPageTouch.lastTime = event.timeStamp
  const target = mobileDetailAdjacentPost(dragY < 0 ? 1 : -1)
  const viewportHeight = Math.max(1, window.innerHeight)
  const distance = Math.abs(dragY)
  const direction = Math.sign(dragY)
  const resistanceSpan = viewportHeight * (target ? .68 : .28)
  const resistedY = direction * resistanceSpan * (1 - Math.exp(-distance / resistanceSpan))
  mobileDetailPageDragX.value = 0
  mobileDetailPageDragY.value = resistedY
}
function finishMobileDetailPageSwipe(event) {
  const touch = mobileGesturePoint(event)
  if (!touch || !mobileDetailPageTouch) return
  const touchState = mobileDetailPageTouch
  const dx = touch.clientX - touchState.x
  const dy = touchState.verticalActive ? touch.clientY - touchState.verticalOriginY : touch.clientY - touchState.y
  const cancelled = event.type === 'touchcancel' || event.type === 'pointercancel'
  const horizontal = mobileDetailPageTouch.axis === 'horizontal'
  const velocityDuration = Math.max(1, mobileDetailPageTouch.lastTime - mobileDetailPageTouch.prevTime)
  const velocityX = (mobileDetailPageTouch.lastX - mobileDetailPageTouch.prevX) / velocityDuration
  const velocityY = (mobileDetailPageTouch.lastY - mobileDetailPageTouch.prevY) / velocityDuration
  mobileDetailPageTouch = null
  if (touchState.targetIsMedia && horizontal) {
    mobileDetailPageDragging.value = false
    mobileDetailPageAnimating.value = false
    mobileDetailPageDragX.value = 0
    return
  }
  mobileDetailPageDragging.value = false
  mobileDetailPageAnimating.value = true
  if (!cancelled && touchState.verticalActive) {
    const threshold = Math.min(196, window.innerHeight * .24)
    const fastVertical = Math.abs(dy) > 64 && Math.abs(velocityY) > .82
    const step = dy < 0 ? 1 : -1
    if ((Math.abs(dy) >= threshold || fastVertical) && commitMobileDetailVerticalSwipe(step, velocityY)) return
    mobileDetailPageTransitionMs.value = Math.max(210, Math.min(280, 220 + Math.abs(mobileDetailPageDragY.value) * .16))
    mobileDetailPageTransitionEasing.value = 'cubic-bezier(.22, 1, .36, 1)'
    mobileDetailPageDragY.value = 0
    mobileDetailPageTimer = window.setTimeout(() => {
      resetMobileDetailPageSwipe()
      mobileDetailSwipeClickBlocked = false
    }, mobileDetailPageTransitionMs.value)
    return
  }
  const threshold = Math.min(112, window.innerWidth * .28)
  const fastLeft = dx < -36 && velocityX < -.78
  const fastRight = dx > 36 && velocityX > .78
  if (!cancelled && horizontal && (dx < -threshold || fastLeft)) {
    const post = masonryDetailPost.value
    mobileDetailPageDragX.value = -window.innerWidth
    mobileDetailPageTimer = window.setTimeout(() => {
      mobileDetailPageTimer = 0
      if (post) {
        openMobileDetailAuthor(post)
      }
      resetMobileDetailPageSwipe()
    }, 320)
    return
  }
  if (!cancelled && horizontal && (dx > threshold || fastRight)) {
    mobileDetailGestureReturnPending = true
    mobileDetailPageDragX.value = window.innerWidth
    mobileDetailPageTimer = window.setTimeout(() => {
      mobileDetailPageTimer = 0
      closePostDetail()
    }, 320)
    return
  }
  mobileDetailPageDragX.value = 0
  mobileDetailPageDragY.value = 0
  mobileDetailPageTimer = window.setTimeout(resetMobileDetailPageSwipe, 320)
}
function resetMobileAuthorPageSwipe() {
  if (mobileAuthorPageTimer) window.clearTimeout(mobileAuthorPageTimer)
  mobileAuthorPageTimer = 0
  mobileAuthorPageTouch = null
  mobileAuthorPageDragX.value = 0
  mobileAuthorPageDragging.value = false
  mobileAuthorPageAnimating.value = false
}
function beginMobileAuthorPageSwipe(event) {
  const touch = event.touches?.[0]
  const target = event.target
  if (!phonePortrait.value || !touch || !mobileAuthorDetailState.value || (!authorProfile.value && !selectedTag.value) || target?.closest?.('input, textarea, select, label')) return
  if (mobileAuthorPageTimer) window.clearTimeout(mobileAuthorPageTimer)
  mobileAuthorPageTimer = 0
  mobileAuthorPageTouch = { x: touch.clientX, y: touch.clientY, time: event.timeStamp, axis: '', prevX: touch.clientX, prevTime: event.timeStamp, lastX: touch.clientX, lastTime: event.timeStamp }
  mobileAuthorPageDragging.value = false
  mobileAuthorPageAnimating.value = false
}
function updateMobileAuthorPageSwipe(event) {
  const touch = event.touches?.[0]
  if (!touch || !mobileAuthorPageTouch) return
  const dx = touch.clientX - mobileAuthorPageTouch.x
  const dy = touch.clientY - mobileAuthorPageTouch.y
  if (!mobileAuthorPageTouch.axis && Math.max(Math.abs(dx), Math.abs(dy)) > 9) {
    mobileAuthorPageTouch.axis = Math.abs(dx) > Math.abs(dy) * 1.12 ? 'horizontal' : 'vertical'
  }
  if (mobileAuthorPageTouch.axis !== 'horizontal' || dx <= 0) return
  event.preventDefault()
  mobileAuthorPageDragging.value = true
  mobileAuthorPageTouch.prevX = mobileAuthorPageTouch.lastX
  mobileAuthorPageTouch.prevTime = mobileAuthorPageTouch.lastTime
  mobileAuthorPageTouch.lastX = touch.clientX
  mobileAuthorPageTouch.lastTime = event.timeStamp
  mobileAuthorPageDragX.value = dampBeyond(dx, window.innerWidth, .18)
}
function finishMobileAuthorPageSwipe(event) {
  const touch = event.changedTouches?.[0]
  if (!touch || !mobileAuthorPageTouch) return
  const touchState = mobileAuthorPageTouch
  const dx = touch.clientX - touchState.x
  const cancelled = event.type === 'touchcancel'
  const horizontal = touchState.axis === 'horizontal'
  const velocityDuration = Math.max(1, touchState.lastTime - touchState.prevTime)
  const velocityX = (touchState.lastX - touchState.prevX) / velocityDuration
  mobileAuthorPageTouch = null
  mobileAuthorPageDragging.value = false
  mobileAuthorPageAnimating.value = true
  const threshold = Math.min(112, window.innerWidth * .28)
  const fastReturn = dx > 36 && velocityX > .78
  if (!cancelled && horizontal && (dx > threshold || fastReturn)) {
    if (mobileAuthorDetailState.value) mobileAuthorDetailState.value.authorScrollY = mobileAuthorScrollY
    mobileAuthorPageDragX.value = window.innerWidth
    mobileAuthorPageTimer = window.setTimeout(() => {
      mobileAuthorPageTimer = 0
      returnToMobileDetailFromAuthor()
    }, 320)
    return
  }
  mobileAuthorPageDragX.value = 0
  mobileAuthorPageTimer = window.setTimeout(resetMobileAuthorPageSwipe, 320)
}
function masonryItemStyle(item) {
  return {
    left: `${item.x}px`,
    top: `${item.y}px`,
    width: `${masonryColumnWidth.value}px`
  }
}
function previewMedia(media) {
  const value = String(media || '')
  if (!value.startsWith('/flow/')) return value
  const configuredLevel = phonePortrait.value ? previewQuality.value.mobile : previewQuality.value.desktop
  const level = normalizePreviewQualityLevel(configuredLevel, phonePortrait.value ? 3 : 2)
  if (level === 0) return value
  const path = value.split('?', 1)[0]
  const adaptive = level === 5 ? `&network=${previewNetworkHint()}` : ''
  return `/preview/${path.slice('/flow/'.length)}?v=q2&level=${level}&device=${phonePortrait.value ? 'mobile' : 'desktop'}${adaptive}`
}
function videoPreviewSource(video) {
  const value = String(video?.url || '')
  if (!value || value.includes('#t=')) return value
  return `${value}#t=0.12`
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
  for (const id of masonryColumnAssignments.keys()) {
    if (!activeIds.has(id)) masonryColumnAssignments.delete(id)
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
  clearPhoneOverlayHistoryForNavigation()
  resetMobileDetailPageSwipe()
  resetMobileAuthorPageSwipe()
  mobileAuthorDetailState.value = null
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
  mobileSourcesOpen.value = false
  const path = nav === 'source' ? `/source/${source}` : nav === 'liked' ? '/favorites' : nav === 'pulls' ? '/pulls' : '/'
  updateRoute(path)
  if (nav === 'pulls') void Promise.all([loadFeeds(), loadPlatformAccounts()])
}
function openTag(tag) {
  clearPhoneOverlayHistoryForNavigation()
  const fromDetail = phonePortrait.value && Boolean(masonryDetailPost.value)
  const previousPath = window.location.pathname + window.location.search
  const previousPost = masonryDetailPost.value
  const previousTimelineIds = filteredPosts.value.map(item => item.id)
  stopSelection()
  masonryDetailPost.value = null
  if (phonePortrait.value) {
    mobileAuthorDetailState.value = {
      post: previousPost || filteredPosts.value[0] || { source: activeSource.value === 'all' ? 'bilibili' : activeSource.value, author: '', avatar: '' },
      tag,
      authorScrollY: 0,
      detailScrollY: window.scrollY,
      returnPath: previousPath,
      returnTitle: activeSource.value === 'all' ? '全部动态' : (sourceMeta[activeSource.value]?.label || '动态'),
      returnPostIds: previousTimelineIds,
      returnToDetail: fromDetail
    }
  } else {
    mobileAuthorDetailState.value = null
  }
  showSettings.value = false
  selectedAuthor.value = null
  selectedTag.value = tag
  activeNav.value = 'tag'
  activeSource.value = 'all'
  mobileSourcesOpen.value = false
  updateRoute(`/tag/${encodeURIComponent(tag)}`, false, phonePortrait.value ? (fromDetail ? { lumicMobileTag: true, lumicMobileDetailParent: previousPath } : { lumicMobileTag: true }) : {})
  window.scrollTo({ top: 0, behavior: 'auto' })
}
function masonryMediaPending(element) {
  const image = element.querySelector('.masonry-cover img')
  if (image && (!image.complete || !image.naturalWidth)) return true
  const video = element.querySelector('.masonry-cover video')
  return Boolean(video && video.readyState < 1)
}
function measurePostElement(post, element, layout = element?.dataset.layout || 'list', measuredHeight = 0) {
  if (layout === 'masonry' && masonryMediaPending(element)) return
  const height = Math.ceil(measuredHeight || element.getBoundingClientRect().height)
  const heights = layout === 'masonry' ? masonryHeights : timelineHeights
  const previousHeight = heights.value[post.id]
  if (height > 0 && (!previousHeight || Math.abs(previousHeight - height) > 1)) heights.value[post.id] = height
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
  measurePostElement(post, element, layout)
  postResizeObserver?.observe(element)
}
function updateMasonryMetrics(measuredWidth = 0) {
  if (!isMasonryView.value || !feedListElement.value) return
  const availableWidth = Math.max(0, Number(measuredWidth) || feedListElement.value.getBoundingClientRect().width || feedListElement.value.clientWidth)
  if (!availableWidth) return
  const gap = phonePortrait.value ? 8 : 14
  const count = phonePortrait.value ? 2 : Math.min(5, Math.max(3, Math.floor((availableWidth + gap) / (252 + gap))))
  const width = Math.max(0, (availableWidth - gap * (count - 1)) / count)
  const widthChanged = Math.abs(masonryColumnWidth.value - width) > 0.5
  const countChanged = masonryColumnCount.value !== count
  if (widthChanged || countChanged) {
    masonryHeights.value = {}
    masonryColumnAssignments.clear()
  }
  if (masonryGap.value !== gap) masonryGap.value = gap
  if (countChanged) masonryColumnCount.value = count
  if (widthChanged) masonryColumnWidth.value = width
  masonryMetricsReady.value = true
}
function scheduleMasonryMetrics(measuredWidth = 0) {
  if (measuredWidth > 0) pendingMasonryWidth = measuredWidth
  if (masonryMetricsFrame) return
  masonryMetricsFrame = window.requestAnimationFrame(() => {
    masonryMetricsFrame = 0
    const width = pendingMasonryWidth
    pendingMasonryWidth = 0
    updateMasonryMetrics(width)
  })
}
function initializeFeedListResizeObserver() {
  feedListResizeObserver = new ResizeObserver(entries => {
    const current = feedListElement.value
    for (const entry of entries) {
      if (entry.target === current) scheduleMasonryMetrics(entry.contentRect.width)
    }
  })
  if (feedListElement.value) {
    feedListResizeObserver.observe(feedListElement.value)
    scheduleMasonryMetrics()
  }
}
function updateFeedListDocumentTop() {
  if (!feedListElement.value) {
    feedListDocumentTop = 0
    feedListTopValid = false
    return
  }
  feedListDocumentTop = feedListElement.value.getBoundingClientRect().top + window.scrollY
  feedListTopValid = true
}
function timelineOffsetIndex(offsets, target) {
  let low = 0
  let high = offsets.length
  while (low < high) {
    const middle = (low + high) >>> 1
    if (offsets[middle] < target) low = middle + 1
    else high = middle
  }
  return low
}
function updateTimelineWindow() {
  timelineFrame = 0
  showScrollTop.value = isTimelinePage.value && window.scrollY > Math.min(360, Math.max(150, window.innerHeight * .32))
  if (!filteredPosts.value.length || showSettings.value || activeNav.value === 'pulls') return
  if (!feedListTopValid) updateFeedListDocumentTop()
  if (isMasonryView.value) {
    masonryViewportTop.value = Math.max(0, window.scrollY - feedListDocumentTop)
    masonryViewportBottom.value = masonryViewportTop.value + window.innerHeight
    return
  }
  const viewportTop = Math.max(0, window.scrollY - feedListDocumentTop)
  const viewportBottom = viewportTop + window.innerHeight
  const offsets = timelineOffsets.value
  const first = Math.max(0, Math.min(filteredPosts.value.length - 1, timelineOffsetIndex(offsets, viewportTop) - 1))
  const last = Math.max(first + 1, Math.min(filteredPosts.value.length, timelineOffsetIndex(offsets, viewportBottom)))
  const overscan = phonePortrait.value ? 2 : timelineOverscan
  timelineStart.value = Math.max(0, first - overscan)
  timelineEnd.value = Math.min(filteredPosts.value.length, last + overscan)
}
function scheduleTimelineWindow() {
  if (!timelineFrame) timelineFrame = window.requestAnimationFrame(updateTimelineWindow)
}
function hideMobileControlsAfterIdle() {
  mobileControlsTimer = 0
  const remaining = 2200 - (performance.now() - mobileControlsLastActivity)
  if (remaining > 0) {
    mobileControlsTimer = window.setTimeout(hideMobileControlsAfterIdle, remaining)
    return
  }
  if (!mobileMenuOpen.value && !mobileSourcesOpen.value) mobileControlsVisible.value = false
}
function showMobileControls() {
  if (!phonePortrait.value || lightbox.value.open || masonryDetailPost.value) return
  mobileControlsLastActivity = performance.now()
  mobileControlsVisible.value = true
  if (!mobileControlsTimer) mobileControlsTimer = window.setTimeout(hideMobileControlsAfterIdle, 2200)
}
function handleWindowScroll() {
  if (phonePortrait.value && authorProfile.value && mobileAuthorDetailState.value) mobileAuthorScrollY = window.scrollY
  showMobileControls()
  scheduleTimelineWindow()
}
function scrollTimelineToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
function resetTimelineWindow() {
  timelineStart.value = 0
  timelineEnd.value = Math.min(filteredPosts.value.length, phonePortrait.value ? 7 : 15)
  masonryViewportTop.value = 0
  masonryViewportBottom.value = typeof window === 'undefined' ? 900 : window.innerHeight
  masonryColumnAssignments.clear()
  masonryAssignmentColumnCount = 0
  feedListTopValid = false
  nextTick(() => { updateFeedListDocumentTop(); scheduleMasonryMetrics(); scheduleTimelineWindow() })
}
function isPhonePortraitScreen() {
  return window.matchMedia('(max-width: 760px)').matches
}
function updatePhonePortrait() {
  const nextPhonePortrait = isPhonePortraitScreen()
  if (phonePortrait.value === nextPhonePortrait) return
  masonryMetricsReady.value = false
  phonePortrait.value = nextPhonePortrait
  mobileControlsVisible.value = true
  mobileMenuOpen.value = false
  mobileSourcesOpen.value = false
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
function openAuthorPage(name, source, avatar = '', feedId = '', fromMobileDetail = false) {
  clearPhoneOverlayHistoryForNavigation()
  mobileSourcesOpen.value = false
  mobileMenuOpen.value = false
  selectedPlatform.value = null
  masonryDetailPost.value = null
  if (!fromMobileDetail) mobileAuthorDetailState.value = null
  showSettings.value = false
  activeNav.value = 'author'
  activeSource.value = source
  selectedAuthor.value = { name, source, avatar, feedId }
  const query = feedId ? `?feed=${encodeURIComponent(feedId)}` : ''
  const routeState = fromMobileDetail ? { lumicMobileAuthor: true, lumicParentPost: `/post/${encodeURIComponent(mobileAuthorDetailState.value?.post?.id || '')}` } : {}
  updateRoute(`/author/${source}/${encodeURIComponent(name)}${query}`, false, routeState)
  window.scrollTo({ top: 0, behavior: 'auto' })
}
function openAuthor(post) {
  if (phonePortrait.value && post && masonryDetailPost.value?.id === post.id) {
    openMobileDetailAuthor(post)
    return
  }
  if (phonePortrait.value && post && !masonryDetailPost.value && !authorProfile.value) {
    const returnPath = window.location.pathname + window.location.search
    mobileAuthorDetailState.value = {
      post,
      authorPath: '/author/' + post.source + '/' + encodeURIComponent(post.author),
      authorScrollY: 0,
      detailScrollY: window.scrollY,
      returnPath,
      returnTitle: activeSource.value === 'all' ? '全部动态' : (sourceMeta[activeSource.value]?.label || '动态'),
      returnPostIds: filteredPosts.value.map(item => item.id),
      returnToDetail: false
    }
    mobileAuthorScrollY = 0
    mobileAuthorPreviewPost.value = post
    captureMobileAuthorPreviewSnapshot(0)
    startMobileAuthorPreviewHandoff(post)
    openAuthorPage(post.author, post.source, post.avatar, '', true)
    restoreMobileScroll(0)
    return
  }
  openAuthorPage(post.author, post.source, post.avatar)
}
function openFeedAuthor(feed) {
  openAuthorPage(feed.name, feed.source, sourceAvatar(feed, platformCardForSource(feed.source)), isAccountCollectionFeed(feed) ? feed.id : '')
}
function closeSettingsPage() { navigateTo(activeSource.value === 'all' ? 'all' : 'source') }
function updateRoute(path, replace = false, state = {}) {
  if (`${window.location.pathname}${window.location.search}` === path) return
  const nextState = replace ? { ...(window.history.state || {}), ...state } : state
  window.history[replace ? 'replaceState' : 'pushState'](nextState, '', path)
}
function ensurePhoneExitBoundary() {
  if (!phonePortrait.value) return
  const currentPath = `${window.location.pathname}${window.location.search}`
  const atAllTimeline = currentPath === '/' && activeNav.value === 'all' && activeSource.value === 'all'
  if (atAllTimeline) {
    window.history.replaceState({ ...(window.history.state || {}), lumicExitBoundary: true }, '', '/')
    return
  }
  if (window.history.state?.lumicChildRoute) return
  window.history.replaceState({ lumicExitBoundary: true }, '', '/')
  window.history.pushState({ lumicChildRoute: true }, '', currentPath)
}
function handlePopState() {
  if (phoneOverlayHistoryClosing) {
    phoneOverlayHistoryClosing = false
    return
  }
  if (phoneOverlayHistoryActive) {
    const kind = phoneOverlayHistoryActive
    phoneOverlayHistoryActive = ''
    dismissPhoneOverlay(kind)
    return
  }
  if (lightboxHistoryPopPending) {
    lightboxHistoryPopPending = false
    return
  }
  if (lightboxHistoryActive) {
    closeLightbox(true)
    return
  }
  const returningFromAuthor = phonePortrait.value && Boolean((authorProfile.value || selectedTag.value) && mobileAuthorDetailState.value)
  const leavingDetail = phonePortrait.value && Boolean(masonryDetailPost.value)
  const departingDetailPost = masonryDetailPost.value
  const returningToTimeline = leavingDetail && !window.location.pathname.startsWith('/post/') && !window.location.pathname.startsWith('/author/')
  const gestureTimelineReturn = returningToTimeline && mobileDetailGestureReturnPending
  const detailRouteExit = returningToTimeline && !gestureTimelineReturn ? captureMobileDetailRouteExit(departingDetailPost) : null
  if (returningFromAuthor) mobileAuthorDetailState.value.authorScrollY = mobileAuthorScrollY
  if (returningFromAuthor && window.location.pathname.startsWith('/post/')) startMobileDetailReturnHandoff()
  else if (returningFromAuthor && mobileAuthorDetailState.value?.returnToDetail === false) startMobileTimelineReturnHandoff(mobileAuthorDetailState.value.post)
  if (returningToTimeline && (gestureTimelineReturn || !detailRouteExit)) startMobileTimelineReturnHandoff(departingDetailPost)
  applyRoute()
  mobileDetailGestureReturnPending = false
  resetMobileAuthorPageSwipe()
  if (returningFromAuthor && window.location.pathname.startsWith('/post/') && mobileAuthorDetailState.value) restoreMobileScroll(mobileAuthorDetailState.value.detailScrollY)
  else if (returningFromAuthor && mobileAuthorDetailState.value?.returnToDetail === false) restoreMobileScroll(mobileAuthorDetailState.value.detailScrollY)
  else if (returningToTimeline) {
    restoreMobileScroll(mobilePostReturnScrollY.value)
    if (detailRouteExit) runMobileDetailRouteExit(detailRouteExit)
  }
}
function applyRoute() {
  resetMobileDetailPageSwipe()
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
    activeNav.value = 'author'; activeSource.value = segments[1]; selectedAuthor.value = { source: segments[1], name: segments.slice(2).join('/'), avatar: '', feedId: new URLSearchParams(window.location.search).get('feed') || '' }
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
  if (platformKey === 'twitter') return '连接 twitterapi.io 后，可将“推特点赞”添加为同步来源。'
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
watch(feedListElement, (element, previous) => {
  if (previous) feedListResizeObserver?.unobserve(previous)
  if (!element) {
    masonryMetricsReady.value = false
    return
  }
  feedListResizeObserver?.observe(element)
  if (isMasonryView.value) masonryMetricsReady.value = false
  nextTick(() => scheduleMasonryMetrics())
}, { flush: 'post' })
watch(feeds, () => { subscriptionPage.value = Math.min(Math.max(1, subscriptionPage.value), subscriptionPageCount.value) })
watch(subscriptionPageCount, count => { subscriptionPage.value = Math.min(Math.max(1, subscriptionPage.value), count) })
watch(phoneOverlayKey, next => {
  if (!phonePortrait.value || phoneOverlayDismissInProgress) return
  if (next) {
    const state = { ...(window.history.state || {}), lumicOverlay: next }
    if (phoneOverlayHistoryActive) window.history.replaceState(state, '', window.location.href)
    else window.history.pushState(state, '', window.location.href)
    phoneOverlayHistoryActive = next
    return
  }
  if (!phoneOverlayHistoryActive) return
  phoneOverlayHistoryActive = ''
  phoneOverlayHistoryClosing = true
  window.history.back()
})
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
onMounted(() => { isDark.value = localStorage.getItem('lumic-theme') === 'dark'; timelineView.value = localStorage.getItem('lumic-timeline-view') === 'masonry' ? 'masonry' : 'list'; loadRememberedLogin(); document.querySelector('meta[name="theme-color"]')?.setAttribute('content', isDark.value ? '#080a0e' : '#fbf7ea'); phonePortraitQuery = window.matchMedia('(max-width: 760px)'); phonePortrait.value = isPhonePortraitScreen(); phonePortraitQuery.addEventListener('change', updatePhonePortrait); window.addEventListener('orientationchange', updatePhonePortrait); document.addEventListener('pointerover', useRoundedTooltip, true); postResizeObserver = new ResizeObserver(entries => { for (const entry of entries) { const post = postById.value.get(String(entry.target.dataset.postId)); const borderBox = Array.isArray(entry.borderBoxSize) ? entry.borderBoxSize[0] : entry.borderBoxSize; if (post) measurePostElement(post, entry.target, entry.target.dataset.layout || 'list', borderBox?.blockSize || entry.contentRect.height) }; scheduleTimelineWindow() }); initializeFeedListResizeObserver(); applyRoute(); if (phonePortrait.value && window.location.pathname === '/') openPhoneDefaultTimeline(); ensurePhoneExitBoundary(); checkSession(); sessionPollTimer = window.setInterval(() => checkSession(false), 60_000); window.addEventListener('keydown', handleGlobalKeydown); window.addEventListener('popstate', handlePopState); window.addEventListener('scroll', handleWindowScroll, { passive: true }); window.addEventListener('resize', handleWindowResize); scheduleTimelineWindow(); if (phonePortrait.value) showMobileControls() })
onUnmounted(() => { postLoadGeneration += 1; stopWeiboPolling(); stopBilibiliPolling(); stopNightMeteorLoop(); if (sessionPollTimer) window.clearInterval(sessionPollTimer); if (mobileControlsTimer) window.clearTimeout(mobileControlsTimer); if (mobileAuthorHandoffTimer) window.clearTimeout(mobileAuthorHandoffTimer); if (mobileDetailReturnHandoffTimer) window.clearTimeout(mobileDetailReturnHandoffTimer); if (mobileTimelineReturnHandoffTimer) window.clearTimeout(mobileTimelineReturnHandoffTimer); phonePortraitQuery?.removeEventListener('change', updatePhonePortrait); window.removeEventListener('orientationchange', updatePhonePortrait); document.removeEventListener('pointerover', useRoundedTooltip, true); postResizeObserver?.disconnect(); feedListResizeObserver?.disconnect(); observedPostElements.clear(); preloadedPreviewUrls.clear(); transientTimers.forEach(timer => window.clearTimeout(timer)); transientTimers.clear(); clearMobileDetailAnimation(); resetMobileDetailPageSwipe(); cleanupMobileDetailRouteExit(); lightboxHistoryActive = false; resetLightboxState(); closeContextMenu(); window.removeEventListener('keydown', handleGlobalKeydown); window.removeEventListener('popstate', handlePopState); window.removeEventListener('scroll', handleWindowScroll); window.removeEventListener('resize', handleWindowResize); if (timelineFrame) window.cancelAnimationFrame(timelineFrame); if (masonryMetricsFrame) window.cancelAnimationFrame(masonryMetricsFrame); if (confirmResolver) closeConfirmDialog(false) })
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
      <form class="login-form" @submit.prevent="login" autocomplete="on">
        <label for="username">账号</label>
        <input id="username" v-model="credentials.username" type="text" name="username" autocomplete="username" required autofocus>
        <label for="password">密码</label>
        <div class="login-password-field"><input id="password" v-model="credentials.password" :type="passwordVisible ? 'text' : 'password'" name="password" autocomplete="current-password" required><button type="button" :title="passwordVisible ? '隐藏密码' : '显示密码'" :aria-label="passwordVisible ? '隐藏密码' : '显示密码'" @click="passwordVisible = !passwordVisible"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2.8 12s3.2-5.2 9.2-5.2S21.2 12 21.2 12s-3.2 5.2-9.2 5.2S2.8 12 2.8 12Z"/><circle cx="12" cy="12" r="2.5"/><path v-if="!passwordVisible" d="m4 4 16 16"/></svg></button></div>
        <label class="remember-password"><input v-model="rememberPassword" type="checkbox"><span>记住密码</span></label>
        <p v-if="loginError" class="login-error" role="alert">{{ loginError }}</p>
        <button class="login-button" type="submit" :disabled="loginBusy">{{ loginBusy ? '验证中…' : '登录拾光' }}</button>
      </form>
    </div>
  </div>
  <div v-else class="app-shell" :class="{ dark: isDark, 'lightbox-active': lightbox.open, 'phone-ui': phonePortrait, 'timeline-search-focused': timelineSearchFocused, 'mobile-page-switching': mobilePageSwitching }" @click="showBrandMenu = false">
    <button v-if="phonePortrait && !lightbox.open && !masonryDetailPost" class="mobile-timeline-toggle mobile-frosted-control" type="button" :class="{ open: mobileSourcesOpen, active: mobileAtAllTimeline, 'icon-switching': mobileTimelineIconSwitching, 'mobile-control-hidden': !mobileControlsVisible && !mobileSourcesOpen }" :aria-expanded="mobileSourcesOpen" :title="mobileSourcesOpen ? `收起${mobileTimelineTitle}栏目` : `展开${mobileTimelineTitle}栏目`" :aria-label="mobileSourcesOpen ? `收起${mobileTimelineTitle}栏目` : `展开${mobileTimelineTitle}栏目`" @pointerdown.stop @click.stop="showMobileControls(); toggleMobileTimelineShortcut()">
      <span v-if="mobileTimelineMeta" class="mobile-platform-mask" :style="{ '--mobile-platform-mask': `url(${mobileTimelineMeta.lineImage})` }" aria-hidden="true"></span>
      <span v-else class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${timelineNavIcon})` }" aria-hidden="true"></span>
    </button>
    <nav v-if="phonePortrait && mobileSourcesOpen && !lightbox.open && !masonryDetailPost" class="mobile-source-shortcuts" aria-label="平台动态">
      <button type="button" :class="{ active: mobileAtAllTimeline }" title="查看全部动态" aria-label="查看全部动态" @click.stop="navigateToMobileAll"><span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${timelineNavIcon})` }" aria-hidden="true"></span></button>
      <button v-for="(meta, key) in sourceMeta" :key="`mobile-source-${key}`" type="button" :class="{ active: activeNav === 'source' && activeSource === key }" :title="`查看${meta.label}动态`" :aria-label="`查看${meta.label}动态`" @click.stop="navigateToMobileSource(key)"><img :class="['sidebar-source-icon', `sidebar-${key}-icon`]" :src="meta.lineImage" alt=""></button>
    </nav>
    <button v-if="phonePortrait && !lightbox.open && !masonryDetailPost" class="mobile-menu-toggle mobile-frosted-control" type="button" :class="{ open: mobileMenuOpen, 'mobile-control-hidden': !mobileControlsVisible && !mobileMenuOpen }" :aria-expanded="mobileMenuOpen" :title="mobileMenuOpen ? '关闭导航' : '打开导航'" :aria-label="mobileMenuOpen ? '关闭导航' : '打开导航'" @pointerdown.stop @click.stop="showMobileControls(); mobileSourcesOpen = false; mobileMenuOpen = !mobileMenuOpen">
      <svg viewBox="0 0 24 24" aria-hidden="true"><path class="menu-line menu-line-top" d="M5 7h14"/><path class="menu-line menu-line-middle" d="M5 12h14"/><path class="menu-line menu-line-bottom" d="M5 17h14"/></svg>
    </button>
    <button v-if="mobileMenuOpen || mobileSourcesOpen" class="mobile-menu-scrim" type="button" aria-label="关闭导航" @click="mobileMenuOpen = false; mobileSourcesOpen = false"></button>
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
            <button class="source-nav-toggle" type="button" data-tooltip-disabled="true" :aria-expanded="sourcesExpanded" @click="sourcesExpanded = !sourcesExpanded"><svg :class="{ collapsed: !sourcesExpanded }" viewBox="0 0 24 24" aria-hidden="true"><path d="m4 8.5 8 4.3 8-4.3"/></svg></button>
          </div>
          <Transition name="source-nav-collapse">
            <div v-show="sourcesExpanded" class="source-nav-children">
              <button v-for="(meta, key) in sourceMeta" :key="key" :class="{ active: activeNav === 'source' && activeSource === key }" @click="navigateTo('source', key)"><img :class="['sidebar-source-icon', `sidebar-${key}-icon`]" :src="meta.lineImage" :alt="`${meta.label}线条图标`">{{ meta.label }}</button>
            </div>
          </Transition>
        </div>
        <button class="mobile-order-favorite" :class="{ active: activeNav === 'liked' }" @click="navigateTo('liked', 'all')">
<span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${favoriteNavIcon})` }" aria-hidden="true"></span> 收藏
</button>
        <button class="mobile-order-pulls" :class="{ active: activeNav === 'pulls' }" @click="navigateTo('pulls')">
<span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${subscriptionsNavIcon})` }" aria-hidden="true"></span> 订阅平台
</button>
      </nav>
      <div class="sidebar-bottom">
        <button class="mobile-order-settings" :class="{ active: activeNav === 'settings' }" @click="openSettings()"><span class="nav-line-symbol nav-mask-symbol" :style="{ '--nav-mask': `url(${settingsNavIcon})` }" aria-hidden="true"></span> 设置</button>
        <button class="sidebar-theme-button" type="button" data-tooltip-disabled="true" @click="setDarkMode(!isDark); mobileMenuOpen = false" :aria-label="isDark ? '当前为夜间主题，点击切换日间主题' : '当前为日间主题，点击切换夜间主题'"><span class="theme-mask-symbol" :style="{ '--nav-mask': `url(${isDark ? nightThemeIcon : dayThemeIcon})` }" aria-hidden="true"></span></button>
        <button v-if="phonePortrait" class="mobile-logout-button" type="button" title="退出登录" aria-label="退出登录" @click="logout"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M10 5H6.5A2.5 2.5 0 0 0 4 7.5v9A2.5 2.5 0 0 0 6.5 19H10M14 8l4 4-4 4M18 12H9"/></svg></button>
      </div>
    </aside>

    <aside v-if="phonePortrait && mobileAuthorDetailState?.returnToDetail !== false && (authorProfile || mobileDetailReturnHandoff)" class="mobile-detail-return-preview" :style="mobileDetailReturnPreviewStyle" aria-hidden="true">
      <div class="mobile-detail-return-content" :style="mobileDetailReturnContentStyle">
      <header class="mobile-post-detail-head"><span class="mobile-detail-preview-back"><svg viewBox="0 0 24 24"><path d="m15 5-7 7 7 7"/></svg></span><div class="mobile-post-author"><img :src="postAvatar(mobileAuthorDetailState.post)" :alt="mobileAuthorDetailState.post.author"><strong>{{ mobileAuthorDetailState.post.author }}</strong></div><span :class="['source-pill', 'mobile-post-source', sourceMeta[mobileAuthorDetailState.post.source].color]"><img class="source-icon" :src="sourceIconFor(mobileAuthorDetailState.post.source)" alt="">{{ sourceMeta[mobileAuthorDetailState.post.source].label }}</span></header>
      <section v-if="mobileDetailPreviewMedia(mobileAuthorDetailState.post)" :class="['mobile-detail-preview-media', { 'video-media': mobileDetailPreviewMedia(mobileAuthorDetailState.post).type === 'video' }]" :style="mobileDetailPreviewMedia(mobileAuthorDetailState.post).type === 'video' ? postVideoFrameStyle(mobileAuthorDetailState.post) : undefined"><img v-if="mobileDetailPreviewCover(mobileAuthorDetailState.post)" :src="previewMedia(mobileDetailPreviewCover(mobileAuthorDetailState.post))" :alt="mobileAuthorDetailState.post.author" loading="eager" decoding="async" fetchpriority="low"><span v-else class="mobile-detail-preview-video-mark"><svg viewBox="0 0 24 24"><path d="m9 7 8 5-8 5Z"/></svg></span></section>
      <section class="mobile-post-copy"><p v-if="mobileAuthorDetailState.post.caption" class="caption">{{ mobileAuthorDetailState.post.caption }}</p><div v-if="mobileAuthorDetailState.post.tags?.length" class="tag-row"><span v-for="tag in mobileAuthorDetailState.post.tags" :key="tag">#{{ tag }}</span></div></section>
      <footer class="mobile-post-detail-foot"><time :datetime="mobileAuthorDetailState.post.published">{{ postDateTime(mobileAuthorDetailState.post.published) }}</time><div class="mobile-detail-preview-actions"><i></i><i></i></div></footer>
      </div>
    </aside>
    <main v-if="!showSettings && activeNav !== 'pulls' && !(phonePortrait && masonryDetailPost)" :class="['content', { 'liked-page': activeNav === 'liked', 'mobile-author-detail-page': mobilePagedDetailActive }]" :style="mobilePagedDetailActive ? mobileAuthorPageStyle : undefined" @touchstart.passive="beginMobileAuthorPageSwipe" @touchmove="updateMobileAuthorPageSwipe" @touchend.passive="finishMobileAuthorPageSwipe" @touchcancel="finishMobileAuthorPageSwipe" @click="closeContextMenu" @contextmenu.prevent="openContextMenu($event)">
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
          <img :key="`${authorProfile.source}:${authorProfile.name}:${postAvatar(authorProfile)}`" :src="postAvatar(authorProfile)" data-fallback-index="0" :alt="authorProfile.name" referrerpolicy="no-referrer" @load="handlePostAvatarLoad($event, authorProfile)" @error="handlePostAvatarError($event, authorProfile)">
          <div><p class="eyebrow">AUTHOR TIMELINE · {{ sourceMeta[authorProfile.source].label }}</p><h1>{{ authorProfile.name }}</h1><p class="subtitle">共 {{ authorProfile.count }} 条已拉取动态</p></div>
        </div>
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
<button v-if="!authorProfile" class="timeline-all-button" :class="{ selected: activeSource === 'all' }" @click="activeSource = 'all'"><span>✦</span>全部</button>
<button class="timeline-sort-button" type="button" :title="timelineSort === 'newest' ? '最新' : '最旧'" :aria-label="timelineSort === 'newest' ? '当前按最新排序，点击切换为最旧' : '当前按最旧排序，点击切换为最新'" @click="timelineSort = timelineSort === 'newest' ? 'oldest' : 'newest'">
  <span class="timeline-sort-symbol" :style="{ '--nav-mask': `url(${timelineSort === 'newest' ? newestSortIcon : oldestSortIcon})` }" aria-hidden="true"></span>
</button>
<button class="timeline-view-button timeline-toolbar-button" type="button" :data-tooltip="isMasonryView ? '瀑布流' : '列表'" :aria-label="isMasonryView ? '当前为瀑布流视图，点击切换为列表' : '当前为列表视图，点击切换为瀑布流'" @click="toggleTimelineView">
  <span :class="['timeline-view-symbol', { 'list-view-symbol': !isMasonryView }]" :style="{ '--nav-mask': `url(${isMasonryView ? masonryViewIcon : listViewIcon})` }" aria-hidden="true"></span>
</button>
<button class="timeline-refresh-button timeline-toolbar-button" type="button" :disabled="syncing" data-tooltip="刷新动态" aria-label="刷新动态" @click="refreshTimeline">
  <span class="timeline-refresh-symbol" :style="{ '--nav-mask': `url(${refreshIcon})` }" aria-hidden="true"></span>
</button>
</div>
<div class="timeline-tools">
  <label class="timeline-search" @pointerdown.stop="closeMobileNavigationOnInputFocus" @click.stop><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m16 16 5 5"/></svg><input v-model="timelineSearch" type="search" :placeholder="phonePortrait ? '搜索' : '搜索内容、作者或标签'" aria-label="搜索动态内容、作者或标签" @focus="closeMobileNavigationOnInputFocus" @input="closeMobileNavigationOnInputFocus" @blur="releaseTimelineSearchFocus"></label>
</div>
</div>
      <p v-if="timelineMessage" class="timeline-message">{{ timelineMessage }}</p>
      <section ref="feedListElement" :class="['feed-list', { 'masonry-feed': isMasonryView }]" :style="masonryFeedStyle">
<template v-if="!isMasonryView">
<div v-if="timelineTopSpace" class="timeline-spacer" :style="{ height: `${timelineTopSpace}px` }" aria-hidden="true"></div>
<article v-for="post in visiblePosts" :key="post.id" :ref="element => setPostCard(post, element, 'list')" :class="['post-card', { selected: selectedPostIds.includes(post.id), selectable: selectionMode }]" :data-post-id="post.id" @click.capture="handlePostSelectionClick($event, post)" @contextmenu.stop.prevent="openContextMenu($event, post)">
<label v-if="selectionMode" class="post-select-control" :title="`选择 ${post.author} 的这条动态`" @click.prevent><input type="checkbox" :checked="selectedPostIds.includes(post.id)" tabindex="-1"><span></span></label>
<div class="post-head">
<button class="post-author-avatar" type="button" :title="`查看 ${post.author} 的动态`" @click="openAuthor(post)"><img :key="`${post.id}:${postAvatar(post)}`" :src="postAvatar(post)" data-fallback-index="0" :alt="post.author" referrerpolicy="no-referrer" @load="handlePostAvatarLoad($event, post)" @error="handlePostAvatarError($event, post)"></button>
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
<article v-for="item in visibleMasonryItems" :key="item.post.id" :ref="element => setPostCard(item.post, element, 'masonry')" :class="['masonry-card', { selected: selectedPostIds.includes(item.post.id), selectable: selectionMode, 'text-only': !masonryCover(item.post) && !item.post.videos?.length }]" :style="masonryItemStyle(item)" :data-post-id="item.post.id" tabindex="0" @click.capture="handlePostSelectionClick($event, item.post)" @click="openMasonryPost(item.post, $event)" @keydown.enter.prevent="openMasonryPost(item.post, $event)" @contextmenu.stop.prevent="openContextMenu($event, item.post)">
  <label v-if="selectionMode" class="post-select-control masonry-select-control" :title="`选择 ${item.post.author} 的这条动态`" @click.prevent><input type="checkbox" :checked="selectedPostIds.includes(item.post.id)" tabindex="-1"><span></span></label>
  <div v-if="masonryCover(item.post)" class="masonry-cover">
    <img :src="previewMedia(masonryCover(item.post))" alt="" loading="lazy" decoding="async" fetchpriority="low" @load="setMasonryCoverRatio(item.post, $event, masonryCoverIsVideo(item.post))">
    <span v-if="!masonryCoverIsVideo(item.post) && item.post.media?.length > 1" class="masonry-media-count">1 / {{ item.post.media.length }}</span>
    <span v-if="masonryPostHasVideo(item.post)" class="masonry-video-mark" aria-label="视频动态"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9 7 8 5-8 5Z"/></svg></span>
  </div>
  <div v-else-if="primaryVideo(item.post)" class="masonry-cover masonry-video-cover">
    <img v-if="primaryVideo(item.post).poster" :src="previewMedia(primaryVideo(item.post).poster)" alt="视频封面" loading="lazy" decoding="async" fetchpriority="low" @load="setMasonryCoverRatio(item.post, $event, true)">
    <video v-else :src="videoPreviewSource(primaryVideo(item.post))" muted playsinline preload="metadata" aria-label="视频封面" @loadedmetadata="setMasonryCoverRatio(item.post, $event, true)"></video>
    <span class="masonry-video-mark" aria-label="视频动态"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9 7 8 5-8 5Z"/></svg></span>
  </div>
  <div class="masonry-card-body">
    <p v-if="item.post.caption" class="masonry-caption">{{ item.post.caption }}</p>
    <div v-if="item.post.tags?.length" class="masonry-tags"><button v-for="tag in item.post.tags.slice(0, 2)" :key="tag" type="button" @click.stop="openTag(tag)">#{{ tag }}</button><span v-if="item.post.tags.length > 2">+{{ item.post.tags.length - 2 }}</span></div>
    <footer class="masonry-meta">
      <button class="masonry-author" type="button" :title="`查看 ${item.post.author} 的动态`" @click.stop="openAuthor(item.post)"><img :key="`${item.post.id}:${postAvatar(item.post)}`" :src="postAvatar(item.post)" data-fallback-index="0" :alt="item.post.author" referrerpolicy="no-referrer" @load="handlePostAvatarLoad($event, item.post)" @error="handlePostAvatarError($event, item.post)"><span><strong>{{ item.post.author }}</strong><small>{{ postDateTime(item.post.published) }}</small></span></button>
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
      <div class="section-heading subscription-list-heading"><div><h2>订阅列表</h2><p>查看同步状态并手动拉取内容。</p></div><span>{{ feeds.length }} 个来源</span></div>
      <section class="pull-list">
        <article v-for="feed in pagedFeeds" :key="feed.id" class="pull-card">
          <button class="pull-avatar-button" type="button" :title="`查看 ${feed.name} 的动态`" :aria-label="`查看 ${feed.name} 的动态`" @click="openFeedAuthor(feed)"><img :key="`${feed.id}:${sourceAvatar(feed, platformCardForSource(feed.source))}`" class="pull-avatar" :src="sourceAvatar(feed, platformCardForSource(feed.source))" data-fallback-index="0" :alt="feed.name" referrerpolicy="no-referrer" @error="handleSourceAvatarError($event, feed, platformCardForSource(feed.source))"></button>
          <div class="pull-info"><div class="pull-title"><strong>{{ feed.name }}</strong><span :class="['source-pill', sourceMeta[feed.source].color]"><img :class="['source-icon', { 'twitter-night-icon': feed.source === 'twitter' && isDark }]" :src="sourceIconFor(feed.source)" :alt="sourceMeta[feed.source].label">{{ sourceMeta[feed.source].label }}</span></div><span class="pull-handle">{{ feed.handle }}</span><small>{{ feedLastSyncText(feed) }}</small></div>
          <div class="pull-status"><i :class="['pull-dot', feed.lastSyncStatus]"></i><span>{{ feed.lastSyncStatus === 'success' ? `新增 ${feed.lastSyncCount || 0} 条` : feed.lastSyncStatus === 'failed' ? '拉取失败' : '待拉取' }}</span></div>
          <div class="pull-actions"><button class="pull-action" :disabled="sourceActionBusy !== ''" @click="syncSource(feed)"><svg :class="{ spin: sourceActionBusy === `sync:${feed.id}` }" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 9a7 7 0 0 1 11.4-2.5L20 9M4 15l2.5 2.5A7 7 0 0 0 17.9 15"/></svg><span>{{ sourceActionBusy === `sync:${feed.id}` ? '同步中' : '立即同步' }}</span></button></div>
        </article>
        <div v-if="!feeds.length" class="empty">还没有订阅内容，请先添加 UP 主、博主、画师或账号收藏来源。</div>
      </section>
      <nav v-if="subscriptionPageCount > 1" class="subscription-pagination" aria-label="订阅列表分页">
        <button type="button" :disabled="subscriptionPage <= 1" title="上一页" aria-label="上一页" @click="subscriptionPage--"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m14.5 5-7 7 7 7"/></svg></button>
        <span>{{ subscriptionPage }} / {{ subscriptionPageCount }}</span>
        <button type="button" :disabled="subscriptionPage >= subscriptionPageCount" title="下一页" aria-label="下一页" @click="subscriptionPage++"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9.5 5 7 7-7 7"/></svg></button>
      </nav>
    </main>
    <aside v-if="phonePortrait && mobileAuthorPreviewDisplayPost" class="mobile-author-swipe-preview" :style="mobileAuthorPreviewStyle" aria-hidden="true">
      <div class="mobile-transition-page-content" :style="mobileAuthorPreviewContentStyle">
      <header class="topbar author-page-header mobile-author-preview-head"><div class="author-profile-main"><img :key="'preview:' + authorAvatarKey(mobileAuthorPreviewDisplayPost) + ':' + postAvatar(mobileAuthorPreviewDisplayPost)" :src="postAvatar(mobileAuthorPreviewDisplayPost)" :alt="mobileAuthorPreviewDisplayPost.author" @load="handlePostAvatarLoad($event, mobileAuthorPreviewDisplayPost)" @error="handlePostAvatarError($event, mobileAuthorPreviewDisplayPost)"><div><p class="eyebrow">AUTHOR TIMELINE · {{ sourceMeta[mobileAuthorPreviewDisplayPost.source].label }}</p><h1>{{ mobileAuthorPreviewDisplayPost.author }}</h1><p class="subtitle">共 {{ mobileAuthorTimelinePosts.length }} 条已拉取动态</p></div></div></header>
      <div class="section-heading mobile-author-preview-heading"><div class="filters"><button class="timeline-sort-button" type="button" tabindex="-1"><span class="timeline-sort-symbol" :style="{ '--nav-mask': `url(${timelineSort === 'newest' ? newestSortIcon : oldestSortIcon})` }"></span></button><button class="timeline-view-button timeline-toolbar-button" type="button" tabindex="-1"><span :class="['timeline-view-symbol', { 'list-view-symbol': !isMasonryView }]" :style="{ '--nav-mask': `url(${isMasonryView ? masonryViewIcon : listViewIcon})` }"></span></button><button class="timeline-refresh-button timeline-toolbar-button" type="button" tabindex="-1"><span class="timeline-refresh-symbol" :style="{ '--nav-mask': `url(${refreshIcon})` }"></span></button></div><div class="timeline-tools"><label class="timeline-search"><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="m16 16 5 5"/></svg><input type="search" value="" placeholder="搜索" tabindex="-1" readonly></label></div></div>
      <section class="mobile-author-preview-feed feed-list masonry-feed" :style="mobileAuthorPreviewFeedStyle">
        <article v-for="item in mobileAuthorPreviewItems" :key="item.post.id" :class="['masonry-card', { 'text-only': !masonryCover(item.post) && !item.post.videos?.length }]" :style="mobilePreviewMasonryItemStyle(item)">
          <div v-if="masonryCover(item.post)" class="masonry-cover">
            <img :src="previewMedia(masonryCover(item.post))" alt="" loading="eager" decoding="async" fetchpriority="low">
            <span v-if="!masonryCoverIsVideo(item.post) && item.post.media?.length > 1" class="masonry-media-count">1 / {{ item.post.media.length }}</span>
            <span v-if="masonryPostHasVideo(item.post)" class="masonry-video-mark" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m9 7 8 5-8 5Z"/></svg></span>
          </div>
          <div v-else-if="primaryVideo(item.post)" class="masonry-cover masonry-video-cover">
            <img v-if="primaryVideo(item.post).poster" :src="previewMedia(primaryVideo(item.post).poster)" alt="" loading="eager" decoding="async" fetchpriority="low">
            <video v-else :src="videoPreviewSource(primaryVideo(item.post))" muted playsinline preload="metadata" aria-hidden="true"></video>
            <span class="masonry-video-mark" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m9 7 8 5-8 5Z"/></svg></span>
          </div>
          <div class="masonry-card-body">
            <p v-if="item.post.caption" class="masonry-caption">{{ item.post.caption }}</p>
            <div v-if="item.post.tags?.length" class="masonry-tags"><button v-for="tag in item.post.tags.slice(0, 2)" :key="tag" type="button" tabindex="-1">#{{ tag }}</button><span v-if="item.post.tags.length > 2">+{{ item.post.tags.length - 2 }}</span></div>
            <footer class="masonry-meta"><span class="masonry-author"><img :src="postAvatar(item.post)" alt=""><span><strong>{{ item.post.author }}</strong><small>{{ postDateTime(item.post.published) }}</small></span></span><span :class="['masonry-like-button', { liked: item.post.liked }]"><span class="post-action-mask post-favorite-symbol" :style="{ '--post-action-mask': `url(${favoriteNavIcon})` }"></span></span></footer>
          </div>
        </article>
      </section>
      </div>
    </aside>
    <aside v-if="phonePortrait && mobileReturnPreviewDisplayPost && (masonryDetailPost || mobileTimelineReturnHandoff || (mobileAuthorReturnsToTimeline && (mobileAuthorPageDragging || mobileAuthorPageAnimating)))" class="mobile-return-swipe-preview" :style="mobileReturnPreviewStyle" aria-hidden="true">
      <header><div><small>SAVED MOMENTS</small><strong>{{ mobileReturnPreviewTitle }}</strong></div><span :class="['source-pill', sourceMeta[mobileReturnPreviewDisplayPost.source].color]"><img class="source-icon" :src="sourceIconFor(mobileReturnPreviewDisplayPost.source)" alt="">{{ sourceMeta[mobileReturnPreviewDisplayPost.source].label }}</span></header>
      <div class="mobile-author-preview-toolbar"><span>全部</span><i></i><i></i><label><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m16 16 5 5"/></svg><b>搜索</b></label></div>
      <section class="mobile-author-preview-feed feed-list masonry-feed" :style="mobileReturnPreviewFeedStyle">
        <article v-for="item in mobileReturnPreviewItems" :key="item.post.id" :class="['masonry-card', { 'text-only': !masonryCover(item.post) && !item.post.videos?.length }]" :style="mobilePreviewMasonryItemStyle(item)">
          <div v-if="masonryCover(item.post)" class="masonry-cover">
            <img :src="previewMedia(masonryCover(item.post))" alt="" loading="eager" decoding="async" fetchpriority="low">
            <span v-if="!masonryCoverIsVideo(item.post) && item.post.media?.length > 1" class="masonry-media-count">1 / {{ item.post.media.length }}</span>
            <span v-if="masonryPostHasVideo(item.post)" class="masonry-video-mark" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m9 7 8 5-8 5Z"/></svg></span>
          </div>
          <div v-else-if="primaryVideo(item.post)" class="masonry-cover masonry-video-cover">
            <img v-if="primaryVideo(item.post).poster" :src="previewMedia(primaryVideo(item.post).poster)" alt="" loading="eager" decoding="async" fetchpriority="low">
            <video v-else :src="videoPreviewSource(primaryVideo(item.post))" muted playsinline preload="metadata" aria-hidden="true"></video>
            <span class="masonry-video-mark" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m9 7 8 5-8 5Z"/></svg></span>
          </div>
          <div class="masonry-card-body">
            <p v-if="item.post.caption" class="masonry-caption">{{ item.post.caption }}</p>
            <div v-if="item.post.tags?.length" class="masonry-tags"><span v-for="tag in item.post.tags.slice(0, 2)" :key="tag">#{{ tag }}</span><span v-if="item.post.tags.length > 2">+{{ item.post.tags.length - 2 }}</span></div>
            <footer class="masonry-meta"><span class="masonry-author"><img :src="postAvatar(item.post)" alt=""><span><strong>{{ item.post.author }}</strong><small>{{ postDateTime(item.post.published) }}</small></span></span><span :class="['masonry-like-button', { liked: item.post.liked }]"><span class="post-action-mask post-favorite-symbol" :style="{ '--post-action-mask': `url(${favoriteNavIcon})` }"></span></span></footer>
          </div>
        </article>
      </section>
    </aside>
    <aside v-if="phonePortrait && masonryDetailPost && mobileDetailPreviousPost" class="mobile-detail-vertical-preview previous" :style="mobilePreviousDetailPreviewStyle" aria-hidden="true">
      <header class="mobile-post-detail-head">
        <span class="mobile-detail-preview-back"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m15 5-7 7 7 7"/></svg></span>
        <div class="mobile-post-author"><img :src="postAvatar(mobileDetailPreviousPost)" alt="" referrerpolicy="no-referrer"><strong>{{ mobileDetailPreviousPost.author }}</strong></div>
        <span :class="['source-pill', 'mobile-post-source', sourceMeta[mobileDetailPreviousPost.source].color]"><img class="source-icon" :src="sourceIconFor(mobileDetailPreviousPost.source)" alt="">{{ sourceMeta[mobileDetailPreviousPost.source].label }}</span>
      </header>
      <section v-if="mobileDetailPreviewMedia(mobileDetailPreviousPost)" :class="['mobile-detail-preview-media', { 'video-media': mobileDetailPreviewMedia(mobileDetailPreviousPost).type === 'video' }]" :style="mobileDetailPreviewMedia(mobileDetailPreviousPost).type === 'video' ? postVideoFrameStyle(mobileDetailPreviousPost) : undefined">
        <img v-if="mobileDetailPreviewMedia(mobileDetailPreviousPost).type === 'image'" :src="previewMedia(mobileDetailPreviewMedia(mobileDetailPreviousPost).src)" alt="" loading="eager" decoding="async" fetchpriority="low">
        <img v-else-if="mobileDetailPreviewMedia(mobileDetailPreviousPost).poster" :src="previewMedia(mobileDetailPreviewMedia(mobileDetailPreviousPost).poster)" alt="" loading="eager" decoding="async" fetchpriority="low">
        <span v-else class="mobile-detail-preview-video-mark" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m9 7 8 5-8 5Z"/></svg></span>
      </section>
      <section class="mobile-post-copy"><p v-if="mobileDetailPreviousPost.caption" class="caption">{{ mobileDetailPreviousPost.caption }}</p><div v-if="mobileDetailPreviousPost.tags?.length" class="tag-row"><span v-for="tag in mobileDetailPreviousPost.tags" :key="tag">#{{ tag }}</span></div></section>
      <footer class="mobile-post-detail-foot"><time :datetime="mobileDetailPreviousPost.published">{{ postDateTime(mobileDetailPreviousPost.published) }}</time><div class="mobile-detail-preview-actions"><i></i><i></i></div></footer>
    </aside>
    <aside v-if="phonePortrait && masonryDetailPost && mobileDetailNextPost" class="mobile-detail-vertical-preview next" :style="mobileNextDetailPreviewStyle" aria-hidden="true">
      <header class="mobile-post-detail-head">
        <span class="mobile-detail-preview-back"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m15 5-7 7 7 7"/></svg></span>
        <div class="mobile-post-author"><img :src="postAvatar(mobileDetailNextPost)" alt="" referrerpolicy="no-referrer"><strong>{{ mobileDetailNextPost.author }}</strong></div>
        <span :class="['source-pill', 'mobile-post-source', sourceMeta[mobileDetailNextPost.source].color]"><img class="source-icon" :src="sourceIconFor(mobileDetailNextPost.source)" alt="">{{ sourceMeta[mobileDetailNextPost.source].label }}</span>
      </header>
      <section v-if="mobileDetailPreviewMedia(mobileDetailNextPost)" :class="['mobile-detail-preview-media', { 'video-media': mobileDetailPreviewMedia(mobileDetailNextPost).type === 'video' }]" :style="mobileDetailPreviewMedia(mobileDetailNextPost).type === 'video' ? postVideoFrameStyle(mobileDetailNextPost) : undefined">
        <img v-if="mobileDetailPreviewMedia(mobileDetailNextPost).type === 'image'" :src="previewMedia(mobileDetailPreviewMedia(mobileDetailNextPost).src)" alt="" loading="eager" decoding="async" fetchpriority="low">
        <img v-else-if="mobileDetailPreviewMedia(mobileDetailNextPost).poster" :src="previewMedia(mobileDetailPreviewMedia(mobileDetailNextPost).poster)" alt="" loading="eager" decoding="async" fetchpriority="low">
        <span v-else class="mobile-detail-preview-video-mark" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m9 7 8 5-8 5Z"/></svg></span>
      </section>
      <section class="mobile-post-copy"><p v-if="mobileDetailNextPost.caption" class="caption">{{ mobileDetailNextPost.caption }}</p><div v-if="mobileDetailNextPost.tags?.length" class="tag-row"><span v-for="tag in mobileDetailNextPost.tags" :key="tag">#{{ tag }}</span></div></section>
      <footer class="mobile-post-detail-foot"><time :datetime="mobileDetailNextPost.published">{{ postDateTime(mobileDetailNextPost.published) }}</time><div class="mobile-detail-preview-actions"><i></i><i></i></div></footer>
    </aside>
    <main v-if="phonePortrait && masonryDetailPost" :class="['content', 'mobile-post-detail-page', { 'page-dragging': mobileDetailPageDragging }]" :style="mobileDetailPageStyle" @touchstart="beginMobileDetailPageSwipe" @touchmove="updateMobileDetailPageSwipe" @touchend="finishMobileDetailPageSwipe" @touchcancel="finishMobileDetailPageSwipe">
      <header class="mobile-post-detail-head">
        <button class="mobile-post-back" type="button" title="返回动态页" aria-label="返回动态页" @click="closePostDetail"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m15 5-7 7 7 7"/></svg></button>
        <button class="mobile-post-author" type="button" @click="openAuthor(masonryDetailPost)"><img :src="postAvatar(masonryDetailPost)" data-fallback-index="0" :alt="masonryDetailPost.author" referrerpolicy="no-referrer" @error="handlePostAvatarError($event, masonryDetailPost)"><strong>{{ masonryDetailPost.author }}</strong></button>
        <span :class="['source-pill', 'mobile-post-source', sourceMeta[masonryDetailPost.source].color]"><img :class="['source-icon', { 'twitter-night-icon': masonryDetailPost.source === 'twitter' && isDark }]" :src="sourceIconFor(masonryDetailPost.source)" :alt="`${sourceMeta[masonryDetailPost.source].label}图标`">{{ sourceMeta[masonryDetailPost.source].label }}</span>
      </header>
      <section v-if="mobileDetailCurrentMedia" :class="['mobile-post-media-stage', { 'video-media': mobileDetailCurrentMedia.type === 'video' }, mobileDetailCurrentMedia.type === 'video' ? postVideoFrameClass(masonryDetailPost) : '']" :style="mobileDetailCurrentMedia.type === 'video' ? postVideoFrameStyle(masonryDetailPost) : undefined" @pointerdown.stop="beginMobileDetailSwipe" @pointermove.stop="updateMobileDetailSwipe" @pointerup.stop="finishMobileDetailSwipe" @pointercancel.stop="cancelMobileDetailSwipe">
        <div class="mobile-post-media-track" :style="mobileDetailTrackStyle">
          <div v-for="slide in mobileDetailSlides" :key="`${slide.position}:${slide.media.key}`" :class="['mobile-post-media-slide', { current: slide.position === 0 }]">
            <img v-if="slide.media.type === 'image'" :src="previewMedia(slide.media.src)" :alt="`${masonryDetailPost.author} 的第 ${slide.index + 1} 张图片`" :loading="slide.position === 0 ? 'eager' : 'lazy'" decoding="async" :fetchpriority="slide.position === 0 ? 'high' : 'low'" @click="slide.position === 0 && openMobileDetailImage()">
            <video v-else :src="slide.media.src" :poster="slide.media.poster ? previewMedia(slide.media.poster) : undefined" :controls="slide.position === 0" playsinline :autoplay="slide.position === 0" muted :preload="slide.position === 0 ? 'metadata' : 'none'" @loadedmetadata="slide.position === 0 && setPostVideoRatio(masonryDetailPost, $event)"></video>
          </div>
        </div>
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
          <button class="post-author-avatar" type="button" :title="`查看 ${masonryDetailPost.author} 的动态`" @click="openAuthor(masonryDetailPost)"><img :src="postAvatar(masonryDetailPost)" data-fallback-index="0" :alt="masonryDetailPost.author" referrerpolicy="no-referrer" @error="handlePostAvatarError($event, masonryDetailPost)"></button>
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
      <button v-else-if="contextMenu.post" type="button" class="danger" role="menuitem" @click="deleteContextPost"><span class="action-icon-mask" :style="{ '--action-icon-mask': `url(${deleteIcon})` }" aria-hidden="true"></span><span>删除这条动态</span></button>
    </div>
    <div v-if="selectionMode" class="selection-dock">
      <button type="button" class="selection-cancel-button" title="取消多选" aria-label="取消多选" @click="stopSelection"><svg viewBox="0 0 24 24"><path d="m6 6 12 12M18 6 6 18"/></svg></button>
      <button type="button" class="selection-select-all-button" :disabled="!loadedSelectionPosts.length" :title="allLoadedPostsSelected ? '取消选择当前已加载动态' : '全选当前已加载动态'" :aria-label="allLoadedPostsSelected ? '取消选择当前已加载动态' : '全选当前已加载动态'" @click="toggleSelectAllLoadedPosts"><span class="action-icon-mask" :style="{ '--action-icon-mask': `url(${selectAllIcon})` }" aria-hidden="true"></span><b>{{ allLoadedPostsSelected ? '取消全选' : '全选' }}</b></button>
      <span>已选择 {{ selectedPostCount }} 条</span>
      <button v-if="selectionAction === 'unfavorite'" type="button" class="selection-delete-button selection-unfavorite-button" :disabled="!selectedPostCount || postActionBusy === 'batch-unfavorite'" title="取消收藏所选动态" aria-label="取消收藏所选动态" @click="unfavoriteSelectedPosts"><svg viewBox="0 0 24 24"><path d="M12 20.5S4.5 16.2 4.5 10.2A4.2 4.2 0 0 1 12 7.6a4.2 4.2 0 0 1 7.5 2.6c0 6-7.5 10.3-7.5 10.3Z"/><path d="M8.5 11.5h7"/></svg><b>取消收藏</b></button>
      <button v-else type="button" class="selection-delete-button" :disabled="!selectedPostCount || postActionBusy === 'batch-delete'" title="删除所选动态" aria-label="删除所选动态" @click="deleteSelectedPosts"><span class="action-icon-mask" :style="{ '--action-icon-mask': `url(${deleteIcon})` }" aria-hidden="true"></span><b>删除</b></button>
    </div>
    <div v-if="lightbox.open" :class="['lightbox-layer', { closing: lightboxClosing, 'mobile-entering': mobileLightboxEntering, 'mobile-exit-dragging': mobileLightboxExitDragging, 'mobile-gesture-exiting': mobileLightboxExitAnimating }]" :style="phonePortrait ? mobileLightboxLayerStyle : undefined" role="dialog" aria-modal="true" :aria-label="`${lightbox.author} 的动态媒体`" @click.self="closeLightbox" @pointermove="updateDesktopLightboxHover" @pointerleave="clearDesktopLightboxHover" @wheel.prevent="zoomLightbox">
      <div v-if="!phonePortrait" class="lightbox-close-hotspot" @pointerenter="showDesktopLightboxClose" @pointerleave="hideDesktopLightboxClose">
        <button :class="['lightbox-close', { 'pointer-visible': desktopLightboxHoverTarget === 'close' }]" type="button" aria-label="关闭大图" @click="closeLightbox"><span class="lightbox-tool-mask lightbox-close-symbol" :style="{ '--lightbox-tool-mask': `url(${closeLightboxIcon})` }" aria-hidden="true"></span></button>
      </div>
      <button v-if="!phonePortrait" :class="['lightbox-edge-nav', 'previous', { 'edge-visible': desktopLightboxHoverTarget === 'previous', disabled: lightbox.media.length < 2 }]" type="button" :aria-disabled="lightbox.media.length < 2" aria-label="上一张" @click.stop="moveLightbox(-1)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m14 5-7 7 7 7"/></svg></button>
      <button v-if="!phonePortrait" :class="['lightbox-edge-nav', 'next', { 'edge-visible': desktopLightboxHoverTarget === 'next', disabled: lightbox.media.length < 2 }]" type="button" :aria-disabled="lightbox.media.length < 2" aria-label="下一张" @click.stop="moveLightbox(1)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m10 5 7 7-7 7"/></svg></button>
      <div :class="['lightbox-image-stage', `motion-${lightbox.motion}`, { 'mobile-original-size': phonePortrait && lightboxAtOriginalSize }]" @click.self="closeLightbox" @contextmenu.prevent>
        <figure v-if="phonePortrait" class="mobile-lightbox-carousel" @click.stop @pointerdown.prevent="startLightboxGesture" @pointermove.prevent="moveLightboxGesture" @pointerup.prevent="stopLightboxGesture" @pointercancel.prevent="stopLightboxGesture">
          <div class="mobile-lightbox-track" :style="mobileLightboxTrackStyle">
            <div v-for="slide in mobileLightboxSlides" :key="`${slide.position}:${slide.media}`" :class="['mobile-lightbox-slide', { current: slide.position === 0 }]">
              <template v-if="slide.position === 0">
                <div :class="['mobile-lightbox-media-frame', { dragging: lightbox.dragging, 'zoom-animating': lightboxZoomAnimating, 'original-size': !lightbox.fit }]" :style="{ transform: `translate3d(${lightbox.x}px, ${lightbox.y}px, 0) rotate(${lightbox.rotation}deg) scale(${lightbox.scale})` }">
                  <img class="mobile-lightbox-preview" :src="previewMedia(slide.media)" :alt="`${lightbox.author} 的动态图片 ${slide.index + 1}`" decoding="async" draggable="false">
                  <img ref="lightboxImageElement" class="mobile-lightbox-original" :src="slide.media" :alt="`${lightbox.author} 的动态图片 ${slide.index + 1}`" :class="{ 'original-loaded': lightboxOriginalLoaded }" decoding="async" fetchpriority="high" draggable="false" @load="handleLightboxImageLoad">
                </div>
              </template>
              <img v-else :src="slide.source" alt="" draggable="false">
            </div>
          </div>
          <div :class="['mobile-lightbox-dots', { visible: mobileLightboxDotsVisible }]" aria-hidden="true"><i v-for="(_, index) in lightbox.media" :key="index" :class="{ active: index === lightbox.index }"></i></div>
        </figure>
        <figure v-else :key="`${lightbox.media[lightbox.index]}:${lightbox.motion}`" @click.self="closeLightbox">
          <img ref="lightboxImageElement" :src="lightboxDisplaySource" :alt="`${lightbox.author} 的动态图片 ${lightbox.index + 1}`" :class="{ dragging: lightbox.dragging, 'original-size': !lightbox.fit, 'original-loaded': lightboxOriginalLoaded }" :style="{ transform: `translate3d(${lightbox.x}px, ${lightbox.y}px, 0) rotate(${lightbox.rotation}deg) scale(${lightbox.scale})` }" draggable="false" @click.stop @pointerdown.prevent.stop="startLightboxGesture" @pointermove.prevent.stop="moveLightboxGesture" @pointerup.prevent.stop="stopLightboxGesture" @pointercancel.prevent.stop="stopLightboxGesture" @load="handleLightboxImageLoad">
        </figure>
      </div>
      <div v-if="!phonePortrait" class="lightbox-dock-zone" @pointerenter="showLightboxDock(false)" @pointermove="showLightboxDock(false)" @pointerleave="scheduleLightboxDockHide(1650)">
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
      <button v-if="phonePortrait && mobileLightboxMenu.open" class="mobile-lightbox-menu-scrim" type="button" aria-label="关闭图片操作菜单" @pointerdown.stop @pointerup.stop @click.stop="dismissMobileLightboxMenu"></button>
      <div v-if="phonePortrait && mobileLightboxMenu.open" class="mobile-lightbox-menu" role="menu" aria-label="图片操作" @pointerdown.stop @click.stop>
        <button type="button" role="menuitem" @click="saveMobileLightboxImage"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12M7.5 10.5 12 15l4.5-4.5M5 19v2h14v-2"/></svg><span>保存图片</span></button>
      </div>
    </div>
    <button v-if="isTimelinePage && showScrollTop && !selectionMode && !lightbox.open" class="scroll-top-button mobile-frosted-control" :class="{ 'mobile-control-hidden': phonePortrait && !mobileControlsVisible }" type="button" title="回到顶部" aria-label="回到顶部" @click="scrollTimelineToTop"><span :style="{ '--scroll-top-mask': `url(${scrollTopIcon})` }" aria-hidden="true"></span></button>
    <button v-if="!showSettings && activeNav === 'pulls'" class="add-fab" @click="showAdd = true">＋ <span>添加订阅</span>
</button>
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
<div class="modal">
<button class="modal-close" @click="showAdd = false">×</button>
<p class="eyebrow">NEW SOURCE</p>
<h2>添加订阅</h2>
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
          <div class="pane-heading"><div><h3>平台凭证</h3></div><span>4 个平台</span></div>
          <div class="platform-auth-grid">
            <article v-for="platform in platformCards" :key="platform.key" :class="['platform-auth-card', platform.key]" tabindex="0" @contextmenu.prevent="openCredentialSettings(platform.key)" @click="handleCredentialCardClick(platform.key)" @keydown.enter="openCredentialSettings(platform.key)">
              <header class="platform-auth-head"><img :class="['source-icon', { 'twitter-night-icon': platform.key === 'twitter' && isDark }]" :src="platform.image" :alt="`${platform.label}图标`"><div><h3>{{ platform.label }}</h3><span>平台账号凭证</span></div><em :class="['connection-dot', { online: platform.configured }]">{{ platform.configured ? '已连接' : '未连接' }}</em></header>
              <div class="platform-account-summary"><img :src="platform.avatar || platform.image" :alt="`${platform.account}头像`" @error="$event.target.src = platform.image"><div><span>接入账号</span><strong>{{ platform.account }}</strong></div></div>
            </article>
          </div>
        </section>
        <div class="settings-window-grid">
          <section class="settings-pane compact-settings-pane">
            <div class="pane-heading"><div><h3>网络代理</h3></div><span>{{ proxyStatus.proxyEnabled ? '已启用' : '未启用' }}</span></div>
            <form class="settings-form" @submit.prevent="saveProxy" autocomplete="off"><label>代理地址</label><input v-model="proxyForm.proxyUrl" placeholder="socks5://host.docker.internal:7890"><div class="form-actions"><button type="button" class="secondary-button" @click="testProxy" :disabled="settingsBusy">测试</button><button class="login-button" :disabled="settingsBusy">保存</button></div></form>
            <p v-if="proxyMessage" class="success-message">{{ proxyMessage }}</p>
          </section>
          <section class="settings-pane quality-settings-pane">
            <div class="pane-heading"><div><h3>画质调节</h3></div></div>
            <div class="quality-slider-list">
              <label><span><b>桌面端</b><em>{{ previewQuality.desktop }} · {{ previewQualityLabel(previewQuality.desktop) }}</em></span><input v-model.number="previewQuality.desktop" type="range" min="0" max="5" step="1" :disabled="settingsBusy" @change="savePreviewQuality"><i><small v-for="level in 6" :key="level">{{ level - 1 }}</small></i></label>
              <label><span><b>移动端</b><em>{{ previewQuality.mobile }} · {{ previewQualityLabel(previewQuality.mobile) }}</em></span><input v-model.number="previewQuality.mobile" type="range" min="0" max="5" step="1" :disabled="settingsBusy" @change="savePreviewQuality"><i><small v-for="level in 6" :key="level">{{ level - 1 }}</small></i></label>
            </div>
          </section>
          <section class="settings-pane backup-settings-pane">
            <div class="pane-heading"><div><h3>备份配置</h3><p class="backup-settings-note">备份或还原账号、订阅、过滤与显示配置，不包含已拉取的动态文件。</p></div></div>
            <div class="backup-action-grid">
              <button class="secondary-button" type="button" :disabled="settingsBusy" @click="downloadConfigurationBackup">备份</button>
              <label class="restore-file-button" :class="{ disabled: settingsBusy }"><input type="file" accept="application/json,.json" :disabled="settingsBusy" @change="restoreConfiguration"><span>还原</span></label>
            </div>
          </section>
          <section class="settings-pane compact-settings-pane"><div class="pane-heading"><div><h3>账号管理</h3></div></div><form class="settings-form account-settings-form" @submit.prevent="saveSettings" autocomplete="off"><label>账号</label><input v-model="settingsForm.username" required minlength="3" autocomplete="username"><label>密码</label><input v-model="settingsForm.newPassword" type="password" required minlength="8" autocomplete="new-password"><button class="login-button" type="submit" :disabled="settingsBusy">{{ settingsBusy ? '保存中…' : '保存' }}</button></form></section>
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
          <em :class="['connection-dot', { online: credentialPlatform.configured }]">{{ credentialPlatform.configured ? '已连接' : '未连接' }}</em>
        </div>

        <div v-if="credentialPlatform.key === 'bilibili'" class="credential-config-body">
          <div v-if="biliQRImage" class="weibo-qr"><img :src="biliQRImage" alt="哔哩哔哩登录二维码"><span>{{ biliQRStatus }}</span></div>
          <button class="login-button platform-login-button" type="button" @click="startBilibiliQR" :disabled="biliBusy && !biliQR">{{ biliQR ? '刷新二维码' : biliBusy ? '获取中…' : biliAccount.configured ? '扫码切换 B 站账号' : '扫码连接 B 站' }}</button>
          <details class="manual-credential"><summary>手动导入 Cookie</summary><form class="settings-form bili-credentials" @submit.prevent="saveBilibiliAccount" autocomplete="off"><label>完整 Cookie</label><textarea v-model="biliCredentials.cookie" rows="4" placeholder="仅在扫码不可用时使用"></textarea><label>SESSDATA</label><input v-model="biliCredentials.SESSDATA" type="password"><label>bili_jct</label><input v-model="biliCredentials.bili_jct" type="password"><label>buvid3</label><input v-model="biliCredentials.buvid3" type="password"><label>DedeUserID</label><input v-model="biliCredentials.DedeUserID" inputmode="numeric"><button class="login-button" :disabled="biliBusy">验证并保存手动凭证</button></form></details>
          <p v-if="biliError" class="login-error bili-error">{{ biliError }}</p>
        </div>

        <div v-else-if="credentialPlatform.key === 'weibo'" class="credential-config-body">
          <details class="platform-auth-details"><summary>{{ weiboAccount.cookieConfigured ? '保存备用账号密码' : '账号密码登录' }}</summary><form class="settings-form platform-auth-form weibo-password-form" @submit.prevent="loginWeiboAccount" autocomplete="off"><label>微博账号</label><input v-model="weiboPasswordCredentials.username" type="text" autocomplete="username" required placeholder="手机号、邮箱或微博账号"><label>微博密码</label><input v-model="weiboPasswordCredentials.password" type="password" autocomplete="current-password" required><p class="credential-note">{{ weiboAccount.cookieConfigured ? '当前扫码会话会继续使用，账号密码将加密保存，用于会话失效后自动重新登录。' : '账号密码会加密保存，后续可用于自动恢复微博会话。' }}</p><button class="login-button" :disabled="weiboBusy">{{ weiboBusy ? '验证中…' : weiboAccount.cookieConfigured ? '验证并保存备用密码' : '账号密码登录并保存' }}</button></form></details>
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

        <div v-else-if="credentialPlatform.key === 'twitter'" class="credential-config-body">
          <form class="settings-form platform-auth-form" @submit.prevent="saveTwitterAccount" autocomplete="off">
            <label class="credential-field"><span>twitterapi.io API Key</span><input v-model="twitterCredentials.apiKey" type="password" required autocomplete="off" placeholder="在 twitterapi.io 控制台创建的 API Key"></label>
            <label class="credential-field"><span>推特用户名</span><input v-model="twitterCredentials.username" required autocomplete="off" placeholder="例如 elonmusk，不含 @"></label>
            <p class="credential-note">API Key 仅加密保存于服务端，不会再次显示；用户名用于读取该账号的点赞来源。</p>
            <button class="login-button" :disabled="twitterBusy">{{ twitterBusy ? '验证中…' : twitterAccount.configured ? '验证并切换推特账号' : '验证并连接推特' }}</button>
          </form>
          <p v-if="twitterError" class="login-error">{{ twitterError }}</p>
        </div>
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
        <div class="platform-detail-actions"><button class="secondary-button" @click="managePlatformCredentials(selectedPlatform.key)">{{ selectedPlatform.configured ? '管理账号凭证' : '连接平台账号' }}</button><button v-if="selectedPlatform.key === 'weibo' && selectedPlatform.configured && !hasWeiboLikesSource" class="secondary-button" :disabled="sourceActionBusy !== ''" @click="addWeiboLikesSource">添加我的点赞</button><button v-if="selectedPlatform.key === 'pixiv' && selectedPlatform.configured && !hasPixivBookmarksSource" class="secondary-button" :disabled="sourceActionBusy !== ''" @click="addPixivBookmarksSource">添加 P站收藏</button><button v-if="selectedPlatform.key === 'twitter' && selectedPlatform.configured && !hasTwitterLikesSource" class="secondary-button" :disabled="sourceActionBusy !== ''" @click="addTwitterLikesSource">添加账号点赞</button><button v-if="selectedPlatform.key === 'bilibili' && selectedPlatform.configured && !hasBilibiliFavoriteOpusSource" class="secondary-button" :disabled="sourceActionBusy !== ''" @click="addBilibiliFavoriteOpusSource">添加收藏专栏</button><button v-if="selectedPlatform.key === 'bilibili' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openBilibili()">添加 UP 主</button><button v-if="selectedPlatform.key === 'weibo' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openWeibo()">添加博主</button><button v-if="selectedPlatform.key === 'pixiv' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openPixiv()">添加画师</button></div>
        <p v-if="sourceActionMessage" class="success-message source-action-message">{{ sourceActionMessage }}</p><p v-if="settingsError" class="login-error">{{ settingsError }}</p>
        <div class="configured-source-list">
          <div class="configured-source-heading"><h3>订阅列表</h3><span>{{ selectedPlatform.feeds.length }} 个</span></div>
          <article v-for="feed in selectedPlatform.feeds" :key="feed.id">
            <button class="configured-source-avatar-button" type="button" :title="`查看 ${feed.name} 的动态`" :aria-label="`查看 ${feed.name} 的动态`" @click="openFeedAuthor(feed)"><img :key="`${feed.id}:${sourceAvatar(feed, selectedPlatform)}`" class="configured-source-avatar" :src="sourceAvatar(feed, selectedPlatform)" data-fallback-index="0" :alt="`${feed.name}头像`" referrerpolicy="no-referrer" @error="handleSourceAvatarError($event, feed, selectedPlatform)"></button>
            <div><strong>{{ feed.name }}</strong><span>{{ feed.handle }} · {{ feed.schedule }}</span><div v-if="feed.tags?.length" class="source-tag-preview"><b v-for="tag in feed.tags" :key="tag">#{{ tag }}</b></div><small>{{ feed.storagePath || `${selectedPlatform.path}/${feed.name}` }}</small></div>
            <button :class="['source-enabled-toggle', { disabled: !feed.enabled }]" type="button" data-tooltip-disabled="true" :disabled="sourceActionBusy !== ''" :aria-label="feed.enabled ? '自动同步已启用，点击停用' : '自动同步已停用，点击启用'" @click="toggleFeedEnabled(feed)"><i></i>{{ sourceActionBusy === `toggle:${feed.id}` ? '保存中' : feed.enabled ? '同步中' : '已停用' }}</button>
            <div class="source-row-actions">
              <button class="source-icon-action" type="button" data-tooltip-disabled="true" aria-label="立即拉取" @click="syncSource(feed)" :disabled="sourceActionBusy !== ''"><svg :class="{ spin: sourceActionBusy === `sync:${feed.id}` }" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 9a7 7 0 0 1 11.4-2.5L20 9M4 15l2.5 2.5A7 7 0 0 0 17.9 15"/></svg></button>
              <button class="source-icon-action" title="获取历史动态" aria-label="获取历史动态" @click="syncSource(feed, true)" :disabled="sourceActionBusy !== ''"><svg :class="{ spin: sourceActionBusy === `resync:${feed.id}` }" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/></svg></button>
              <button class="source-icon-action" title="订阅设置" aria-label="订阅设置" @click="openFeedSettings(feed)"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.6v-.2h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z"/></svg></button>
              <button class="source-icon-action delete-posts-button" title="删除全部动态" aria-label="删除全部动态" @click="deleteAuthorPosts(feed.source, feed.name)" :disabled="sourceActionBusy !== '' || postActionBusy !== ''"><span class="action-icon-mask" :style="{ '--action-icon-mask': `url(${deleteIcon})` }" aria-hidden="true"></span></button>
              <button class="source-icon-action delete-source-button" title="删除订阅" aria-label="删除订阅" @click="deleteSource(feed)" :disabled="sourceActionBusy !== ''"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
            </div>
          </article>
          <div v-if="!selectedPlatform.feeds.length" class="platform-empty"><span>＋</span><strong>还没有作者来源</strong><p>{{ platformEmptyMessage(selectedPlatform.key) }}</p></div>
        </div>
      </div>
    </div>
    <div v-if="showFeedSettings && selectedFeed" class="modal-backdrop feed-detail-backdrop" @click.self="showFeedSettings = false; showStartDatePicker = false">
      <div class="modal feed-settings-modal" @click="showStartDatePicker = false">
        <button class="modal-close" @click="showFeedSettings = false; showStartDatePicker = false">×</button>
        <p class="eyebrow">SOURCE DETAILS</p><h2>{{ selectedFeed.name }}</h2><p>{{ sourceMeta[selectedFeed.source]?.label }} · {{ selectedFeed.handle }}</p>
        <form class="settings-form feed-settings-form" @submit.prevent="saveFeedSettings">
          <div class="feed-field-row feed-primary-fields">
            <label><span>Cron</span><input v-model="selectedFeed.schedule" class="cron-input" :readonly="!cronEditing" :title="nextCronExecution(selectedFeed.schedule)" required placeholder="0 6 * * *" maxlength="80" @dblclick="cronEditing = true" @blur="cronEditing = false"></label>
            <label><span>作者标签</span><input v-model="selectedFeed.tagInput" placeholder="#绘画 #日常" maxlength="120"></label>
            <div class="feed-date-field"><span>订阅起始日期</span><button class="source-start-date-button" type="button" :class="{ empty: !selectedFeed.startDate }" :aria-expanded="showStartDatePicker" title="留空表示不限制起始日期" @click.stop="showStartDatePicker ? showStartDatePicker = false : openStartDatePicker()"><time>{{ formatStartDate(selectedFeed.startDate) || '选择日期' }}</time><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4.5" y="5.5" width="15" height="14" rx="2"/><path d="M8 3.5v4M16 3.5v4M4.5 10h15"/></svg></button><div v-if="showStartDatePicker" class="start-date-picker" role="dialog" aria-label="选择订阅起始日期" @click.stop><header><button type="button" title="上个月" aria-label="上个月" @click="changeStartDatePickerMonth(-1)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m14 5-7 7 7 7"/></svg></button><strong>{{ startDatePickerLabel }}</strong><button type="button" title="下个月" aria-label="下个月" @click="changeStartDatePickerMonth(1)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m10 5 7 7-7 7"/></svg></button></header><div class="start-date-weekdays"><span v-for="day in startDatePickerWeekdays" :key="day">{{ day }}</span></div><div class="start-date-days"><button v-for="date in startDatePickerDays" :key="date.key" type="button" :class="{ outside: !date.current, selected: selectedFeed.startDate === date.key, today: startDateTodayKey() === date.key }" :aria-label="date.key" @click="selectStartDate(date)">{{ date.day }}</button></div><footer><button type="button" @click="clearStartDate">清除</button><button type="button" @click="selectStartDateToday">今天</button></footer></div></div>
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
