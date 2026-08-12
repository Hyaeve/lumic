<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import QRCode from 'qrcode'
import bilibiliIcon from '../icon/bilibili.png'
import bilibiliLineIcon from '../icon/bilibili-1.png'
import pixivIcon from '../icon/pixiv.png'
import pixivLineIcon from '../icon/pixiv-1.png'
import weiboIcon from '../icon/weibo.png'
import weiboLineIcon from '../icon/weibo-1.png'
import twitterIcon from '../icon/推特.png'
import twitterLineIcon from '../icon/推特-1.png'

const authenticated = ref(false)
const loginError = ref('')
const showSettings = ref(false)
const settingsTab = ref('sources')
const settingsError = ref('')
const settingsBusy = ref(false)
const settingsForm = ref({ username: '', currentPassword: '', newPassword: '' })
const proxyForm = ref({ proxyUrl: '' })
const proxyStatus = ref({ proxyEnabled: false, proxyUrl: '' })
const proxyMessage = ref('')
const selectedFeed = ref(null)
const showFeedSettings = ref(false)
const selectedPlatform = ref(null)
const sourceActionBusy = ref('')
const sourceActionMessage = ref('')
const confirmDialog = ref({ open: false, title: '', message: '', confirmText: '确认', cancelText: '取消', tone: 'danger' })
let confirmResolver = null
const loginBusy = ref(false)
const credentials = ref({ username: '', password: '' })
const activeNav = ref('all')
const activeSource = ref('all')
const isDark = ref(false)
const showAdd = ref(false)
const showBilibili = ref(false)
const showWeibo = ref(false)
const weiboKeyword = ref('')
const weiboResults = ref([])
const weiboIncludePast = ref(false)
const biliAccount = ref({ configured: false, userId: '' })
const biliCredentials = ref({ cookie: '', SESSDATA: '', bili_jct: '', buvid3: '', DedeUserID: '', ac_time_value: '', buvid4: '', DedeUserID__ckMd5: '' })
const biliKeyword = ref('')
const biliResults = ref([])
const biliBusy = ref(false)
const biliError = ref('')
const biliIncludePast = ref(false)
const biliQR = ref(null)
const biliQRImage = ref('')
const biliQRStatus = ref('')
let biliPollTimer = null
const pixivAccount = ref({ configured: false, userId: '', userName: '' })
const pixivRefreshToken = ref('')
const pixivBusy = ref(false)
const pixivError = ref('')
const weiboAccount = ref({ configured: false, userId: '', userName: '' })
const weiboCredentials = ref({ cookie: '', userId: '' })
const weiboQR = ref(null)
const weiboBusy = ref(false)
const weiboError = ref('')
let weiboPollTimer = null
const syncing = ref(false)
const posts = ref([])
const feeds = ref([])
const postActionBusy = ref('')
const timelineMessage = ref('')
const lightbox = ref({ open: false, media: [], index: 0, author: '', scale: 1, x: 0, y: 0, dragging: false })
let lightboxDrag = null

const fallbackPosts = [
  { id: 'wb-001', source: 'weibo', author: '林间拾光', avatar: 'https://i.pravatar.cc/96?img=47', caption: '把黄昏收藏进今天的相册里。风经过树梢，带来一点夏天的回声。', tags: ['日常', '摄影'], published: new Date(Date.now() - 42 * 60000), liked: true },
  { id: 'px-002', source: 'pixiv', author: 'Aoi Sora', avatar: 'https://i.pravatar.cc/96?img=32', caption: '新作｜雨后的玻璃温室，想画出潮湿空气里柔软的光。', tags: ['原创', '插画', '光影'], published: new Date(Date.now() - 2 * 3600000), media: ['https://images.unsplash.com/photo-1518709268805-4e9042af9f23?w=900&q=80'] },
  { id: 'bl-003', source: 'bilibili', author: '慢慢生活研究所', avatar: 'https://i.pravatar.cc/96?img=12', caption: '一期关于城市散步的记录，和你分享最近发现的三家小店。', tags: ['VLOG', '城市漫游'], published: new Date(Date.now() - 5 * 3600000) },
  { id: 'wb-004', source: 'weibo', author: '山茶花开时', avatar: 'https://i.pravatar.cc/96?img=5', caption: '今日份蓝色时刻。愿我们都能留住一些不被打扰的浪漫。', tags: ['随手拍', '生活'], published: new Date(Date.now() - 8 * 3600000), liked: true },
  { id: 'px-005', source: 'pixiv', author: 'Mori', avatar: 'https://i.pravatar.cc/96?img=20', caption: '夏日习作 #sketch #summer', tags: ['sketch', 'summer'], published: new Date(Date.now() - 24 * 3600000) }
]

const sourceMeta = {
  bilibili: { label: '哔哩哔哩', icon: 'bl', image: bilibiliIcon, lineImage: bilibiliLineIcon, color: 'blue' },
  weibo: { label: '微博', icon: 'wb', image: weiboIcon, lineImage: weiboLineIcon, color: 'coral' },
  pixiv: { label: 'Pixiv', icon: 'px', image: pixivIcon, lineImage: pixivLineIcon, color: 'violet' },
  twitter: { label: '推特', icon: 'tw', image: twitterIcon, lineImage: twitterLineIcon, color: 'twitter' }
}
const likedCount = computed(() => posts.value.filter(post => post.liked).length)
const filteredPosts = computed(() => {
  const timeline = activeNav.value === 'liked' ? posts.value.filter(post => post.liked) : posts.value
  return activeSource.value === 'all' ? timeline : timeline.filter(post => post.source === activeSource.value)
})
const sourceCount = computed(() => new Set(posts.value.map(p => p.source)).size)
const platformCards = computed(() => [
  { key: 'bilibili', label: '哔哩哔哩', short: '哔', ...sourceMeta.bilibili, configured: biliAccount.value.configured, account: biliAccount.value.configured ? `UID ${biliAccount.value.userId}` : '尚未连接账号', path: '/flow/bilibili', description: 'UP 主图文动态与专栏', feeds: feeds.value.filter(feed => feed.source === 'bilibili') },
  { key: 'weibo', label: '微博', short: '微', ...sourceMeta.weibo, configured: weiboAccount.value.configured, account: weiboAccount.value.configured ? (weiboAccount.value.userName || `UID ${weiboAccount.value.userId}`) : '尚未连接账号', path: '/flow/weibo', description: '博主动态与图文媒体', feeds: feeds.value.filter(feed => feed.source === 'weibo') },
  { key: 'pixiv', label: 'Pixiv', short: 'P', ...sourceMeta.pixiv, configured: pixivAccount.value.configured, account: pixivAccount.value.configured ? (pixivAccount.value.userName || `UID ${pixivAccount.value.userId}`) : '尚未连接账号', path: '/flow/pixiv', description: '画师作品与插画媒体', feeds: feeds.value.filter(feed => feed.source === 'pixiv') },
  { key: 'twitter', label: '推特', short: '推', ...sourceMeta.twitter, configured: false, account: '采集器尚未配置', path: '/flow/twitter', description: '推文、图片与媒体动态', feeds: feeds.value.filter(feed => feed.source === 'twitter') }
])

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
    const response = await fetch('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(credentials.value) })
    if (!response.ok) throw new Error('账号或密码不正确')
    authenticated.value = true
    await loadData()
  } catch (error) { loginError.value = error.message }
  finally { loginBusy.value = false }
}
async function loadData() {
  try {
    const [postResponse, feedResponse, biliFeedResponse, weiboFeedResponse] = await Promise.all([fetch('/api/posts'), fetch('/api/feeds'), fetch('/api/bilibili/subscriptions'), fetch('/api/weibo/subscriptions')])
    if (!postResponse.ok || !feedResponse.ok || !biliFeedResponse.ok || !weiboFeedResponse.ok) throw new Error('api unavailable')
    posts.value = await postResponse.json()
    feeds.value = [...await feedResponse.json(), ...await biliFeedResponse.json(), ...await weiboFeedResponse.json()]
  } catch { posts.value = fallbackPosts; feeds.value = [] }
}
async function syncNow() {
  syncing.value = true
  try { await fetch('/api/sync', { method: 'POST' }) } catch {}
  setTimeout(() => { syncing.value = false }, 900)
}
async function openSettings(tab = 'sources') {
  settingsTab.value = tab; showSettings.value = true; activeNav.value = 'settings'; settingsError.value = ''; proxyMessage.value = ''; pixivError.value = ''; weiboError.value = ''
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
    if (result.status === 'connected') { biliAccount.value = { configured: true, userId: result.userId }; biliQR.value = null; biliQRImage.value = ''; biliBusy.value = false; stopBilibiliPolling(); return }
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
    const response = await fetch('/api/pixiv/account', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ refreshToken: pixivRefreshToken.value.trim() }) })
    if (!response.ok) throw new Error(await responseError(response, 'Pixiv 凭证验证失败'))
    pixivAccount.value = await response.json(); pixivRefreshToken.value = ''
  } catch (error) { pixivError.value = error.message } finally { pixivBusy.value = false }
}
function stopWeiboPolling() { if (weiboPollTimer) clearTimeout(weiboPollTimer); weiboPollTimer = null }
async function pollWeiboQR() {
  if (!weiboQR.value?.id) return
  try {
    const response = await fetch(`/api/weibo/qr?id=${encodeURIComponent(weiboQR.value.id)}`)
    if (!response.ok) throw new Error(await responseError(response, '微博扫码状态查询失败'))
    const result = await response.json()
    if (result.status === 'connected') { weiboAccount.value = { configured: true, userId: result.userId, userName: result.userName }; weiboQR.value = null; weiboBusy.value = false; return }
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
    const response = await fetch('/api/weibo/subscriptions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId: user.userId, name: user.name, avatar: user.avatar, includePast: weiboIncludePast.value, schedule: '每 6 小时' }) })
    if (!response.ok) throw new Error(await responseError(response, response.status === 409 ? '已经订阅该微博博主' : '订阅失败'))
    feeds.value.push(await response.json())
  } catch (error) { weiboError.value = error.message } finally { weiboBusy.value = false }
}
async function subscribeBilibili(user) {
  biliBusy.value = true; biliError.value = ''
  try {
    const response = await fetch('/api/bilibili/subscriptions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId: user.userId, name: user.name, avatar: user.avatar, includePast: biliIncludePast.value, schedule: '每 6 小时' }) })
    if (!response.ok) throw new Error(response.status === 409 ? '已经订阅该 UP 主' : '订阅失败')
    feeds.value.push(await response.json())
  } catch (error) { biliError.value = error.message } finally { biliBusy.value = false }
}
function openPlatformSettings(platform) {
  selectedPlatform.value = platform
}
function managePlatformCredentials(platformKey) {
  selectedPlatform.value = null
  settingsTab.value = 'platforms'
  settingsError.value = ''
  if (platformKey === 'bilibili' && !biliAccount.value.configured) startBilibiliQR()
  if (platformKey === 'weibo' && !weiboAccount.value.configured) startWeiboQR()
}
function openFeedSettings(feed) {
  selectedFeed.value = { ...feed, contentTypes: [...(feed.contentTypes || [])] }
  showFeedSettings.value = true
}
async function saveFeedSettings() {
  if (!selectedFeed.value) return
  settingsBusy.value = true; settingsError.value = ''
  try {
    if ((selectedFeed.value.source === 'bilibili' && selectedFeed.value.id.startsWith('bili-')) || (selectedFeed.value.source === 'weibo' && selectedFeed.value.id.startsWith('weibo-'))) {
      const response = await fetch(sourceOperationEndpoint(selectedFeed.value), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(selectedFeed.value) })
      if (!response.ok) throw new Error(await responseError(response, '来源设置保存失败'))
      const saved = await response.json(); const index = feeds.value.findIndex(feed => feed.id === saved.id)
      if (index >= 0) feeds.value[index] = saved
      if (selectedPlatform.value) {
        const platformFeedIndex = selectedPlatform.value.feeds.findIndex(feed => feed.id === saved.id)
        if (platformFeedIndex >= 0) selectedPlatform.value.feeds[platformFeedIndex] = saved
      }
    } else {
      const index = feeds.value.findIndex(feed => feed.id === selectedFeed.value.id)
      if (index >= 0) feeds.value[index] = { ...selectedFeed.value }
    }
    showFeedSettings.value = false
  } catch (error) { settingsError.value = error.message } finally { settingsBusy.value = false }
}
function sourceOperationEndpoint(feed) {
  if (feed.source === 'bilibili' && feed.id.startsWith('bili-')) return '/api/bilibili/subscriptions'
  if (feed.source === 'weibo' && feed.id.startsWith('weibo-')) return '/api/weibo/subscriptions'
  return '/api/feeds'
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
  lightbox.value.x = 0
  lightbox.value.y = 0
  lightbox.value.dragging = false
  lightboxDrag = null
}
function openLightbox(post, index) {
  lightbox.value = { open: true, media: post.media || [], index, author: post.author, scale: 1, x: 0, y: 0, dragging: false }
}
function closeLightbox() {
  lightbox.value = { open: false, media: [], index: 0, author: '', scale: 1, x: 0, y: 0, dragging: false }
  lightboxDrag = null
}
function moveLightbox(step) {
  const total = lightbox.value.media.length
  if (total > 1) {
    lightbox.value.index = (lightbox.value.index + step + total) % total
    resetLightboxView()
  }
}
function zoomLightbox(event) {
  const step = event.deltaY < 0 ? 0.15 : -0.15
  lightbox.value.scale = Math.min(5, Math.max(0.5, Number((lightbox.value.scale + step).toFixed(2))))
  if (lightbox.value.scale <= 1) {
    lightbox.value.x = 0
    lightbox.value.y = 0
  }
}
function startLightboxDrag(event) {
  if (lightbox.value.scale <= 1) return
  event.currentTarget.setPointerCapture(event.pointerId)
  lightbox.value.dragging = true
  lightboxDrag = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, imageX: lightbox.value.x, imageY: lightbox.value.y }
}
function moveLightboxDrag(event) {
  if (!lightboxDrag || lightboxDrag.pointerId !== event.pointerId) return
  lightbox.value.x = lightboxDrag.imageX + event.clientX - lightboxDrag.startX
  lightbox.value.y = lightboxDrag.imageY + event.clientY - lightboxDrag.startY
}
function stopLightboxDrag(event) {
  if (!lightboxDrag || lightboxDrag.pointerId !== event.pointerId) return
  if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  lightbox.value.dragging = false
  lightboxDrag = null
}
function handleGlobalKeydown(event) {
  if (lightbox.value.open) {
    if (event.key === 'Escape') closeLightbox()
    if (event.key === 'ArrowLeft') moveLightbox(-1)
    if (event.key === 'ArrowRight') moveLightbox(1)
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
    window.setTimeout(() => { if (timelineMessage.value === '动态已从时间线删除') timelineMessage.value = '' }, 2500)
  } catch (error) { timelineMessage.value = error.message } finally { postActionBusy.value = '' }
}
async function togglePostLike(post) {
  if (postActionBusy.value) return
  const previous = Boolean(post.liked)
  post.liked = !previous
  postActionBusy.value = `like:${post.id}`
  timelineMessage.value = ''
  try {
    const response = await fetch(`/api/posts?id=${encodeURIComponent(post.id)}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ liked: post.liked }) })
    if (!response.ok) throw new Error(await responseError(response, '点赞状态保存失败'))
    const updated = await response.json()
    post.liked = Boolean(updated.liked)
  } catch (error) {
    post.liked = previous
    timelineMessage.value = error.message
  } finally {
    postActionBusy.value = ''
  }
}
function navigateTo(nav, source = activeSource.value) {
  showSettings.value = false
  selectedPlatform.value = null
  showFeedSettings.value = false
  activeNav.value = nav
  activeSource.value = source
}
function closeSettingsPage() { navigateTo(activeSource.value === 'all' ? 'all' : 'source') }
function formatFans(count) { return count >= 10000 ? `${(count / 10000).toFixed(1)}万` : count }
function platformEmptyMessage(platformKey) {
  if (platformKey === 'bilibili') return '点击“添加 UP 主”开始订阅图文与专栏。'
  if (platformKey === 'weibo') return '点击“添加博主”开始订阅微博动态。'
  if (platformKey === 'twitter') return '推特账号连接与作者采集器尚未开放。'
  return '作者订阅连接器将在后续版本开放。'
}
async function checkSession() {
  try {
    const response = await fetch('/api/posts')
    if (!response.ok) throw new Error('unauthorized')
    posts.value = await response.json()
    authenticated.value = true
    const [feedResponse, biliFeedResponse, weiboFeedResponse] = await Promise.all([fetch('/api/feeds'), fetch('/api/bilibili/subscriptions'), fetch('/api/weibo/subscriptions')])
    if (feedResponse.ok && biliFeedResponse.ok && weiboFeedResponse.ok) feeds.value = [...await feedResponse.json(), ...await biliFeedResponse.json(), ...await weiboFeedResponse.json()]
  } catch { authenticated.value = false }
}
onMounted(() => { checkSession(); window.addEventListener('keydown', handleGlobalKeydown) })
onUnmounted(() => { stopWeiboPolling(); stopBilibiliPolling(); window.removeEventListener('keydown', handleGlobalKeydown); if (confirmResolver) closeConfirmDialog(false) })
</script>

<template>
  <div v-if="!authenticated" class="login-shell">
    <div class="login-panel">
      <div class="login-brand">
<span class="brand-mark">✦</span>
<strong>Lumic</strong>
<small>拾光</small>
</div>
      <p class="eyebrow">A QUIET PLACE FOR YOUR MOMENTS</p>
      <h1>欢迎回来</h1>
      <p class="login-subtitle">登录后继续收集你喜欢的动态。</p>
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
  <div v-else class="app-shell" :class="{ dark: isDark }">
    <aside class="sidebar">
      <div class="brand">
<span class="brand-mark">✦</span>
<span>Lumic</span>
<small>拾光</small>
</div>
      <nav class="main-nav">
        <button :class="{ active: activeNav === 'all' }" @click="navigateTo('all', 'all')">
<span class="nav-line-symbol">⌂</span> 全部动态 <b>{{ posts.length }}</b>
</button>
        <button v-for="(meta, key) in sourceMeta" :key="key" :class="{ active: activeNav === 'source' && activeSource === key }" @click="navigateTo('source', key)">
<img class="sidebar-source-icon" :src="meta.lineImage" :alt="`${meta.label}线条图标`">{{ meta.label }} <b>{{ posts.filter(p => p.source === key).length }}</b>
</button>
        <button :class="{ active: activeNav === 'liked' }" @click="navigateTo('liked', 'all')">
<span class="nav-line-symbol">♡</span> 我的点赞 <b>{{ likedCount }}</b>
</button>
        <button :class="{ active: activeNav === 'pulls' }" @click="navigateTo('pulls')">
<span class="nav-line-symbol">↻</span> 拉取列表 <b>{{ feeds.length }}</b>
</button>
        <button :class="{ active: activeNav === 'settings' }" @click="openSettings()">
<span class="nav-line-symbol">⚙</span> 设置</button>
      </nav>
      <div class="sidebar-bottom">
        <div class="profile">
<div class="profile-avatar">拾</div>
<div>
<strong>我的空间</strong>
<small>本地收藏</small>
</div>
<span>⋮</span>
</div>
      </div>
    </aside>

    <main v-if="!showSettings && activeNav !== 'pulls'" class="content">
      <header class="topbar">
<div>
<p class="eyebrow">{{ activeNav === 'liked' ? 'LIKED MOMENTS' : 'SAVED MOMENTS' }} · {{ new Date().toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' }) }}</p>
<h1>{{ activeNav === 'liked' ? '我的点赞' : '早上好，拾光者' }} <span>{{ activeNav === 'liked' ? '♡' : '☼' }}</span>
</h1>
<p class="subtitle">{{ activeNav === 'liked' ? '这里收集了你标记为喜欢的动态。' : '这里有你关注的世界，和刚刚发生的一切。' }}</p>
</div>
<div class="header-actions">
<button class="icon-button" @click="isDark = !isDark" :title="isDark ? '切换日间主题' : '切换夜间主题'">{{ isDark ? '☀' : '☾' }}</button>
<button class="sync-button" @click="runFullSync">
<span :class="{ spin: syncing }">↻</span> {{ syncing ? '同步中' : '立即同步' }}</button>
</div>
</header>
      <section class="stats">
<div class="stat-card">
<div class="stat-icon mint">✦</div>
<div>
<span>今日新动态</span>
<strong>{{ posts.length }} <em>+12%</em>
</strong>
</div>
<small>较昨日</small>
</div>
<div class="stat-card">
<div class="stat-icon rose">♡</div>
<div>
<span>我的点赞</span>
<strong>{{ likedCount }}</strong>
</div>
<small>持续增长中</small>
</div>
<div class="stat-card">
<div class="stat-icon sand">◷</div>
<div>
<span>关注来源</span>
<strong>{{ sourceCount }} <em class="neutral">个</em>
</strong>
</div>
<small>同步正常</small>
</div>
</section>
      <div class="section-heading">
<div>
<h2>{{ activeNav === 'liked' ? '点赞动态' : '最新动态' }}</h2>
<p>{{ activeNav === 'liked' ? `共 ${likedCount} 条 · 按时间顺序排列` : '按时间顺序排列 · 自动同步于 10 分钟前' }}</p>
</div>
<div class="filters">
<button v-for="(meta, key) in { all: { label: '全部', icon: '✦' }, ...sourceMeta }" :key="key" :class="{ selected: activeSource === key }" @click="activeSource = key">
<span v-if="key === 'all'">✦</span>
<img v-else class="source-icon" :src="meta.image" :alt="`${meta.label}图标`">{{ meta.label }}</button>
<button class="view-button">▦</button>
</div>
</div>
      <p v-if="timelineMessage" class="timeline-message">{{ timelineMessage }}</p>
      <section class="feed-list">
        <article v-for="post in filteredPosts" :key="post.id" class="post-card">
<div class="post-head">
<img :src="post.avatar" :alt="post.author">
<div class="author">
<strong>{{ post.author }}</strong>
<span>{{ relativeTime(post.published) }}</span>
</div>
<span :class="['source-pill', 'post-source-pill', sourceMeta[post.source].color]">
<img class="source-icon" :src="sourceMeta[post.source].image" :alt="`${sourceMeta[post.source].label}图标`">{{ sourceMeta[post.source].label }}</span>
</div>
<p v-if="post.caption" class="caption">{{ post.caption }}</p>
<div v-if="post.media?.length" :class="['media-grid', `media-count-${Math.min(post.media.length, 4)}`]">
<button v-for="(media, mediaIndex) in post.media" :key="media" type="button" :aria-label="`查看 ${post.author} 的第 ${mediaIndex + 1} 张图片`" @click="openLightbox(post, mediaIndex)"><img :src="media" alt=""></button>
</div>
<div class="tag-row">
<span v-for="tag in post.tags" :key="tag"># {{ tag }}</span>
</div>
<div class="post-foot">
<span>◷ {{ new Date(post.published).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }}</span>
<div class="post-foot-actions">
<button :class="['post-like-button', { liked: post.liked }]" :disabled="postActionBusy === `like:${post.id}`" :title="post.liked ? '取消点赞' : '点赞'" :aria-label="post.liked ? '取消点赞' : '点赞'" @click="togglePostLike(post)">{{ post.liked ? '♥' : '♡' }}</button>
<button title="打开原动态" aria-label="打开原动态">↗</button>
<button class="post-delete-button" :disabled="postActionBusy === post.id" title="删除这条动态" @click="deletePost(post)"><span>⌫</span>{{ postActionBusy === post.id ? '删除中…' : '删除' }}</button>
</div>
</div>
</article>
<div v-if="!filteredPosts.length" class="empty">{{ activeNav === 'liked' ? '还没有点赞的动态' : '还没有这个来源的动态' }}</div>
</section>
    </main>
    <main v-if="!showSettings && activeNav === 'pulls'" class="content pulls-page">
      <header class="topbar"><div><p class="eyebrow">SOURCE PULLS · {{ feeds.length }} SOURCES</p><h1>拉取列表</h1><p class="subtitle">管理订阅作者，并按来源拉取最新动态。</p></div><div class="header-actions"><button class="sync-button" :disabled="syncing" @click="runFullSync"><span :class="{ spin: syncing }">↻</span>{{ syncing ? '拉取中' : '全部拉取' }}</button></div></header>
      <p v-if="timelineMessage" class="timeline-message">{{ timelineMessage }}</p>
      <section class="pull-list">
        <article v-for="feed in feeds" :key="feed.id" class="pull-card">
          <img class="pull-avatar" :src="feed.avatar || sourceMeta[feed.source]?.image || '/favicon.ico'" :alt="feed.name" @error="$event.target.src = sourceMeta[feed.source]?.image || '/favicon.ico'">
          <div class="pull-info"><div class="pull-title"><strong>{{ feed.name }}</strong><span :class="['source-pill', sourceMeta[feed.source].color]"><img class="source-icon" :src="sourceMeta[feed.source].image" :alt="sourceMeta[feed.source].label">{{ sourceMeta[feed.source].label }}</span></div><span class="pull-handle">{{ feed.handle }}</span><small>{{ feed.lastSyncMessage || (feed.lastSyncedAt ? `上次拉取：${relativeTime(feed.lastSyncedAt)}` : '尚未拉取') }}</small></div>
          <div class="pull-status"><i :class="['pull-dot', feed.lastSyncStatus]"></i><span>{{ feed.lastSyncStatus === 'success' ? `新增 ${feed.lastSyncCount || 0} 条` : feed.lastSyncStatus === 'failed' ? '拉取失败' : '待拉取' }}</span></div>
          <div class="pull-actions"><button class="pull-action" :disabled="sourceActionBusy !== ''" @click="syncSource(feed)">{{ sourceActionBusy === `sync:${feed.id}` ? '拉取中…' : '立即拉取' }}</button><button class="pull-action secondary" :disabled="sourceActionBusy !== ''" @click="syncSource(feed, true)">{{ sourceActionBusy === `resync:${feed.id}` ? '重拉中…' : '重新拉取' }}</button></div>
        </article>
        <div v-if="!feeds.length" class="empty">还没有订阅作者，请先添加 UP 主或微博博主。</div>
      </section>
    </main>
    <div v-if="lightbox.open" class="lightbox-layer" role="dialog" aria-modal="true" :aria-label="`${lightbox.author} 的动态图片`" @click.self="closeLightbox" @wheel.prevent="zoomLightbox">
      <button class="lightbox-close" type="button" title="关闭大图" @click="closeLightbox">×</button>
      <button v-if="lightbox.media.length > 1" class="lightbox-nav lightbox-prev" type="button" title="上一张" @click="moveLightbox(-1)">‹</button>
      <figure><img :src="lightbox.media[lightbox.index]" :alt="`${lightbox.author} 的动态图片 ${lightbox.index + 1}`" :class="{ dragging: lightbox.dragging, pannable: lightbox.scale > 1 }" :style="{ transform: `translate3d(${lightbox.x}px, ${lightbox.y}px, 0) scale(${lightbox.scale})` }" draggable="false" @pointerdown.prevent="startLightboxDrag" @pointermove.prevent="moveLightboxDrag" @pointerup="stopLightboxDrag" @pointercancel="stopLightboxDrag"><figcaption>{{ lightbox.media.length > 1 ? `${lightbox.index + 1} / ${lightbox.media.length} · ` : '' }}{{ Math.round(lightbox.scale * 100) }}%</figcaption></figure>
      <button v-if="lightbox.media.length > 1" class="lightbox-nav lightbox-next" type="button" title="下一张" @click="moveLightbox(1)">›</button>
    </div>
    <button v-if="!showSettings" class="add-fab" @click="showAdd = true">＋ <span>添加来源</span>
</button>
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
<div class="modal">
<button class="modal-close" @click="showAdd = false">×</button>
<p class="eyebrow">NEW SOURCE</p>
<h2>添加一个新来源</h2>
<p>连接账号后，Lumic 会帮你把喜欢的内容收进同一条时间线。</p>
<div class="connect-options">
<button @click="openWeibo">
<img class="source-icon" :src="sourceMeta.weibo.image" alt="微博图标">添加微博博主</button>
<button @click="showAdd = false; openSettings('platforms')">
<img class="source-icon" :src="sourceMeta.pixiv.image" alt="pixiv图标">连接 pixiv</button>
<button @click="openBilibili">
<img class="source-icon" :src="sourceMeta.bilibili.image" alt="哔哩哔哩图标">连接哔哩哔哩</button>
<button @click="showAdd = false; openSettings('platforms')">
<img class="source-icon" :src="sourceMeta.twitter.image" alt="推特图标">连接推特</button>
</div>
</div>
</div>
    <div v-if="showBilibili" class="modal-backdrop" @click.self="showBilibili = false">
      <div class="modal bili-modal">
        <button class="modal-close" @click="showBilibili = false">×</button>
        <p class="eyebrow">BILIBILI SOURCE</p>
        <h2>订阅 UP 主图文</h2>
        <p>仅收集图文动态与专栏，视频动态和转发视频不会进入时间线。账号凭证请在设置页面管理。</p>
        <div class="bili-account"><span>已连接 B 站账号 · UID {{ biliAccount.userId }}</span><button @click="showBilibili = false; openSettings('platforms')">管理凭证</button></div>
        <form class="bili-search" @submit.prevent="searchBilibili"><input v-model="biliKeyword" placeholder="搜索 UP 主昵称" maxlength="40" required><button :disabled="biliBusy">⌕ 搜索</button></form>
        <label class="history-option"><input v-model="biliIncludePast" type="checkbox"> 首次订阅时拉取历史图文与专栏</label>
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
        <p>搜索并订阅微博博主，后续动态将按计划进入时间线。账号凭证请在设置页面管理。</p>
        <div class="bili-account"><span>已连接微博账号 · {{ weiboAccount.userName || `UID ${weiboAccount.userId}` }}</span><button @click="showWeibo = false; openSettings('platforms')">管理凭证</button></div>
        <form class="bili-search" @submit.prevent="searchWeibo"><input v-model="weiboKeyword" placeholder="搜索微博博主昵称" maxlength="40" required><button :disabled="weiboBusy">⌕ 搜索</button></form>
        <label class="history-option"><input v-model="weiboIncludePast" type="checkbox"> 首次订阅时拉取历史动态</label>
        <div class="bili-results">
          <article v-for="user in weiboResults" :key="user.userId"><img :src="user.avatar" :alt="user.name"><div><strong>{{ user.name }}</strong><span>UID {{ user.userId }} · 粉丝 {{ formatFans(user.fans) }}</span><p>{{ user.description || '这位博主还没有填写简介' }}</p></div><button @click="subscribeWeibo(user)" :disabled="weiboBusy">订阅</button></article>
          <div v-if="!weiboResults.length" class="bili-placeholder">输入昵称搜索微博博主，然后选择订阅</div>
        </div>
        <p v-if="weiboError" class="login-error bili-error">{{ weiboError }}</p>
      </div>
    </div>
    <main v-if="showSettings" class="settings-page">
      <div class="settings-page-inner">
        <nav class="settings-tabs" aria-label="设置分类">
          <button :class="{ active: settingsTab === 'sources' }" @click="settingsTab = 'sources'; settingsError = ''">来源管理</button>
          <button :class="{ active: settingsTab === 'platforms' }" @click="settingsTab = 'platforms'; settingsError = ''">平台凭证</button>
          <button :class="{ active: settingsTab === 'network' }" @click="settingsTab = 'network'; settingsError = ''">网络代理</button>
          <button :class="{ active: settingsTab === 'security' }" @click="settingsTab = 'security'; settingsError = ''">登录安全</button>
        </nav>
        <section v-if="settingsTab === 'network'" class="settings-pane compact-settings-pane">
          <div class="pane-heading"><div><h3>网络代理</h3><p>统一配置后端访问外部平台时使用的 HTTP、HTTPS 或 SOCKS5 代理。</p></div><span>{{ proxyStatus.proxyEnabled ? '已启用' : '未启用' }}</span></div>
          <div v-if="proxyStatus.proxyEnabled" class="setting-status">当前代理：{{ proxyStatus.proxyUrl }}</div>
          <form class="settings-form" @submit.prevent="saveProxy" autocomplete="off"><label>代理地址</label><input v-model="proxyForm.proxyUrl" placeholder="socks5://host.docker.internal:7890"><p class="credential-note">Docker 中的 127.0.0.1 指向容器自身；访问宿主机代理请使用 host.docker.internal。</p><div class="form-actions"><button type="button" class="secondary-button" @click="testProxy" :disabled="settingsBusy">测试连接</button><button class="login-button" :disabled="settingsBusy">保存代理</button><button type="button" class="danger-link" @click="proxyForm.proxyUrl = ''; saveProxy()">关闭代理</button></div></form>
          <p v-if="proxyMessage" class="success-message">{{ proxyMessage }}</p>
        </section>
        <section v-if="settingsTab === 'platforms'" class="settings-pane platform-credentials-pane">
          <div class="pane-heading"><div><h3>平台凭证</h3><p>各平台凭证仅由服务端验证，并加密保存在本地数据目录。</p></div><span>4 个平台</span></div>
          <div class="platform-auth-grid">
            <article class="platform-auth-card bilibili">
              <header class="platform-auth-head"><img class="source-icon" :src="sourceMeta.bilibili.image" alt="哔哩哔哩图标"><div><h3>哔哩哔哩</h3><span>手机客户端扫码登录</span></div><em :class="['connection-dot', { online: biliAccount.configured }]">{{ biliAccount.configured ? '已连接' : '未连接' }}</em></header>
              <p v-if="biliAccount.configured">当前账号：UID {{ biliAccount.userId }}。扫码可切换账号。</p><p v-else>登录凭证会自动获取并加密保存。</p>
              <div v-if="biliQRImage" class="weibo-qr"><img :src="biliQRImage" alt="哔哩哔哩登录二维码"><span>{{ biliQRStatus }}</span></div><button class="login-button platform-login-button" type="button" @click="startBilibiliQR" :disabled="biliBusy && !biliQR">{{ biliQR ? '刷新二维码' : biliBusy ? '获取中…' : biliAccount.configured ? '扫码切换 B 站账号' : '扫码连接 B 站' }}</button>
              <details class="manual-credential"><summary>高级：手动导入 Cookie</summary><form class="settings-form bili-credentials" @submit.prevent="saveBilibiliAccount" autocomplete="off"><label>完整 Cookie</label><textarea v-model="biliCredentials.cookie" rows="4" placeholder="仅在扫码不可用时使用"></textarea><label>SESSDATA</label><input v-model="biliCredentials.SESSDATA" type="password"><label>bili_jct</label><input v-model="biliCredentials.bili_jct" type="password"><label>buvid3</label><input v-model="biliCredentials.buvid3" type="password"><label>DedeUserID</label><input v-model="biliCredentials.DedeUserID" inputmode="numeric"><button class="login-button" :disabled="biliBusy">验证并保存手动凭证</button></form></details><p v-if="biliError" class="login-error bili-error">{{ biliError }}</p>
            </article>
            <article class="platform-auth-card weibo">
              <header class="platform-auth-head"><img class="source-icon" :src="sourceMeta.weibo.image" alt="微博图标"><div><h3>微博</h3><span>扫码或 Cookie 登录</span></div><em :class="['connection-dot', { online: weiboAccount.configured }]">{{ weiboAccount.configured ? '已连接' : '未连接' }}</em></header>
              <p v-if="weiboAccount.configured">当前账号：{{ weiboAccount.userName || `UID ${weiboAccount.userId}` }}。</p><p v-else>优先使用微博客户端扫码，也可导入浏览器登录 Cookie。</p>
              <div v-if="weiboQR" class="weibo-qr"><img :src="weiboQR.image.startsWith('//') ? `https:${weiboQR.image}` : weiboQR.image" alt="微博登录二维码"><span>请在二维码过期前扫码并确认</span></div><button class="login-button platform-login-button" type="button" @click="startWeiboQR" :disabled="weiboBusy && !weiboQR">{{ weiboQR ? '刷新二维码' : weiboBusy ? '获取中…' : weiboAccount.configured ? '扫码切换微博账号' : '扫码连接微博' }}</button><details class="manual-credential"><summary>高级：手动导入 Cookie</summary><form class="settings-form bili-credentials" @submit.prevent="saveWeiboAccount" autocomplete="off"><label>微博 UID</label><input v-model="weiboCredentials.userId" inputmode="numeric" required placeholder="个人主页地址中的数字 UID"><label>完整 Cookie</label><textarea v-model="weiboCredentials.cookie" rows="4" required placeholder="可粘贴浏览器请求头中的 Cookie: 完整内容"></textarea><p class="credential-note">请使用已成功打开微博的同一网络出口获取 Cookie；保存前会验证账号资料，不会回显原始 Cookie。</p><button class="login-button" :disabled="weiboBusy">{{ weiboBusy ? '验证中…' : '验证并保存 Cookie' }}</button></form></details><p v-if="weiboError" class="login-error">{{ weiboError }}</p>
            </article>
            <article class="platform-auth-card pixiv">
              <header class="platform-auth-head"><img class="source-icon" :src="sourceMeta.pixiv.image" alt="Pixiv图标"><div><h3>Pixiv</h3><span>OAuth refresh_token</span></div><em :class="['connection-dot', { online: pixivAccount.configured }]">{{ pixivAccount.configured ? '已连接' : '未连接' }}</em></header>
              <p v-if="pixivAccount.configured">当前账号：{{ pixivAccount.userName || pixivAccount.userId }}。重新保存时需填写新 token。</p><p v-else>使用 OAuth refresh_token 连接，不保存账号密码。</p>
              <form class="settings-form platform-auth-form" @submit.prevent="savePixivAccount"><label>refresh_token</label><input v-model="pixivRefreshToken" type="password" required autocomplete="off"><p class="credential-note">服务端需配置 Pixiv OAuth 客户端环境变量，token 将加密保存。</p><button class="login-button" :disabled="pixivBusy">{{ pixivBusy ? '验证中…' : '验证并保存 Pixiv' }}</button></form><p v-if="pixivError" class="login-error">{{ pixivError }}</p>
            </article>
            <article class="platform-auth-card twitter">
              <header class="platform-auth-head"><img class="source-icon" :src="sourceMeta.twitter.image" alt="推特图标"><div><h3>推特</h3><span>账号连接与作者采集</span></div><em class="connection-dot">未开放</em></header>
              <p>推特已加入来源分类、侧栏筛选和本地媒体目录；账号授权与作者采集器将在后续版本开放。</p>
              <button class="secondary-button platform-login-button" type="button" disabled>连接能力开发中</button>
            </article>
          </div>
        </section>
        <section v-if="settingsTab === 'sources'" class="settings-pane source-platform-pane">
          <div class="pane-heading"><div><h3>来源管理</h3><p>集中查看平台连接状态、订阅作者和内容存储位置。</p></div><span>4 个平台</span></div>
          <div class="platform-source-grid">
            <article v-for="platform in platformCards" :key="platform.key" :class="['platform-source-card', platform.key]" @contextmenu.prevent="openPlatformSettings(platform)">
              <div class="platform-card-head"><img class="source-icon" :src="platform.image" :alt="`${platform.label}图标`"><span :class="['connection-dot', { online: platform.configured }]">{{ platform.configured ? '已连接' : '未连接' }}</span></div>
              <h4>{{ platform.label }}</h4><p>{{ platform.description }}</p>
              <dl><div><dt>账号</dt><dd>{{ platform.account }}</dd></div><div><dt>已配置来源</dt><dd>{{ platform.feeds.length }} 个</dd></div><div><dt>内容目录</dt><dd>{{ platform.path }}</dd></div></dl>
              <button @click="openPlatformSettings(platform)">详细设置 <span>→</span></button>
            </article>
          </div>
        </section>
        <section v-if="settingsTab === 'security'" class="settings-pane compact-settings-pane"><div class="pane-heading"><div><h3>登录安全</h3><p>更新本地管理账号。保存后请使用新的账号和密码登录。</p></div><span>密码至少 8 位</span></div><form class="settings-form" @submit.prevent="saveSettings" autocomplete="off"><label>新账号</label><input v-model="settingsForm.username" required minlength="3" autocomplete="off"><label>当前密码</label><input v-model="settingsForm.currentPassword" type="password" required autocomplete="current-password"><label>新密码</label><input v-model="settingsForm.newPassword" type="password" required minlength="8" autocomplete="new-password"><button class="login-button" type="submit" :disabled="settingsBusy">{{ settingsBusy ? '保存中…' : '保存设置' }}</button></form></section>
        <p v-if="settingsError" class="login-error settings-page-error">{{ settingsError }}</p>
      </div>
    </main>
    <div v-if="selectedPlatform" class="modal-backdrop platform-detail-backdrop" @click.self="selectedPlatform = null">
      <div class="modal platform-detail-modal">
        <button class="modal-close" @click="selectedPlatform = null">×</button>
        <div class="platform-detail-title"><img class="source-icon" :src="selectedPlatform.image" alt="平台图标"><div><p class="eyebrow">PLATFORM SOURCE</p><h2>{{ selectedPlatform.label }}</h2></div><span :class="['connection-dot', { online: selectedPlatform.configured }]">{{ selectedPlatform.configured ? '已连接' : '未连接' }}</span></div>
        <div class="platform-detail-summary"><div><span>当前账号</span><strong>{{ selectedPlatform.account }}</strong></div><div><span>内容目录</span><strong>{{ selectedPlatform.path }}</strong></div><div><span>作者来源</span><strong>{{ selectedPlatform.feeds.length }} 个</strong></div></div>
        <div class="platform-detail-actions"><button v-if="selectedPlatform.key !== 'twitter'" class="secondary-button" @click="managePlatformCredentials(selectedPlatform.key)">{{ selectedPlatform.configured ? '管理账号凭证' : '连接平台账号' }}</button><button v-if="selectedPlatform.key === 'bilibili' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openBilibili()">添加 UP 主</button><button v-if="selectedPlatform.key === 'weibo' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openWeibo()">添加博主</button></div>
        <p v-if="sourceActionMessage" class="success-message source-action-message">{{ sourceActionMessage }}</p><p v-if="settingsError" class="login-error">{{ settingsError }}</p>
        <div class="configured-source-list">
          <div class="configured-source-heading"><h3>已配置作者</h3><span>{{ selectedPlatform.feeds.length }} 个</span></div>
          <article v-for="feed in selectedPlatform.feeds" :key="feed.id">
            <img class="configured-source-avatar" :src="feed.avatar || selectedPlatform.image" :alt="`${feed.name}头像`" @error="$event.target.src = selectedPlatform.image">
            <div><strong>{{ feed.name }}</strong><span>{{ feed.handle }} · {{ feed.schedule }}</span><small>{{ feed.storagePath || `${selectedPlatform.path}/${feed.name}` }}</small></div>
            <em :class="{ disabled: !feed.enabled }">{{ feed.enabled ? '同步中' : '已停用' }}</em>
            <div class="source-row-actions">
              <button class="source-icon-action" title="立即拉取最新动态" aria-label="立即拉取最新动态" @click="syncSource(feed)" :disabled="sourceActionBusy !== ''"><span :class="{ spin: sourceActionBusy === `sync:${feed.id}` }">↻</span></button>
              <button class="source-icon-action" title="重新拉取全部历史动态" aria-label="重新拉取全部历史动态" @click="syncSource(feed, true)" :disabled="sourceActionBusy !== ''"><span :class="{ spin: sourceActionBusy === `resync:${feed.id}` }">⟳</span></button>
              <button class="source-icon-action" title="来源设置" aria-label="来源设置" @click="openFeedSettings(feed)"><span>⚙</span></button>
              <button class="source-icon-action delete-source-button" title="删除来源" aria-label="删除来源" @click="deleteSource(feed)" :disabled="sourceActionBusy !== ''"><span>×</span></button>
            </div>
          </article>
          <div v-if="!selectedPlatform.feeds.length" class="platform-empty"><span>＋</span><strong>还没有作者来源</strong><p>{{ platformEmptyMessage(selectedPlatform.key) }}</p></div>
        </div>
      </div>
    </div>
    <div v-if="showFeedSettings && selectedFeed" class="modal-backdrop feed-detail-backdrop" @click.self="showFeedSettings = false"><div class="modal feed-settings-modal"><button class="modal-close" @click="showFeedSettings = false">×</button><p class="eyebrow">SOURCE DETAILS</p><h2>{{ selectedFeed.name }}</h2><p>{{ sourceMeta[selectedFeed.source]?.label }} · {{ selectedFeed.handle }}</p><form class="settings-form" @submit.prevent="saveFeedSettings"><label class="switch-row"><span>启用自动同步</span><input v-model="selectedFeed.enabled" type="checkbox"></label><label>执行计划</label><select v-model="selectedFeed.schedule"><option>每 1 小时</option><option>每 6 小时</option><option>每 12 小时</option><option>每天 20:00</option></select><label class="switch-row"><span>首次拉取历史内容</span><input v-model="selectedFeed.includePast" type="checkbox"></label><div v-if="selectedFeed.source === 'bilibili'" class="content-scope"><strong>内容范围</strong><span>图文动态（DRAW）</span><span>专栏（ARTICLE）</span><small>视频及转发视频始终过滤，无法在此开启。</small></div><p v-if="settingsError" class="login-error">{{ settingsError }}</p><button class="login-button" :disabled="settingsBusy">保存来源设置</button></form></div></div>
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
