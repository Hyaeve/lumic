<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import QRCode from 'qrcode'

const authenticated = ref(false)
const loginError = ref('')
const showSettings = ref(false)
const settingsTab = ref('network')
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
const weiboQR = ref(null)
const weiboBusy = ref(false)
const weiboError = ref('')
let weiboPollTimer = null
const syncing = ref(false)
const posts = ref([])
const feeds = ref([])

const fallbackPosts = [
  { id: 'wb-001', source: 'weibo', author: '林间拾光', avatar: 'https://i.pravatar.cc/96?img=47', caption: '把黄昏收藏进今天的相册里。风经过树梢，带来一点夏天的回声。', tags: ['日常', '摄影'], published: new Date(Date.now() - 42 * 60000), liked: true },
  { id: 'px-002', source: 'pixiv', author: 'Aoi Sora', avatar: 'https://i.pravatar.cc/96?img=32', caption: '新作｜雨后的玻璃温室，想画出潮湿空气里柔软的光。', tags: ['原创', '插画', '光影'], published: new Date(Date.now() - 2 * 3600000), media: ['https://images.unsplash.com/photo-1518709268805-4e9042af9f23?w=900&q=80'] },
  { id: 'bl-003', source: 'bilibili', author: '慢慢生活研究所', avatar: 'https://i.pravatar.cc/96?img=12', caption: '一期关于城市散步的记录，和你分享最近发现的三家小店。', tags: ['VLOG', '城市漫游'], published: new Date(Date.now() - 5 * 3600000) },
  { id: 'wb-004', source: 'weibo', author: '山茶花开时', avatar: 'https://i.pravatar.cc/96?img=5', caption: '今日份蓝色时刻。愿我们都能留住一些不被打扰的浪漫。', tags: ['随手拍', '生活'], published: new Date(Date.now() - 8 * 3600000), liked: true },
  { id: 'px-005', source: 'pixiv', author: 'Mori', avatar: 'https://i.pravatar.cc/96?img=20', caption: '夏日习作 #sketch #summer', tags: ['sketch', 'summer'], published: new Date(Date.now() - 24 * 3600000) }
]

const sourceMeta = {
  weibo: { label: '微博', icon: 'wb', color: 'coral' },
  pixiv: { label: 'pixiv', icon: 'px', color: 'violet' },
  bilibili: { label: '哔哩哔哩', icon: 'bl', color: 'blue' }
}
const filteredPosts = computed(() => activeSource.value === 'all' ? posts.value : posts.value.filter(p => p.source === activeSource.value))
const sourceCount = computed(() => new Set(posts.value.map(p => p.source)).size)
const platformCards = computed(() => [
  { key: 'bilibili', label: '哔哩哔哩', short: '哔', icon: 'bl', configured: biliAccount.value.configured, account: biliAccount.value.configured ? `UID ${biliAccount.value.userId}` : '尚未连接账号', path: '/flow/bilibili', description: 'UP 主图文动态与专栏', feeds: feeds.value.filter(feed => feed.source === 'bilibili') },
  { key: 'pixiv', label: 'Pixiv', short: 'P', icon: 'px', configured: pixivAccount.value.configured, account: pixivAccount.value.configured ? (pixivAccount.value.userName || `UID ${pixivAccount.value.userId}`) : '尚未连接账号', path: '/flow/pixiv', description: '画师作品与插画媒体', feeds: feeds.value.filter(feed => feed.source === 'pixiv') },
  { key: 'weibo', label: '微博', short: '微', icon: 'wb', configured: weiboAccount.value.configured, account: weiboAccount.value.configured ? (weiboAccount.value.userName || `UID ${weiboAccount.value.userId}`) : '尚未连接账号', path: '/flow/weibo', description: '博主动态与图文媒体', feeds: feeds.value.filter(feed => feed.source === 'weibo') }
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
    const [postResponse, feedResponse, biliFeedResponse] = await Promise.all([fetch('/api/posts'), fetch('/api/feeds'), fetch('/api/bilibili/subscriptions')])
    if (!postResponse.ok || !feedResponse.ok || !biliFeedResponse.ok) throw new Error('api unavailable')
    posts.value = await postResponse.json()
    feeds.value = [...await feedResponse.json(), ...await biliFeedResponse.json()]
  } catch { posts.value = fallbackPosts; feeds.value = [] }
}
async function syncNow() {
  syncing.value = true
  try { await fetch('/api/sync', { method: 'POST' }) } catch {}
  setTimeout(() => { syncing.value = false }, 900)
}
async function openSettings(tab = 'network') {
  settingsTab.value = tab; showSettings.value = true; activeNav.value = 'settings'; settingsError.value = ''; proxyMessage.value = ''; pixivError.value = ''; weiboError.value = ''
  try {
    const [projectResponse, biliResponse, pixivResponse, weiboResponse] = await Promise.all([fetch('/api/project/settings'), fetch('/api/bilibili/account'), fetch('/api/pixiv/account'), fetch('/api/weibo/qr')])
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
async function searchBilibili() {
  if (!biliKeyword.value.trim()) return
  biliBusy.value = true; biliError.value = ''; biliResults.value = []
  try {
    const response = await fetch(`/api/bilibili/search?keyword=${encodeURIComponent(biliKeyword.value.trim())}`)
    if (!response.ok) throw new Error(response.status === 412 ? '请先配置 B 站凭证' : '搜索暂时不可用，请稍后重试')
    biliResults.value = await response.json()
  } catch (error) { biliError.value = error.message } finally { biliBusy.value = false }
}
async function subscribeBilibili(user) {
  biliBusy.value = true; biliError.value = ''
  try {
    const response = await fetch('/api/bilibili/subscriptions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId: user.userId, name: user.name, includePast: biliIncludePast.value, schedule: '每 6 小时' }) })
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
    if (selectedFeed.value.source === 'bilibili' && selectedFeed.value.id.startsWith('bili-')) {
      const response = await fetch('/api/bilibili/subscriptions', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(selectedFeed.value) })
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
  return feed.source === 'bilibili' && feed.id.startsWith('bili-') ? '/api/bilibili/subscriptions' : '/api/feeds'
}
async function syncSource(feed) {
  sourceActionBusy.value = `sync:${feed.id}`; sourceActionMessage.value = ''; settingsError.value = ''
  const endpoint = sourceOperationEndpoint(feed)
  try {
    const response = await fetch(`${endpoint}?action=sync&id=${encodeURIComponent(feed.id)}`, { method: 'POST' })
    if (!response.ok) throw new Error(await responseError(response, '无法启动来源拉取'))
    const result = await response.json(); sourceActionMessage.value = result.message || '拉取任务已加入队列'
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
function handleConfirmKeydown(event) {
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
function closeSettingsPage() { showSettings.value = false; selectedPlatform.value = null; activeNav.value = activeSource.value === 'all' ? 'all' : 'source' }
function formatFans(count) { return count >= 10000 ? `${(count / 10000).toFixed(1)}万` : count }
async function checkSession() {
  try {
    const response = await fetch('/api/posts')
    if (!response.ok) throw new Error('unauthorized')
    posts.value = await response.json()
    authenticated.value = true
    const [feedResponse, biliFeedResponse] = await Promise.all([fetch('/api/feeds'), fetch('/api/bilibili/subscriptions')])
    if (feedResponse.ok && biliFeedResponse.ok) feeds.value = [...await feedResponse.json(), ...await biliFeedResponse.json()]
  } catch { authenticated.value = false }
}
onMounted(() => { checkSession(); window.addEventListener('keydown', handleConfirmKeydown) })
onUnmounted(() => { stopWeiboPolling(); stopBilibiliPolling(); window.removeEventListener('keydown', handleConfirmKeydown); if (confirmResolver) closeConfirmDialog(false) })
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
        <button :class="{ active: activeNav === 'all' }" @click="activeNav = 'all'; activeSource = 'all'">
<span>⌂</span> 全部动态 <b>{{ posts.length }}</b>
</button>
        <button :class="{ active: activeNav === 'liked' }" @click="activeNav = 'liked'; activeSource = 'weibo'">
<span>♡</span> 我的点赞 <b>24</b>
</button>
        <div class="nav-label">来源</div>
        <button v-for="(meta, key) in sourceMeta" :key="key" :class="{ active: activeSource === key }" @click="activeNav = 'source'; activeSource = key">
<i :class="['source-icon', meta.icon]">{{ meta.icon === 'wb' ? '微' : meta.icon === 'px' ? 'P' : '哔' }}</i>{{ meta.label }} <b>{{ posts.filter(p => p.source === key).length }}</b>
</button>
      </nav>
      <div class="sidebar-bottom">
        <button :class="{ active: activeNav === 'settings' }" @click="openSettings()">
<span>⚙</span> 设置</button>
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

    <main v-if="!showSettings" class="content">
      <header class="topbar">
<div>
<p class="eyebrow">SAVED MOMENTS · {{ new Date().toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' }) }}</p>
<h1>早上好，拾光者 <span>☼</span>
</h1>
<p class="subtitle">这里有你关注的世界，和刚刚发生的一切。</p>
</div>
<div class="header-actions">
<button class="icon-button" @click="isDark = !isDark" :title="isDark ? '切换日间主题' : '切换夜间主题'">{{ isDark ? '☀' : '☾' }}</button>
<button class="sync-button" @click="syncNow">
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
<span>已收藏动态</span>
<strong>128</strong>
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
<h2>最新动态</h2>
<p>按时间顺序排列 · 自动同步于 10 分钟前</p>
</div>
<div class="filters">
<button v-for="(meta, key) in { all: { label: '全部', icon: '✦' }, ...sourceMeta }" :key="key" :class="{ selected: activeSource === key }" @click="activeSource = key">
<span v-if="key === 'all'">✦</span>
<i v-else :class="['source-icon', meta.icon]">{{ meta.icon === 'wb' ? '微' : meta.icon === 'px' ? 'P' : '哔' }}</i>{{ meta.label }}</button>
<button class="view-button">▦</button>
</div>
</div>
      <section class="feed-list">
<article v-for="post in filteredPosts" :key="post.id" class="post-card">
<div class="post-head">
<img :src="post.avatar" :alt="post.author">
<div class="author">
<strong>{{ post.author }}</strong>
<span>{{ relativeTime(post.published) }}</span>
</div>
<span :class="['source-pill', sourceMeta[post.source].color]">
<i :class="['source-icon', sourceMeta[post.source].icon]">{{ sourceMeta[post.source].icon === 'wb' ? '微' : sourceMeta[post.source].icon === 'px' ? 'P' : '哔' }}</i>{{ sourceMeta[post.source].label }}</span>
<button class="more">···</button>
</div>
<p class="caption">{{ post.caption }}</p>
<div v-if="post.media?.length" class="media-grid">
<img v-for="media in post.media" :key="media" :src="media" alt="动态图片">
</div>
<div class="tag-row">
<span v-for="tag in post.tags" :key="tag"># {{ tag }}</span>
</div>
<div class="post-foot">
<span>◷ {{ new Date(post.published).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }}</span>
<div>
<button>♡</button>
<button>↗</button>
<button>⌑</button>
</div>
</div>
</article>
<div v-if="!filteredPosts.length" class="empty">还没有这个来源的动态</div>
</section>
    </main>
    <button v-if="!showSettings" class="add-fab" @click="showAdd = true">＋ <span>添加来源</span>
</button>
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
<div class="modal">
<button class="modal-close" @click="showAdd = false">×</button>
<p class="eyebrow">NEW SOURCE</p>
<h2>添加一个新来源</h2>
<p>连接账号后，Lumic 会帮你把喜欢的内容收进同一条时间线。</p>
<div class="connect-options">
<button @click="showAdd = false; openSettings('platforms'); startWeiboQR()">
<i class="source-icon wb">微</i>连接微博</button>
<button @click="showAdd = false; openSettings('platforms')">
<i class="source-icon px">P</i>连接 pixiv</button>
<button @click="openBilibili">
<i class="source-icon bl">哔</i>连接哔哩哔哩</button>
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
    <main v-if="showSettings" class="settings-page">
      <div class="settings-page-inner">
        <button class="settings-back" @click="closeSettingsPage">← 返回动态</button>
        <p class="eyebrow">LUMIC SETTINGS</p>
        <h2>设置</h2>
        <div class="settings-tabs">
          <button :class="{ active: settingsTab === 'network' }" @click="settingsTab = 'network'; settingsError = ''">网络代理</button>
          <button :class="{ active: settingsTab === 'platforms' }" @click="settingsTab = 'platforms'; settingsError = ''">平台凭证</button>
          <button :class="{ active: settingsTab === 'sources' }" @click="settingsTab = 'sources'; settingsError = ''">来源管理</button>
          <button :class="{ active: settingsTab === 'security' }" @click="settingsTab = 'security'; settingsError = ''">登录安全</button>
        </div>
        <section v-if="settingsTab === 'network'" class="settings-pane">
          <h3>项目代理</h3><p>用于 Pixiv、B 站等所有后端外部请求。支持 HTTP、HTTPS、SOCKS5。</p>
          <div v-if="proxyStatus.proxyEnabled" class="setting-status">当前代理：{{ proxyStatus.proxyUrl }}</div>
          <form class="settings-form" @submit.prevent="saveProxy" autocomplete="off"><label>代理地址</label><input v-model="proxyForm.proxyUrl" placeholder="socks5://host.docker.internal:7890"><p class="credential-note">Docker 中的 127.0.0.1 指向容器自身；访问宿主机代理请使用 host.docker.internal。</p><div class="form-actions"><button type="button" class="secondary-button" @click="testProxy" :disabled="settingsBusy">测试连接</button><button class="login-button" :disabled="settingsBusy">保存代理</button><button type="button" class="danger-link" @click="proxyForm.proxyUrl = ''; saveProxy()">关闭代理</button></div></form>
          <p v-if="proxyMessage" class="success-message">{{ proxyMessage }}</p>
        </section>
        <section v-if="settingsTab === 'platforms'" class="settings-pane">
          <div class="platform-auth-grid">
            <article class="platform-auth-card"><h3>Pixiv</h3><p v-if="pixivAccount.configured">已连接 {{ pixivAccount.userName || pixivAccount.userId }}，重新保存时需填写新 token。</p><p v-else>使用 OAuth refresh_token 连接，不保存账号密码。</p><form class="settings-form" @submit.prevent="savePixivAccount"><label>refresh_token</label><input v-model="pixivRefreshToken" type="password" required autocomplete="off"><p class="credential-note">服务端需配置 Pixiv OAuth 客户端环境变量，token 将加密保存。</p><button class="login-button" :disabled="pixivBusy">{{ pixivBusy ? '验证中…' : '验证并保存 Pixiv' }}</button></form><p v-if="pixivError" class="login-error">{{ pixivError }}</p></article>
            <article class="platform-auth-card"><h3>微博</h3><p v-if="weiboAccount.configured">已连接 {{ weiboAccount.userName || `UID ${weiboAccount.userId}` }}。</p><p v-else>使用微博客户端扫描二维码登录。</p><div v-if="weiboQR" class="weibo-qr"><img :src="weiboQR.image.startsWith('//') ? `https:${weiboQR.image}` : weiboQR.image" alt="微博登录二维码"><span>请在二维码过期前扫码并确认</span></div><button class="login-button" type="button" @click="startWeiboQR" :disabled="weiboBusy && !weiboQR">{{ weiboQR ? '刷新二维码' : weiboBusy ? '获取中…' : '扫码连接微博' }}</button><p v-if="weiboError" class="login-error">{{ weiboError }}</p></article>
          </div>
          <hr class="platform-divider">
          <h3>哔哩哔哩扫码登录</h3><p v-if="biliAccount.configured">已连接 UID {{ biliAccount.userId }}。扫码可切换账号。</p><p v-else>使用哔哩哔哩手机客户端扫码，登录凭证将自动获取并加密保存。</p>
          <div v-if="biliQRImage" class="weibo-qr"><img :src="biliQRImage" alt="哔哩哔哩登录二维码"><span>{{ biliQRStatus }}</span></div><button class="login-button platform-login-button" type="button" @click="startBilibiliQR" :disabled="biliBusy && !biliQR">{{ biliQR ? '刷新二维码' : biliBusy ? '获取中…' : biliAccount.configured ? '扫码切换 B 站账号' : '扫码连接 B 站' }}</button>
          <details class="manual-credential"><summary>高级：手动导入 Cookie</summary><form class="settings-form bili-credentials" @submit.prevent="saveBilibiliAccount" autocomplete="off"><label>完整 Cookie</label><textarea v-model="biliCredentials.cookie" rows="4" placeholder="仅在扫码不可用时使用"></textarea><label>SESSDATA</label><input v-model="biliCredentials.SESSDATA" type="password"><label>bili_jct</label><input v-model="biliCredentials.bili_jct" type="password"><label>buvid3</label><input v-model="biliCredentials.buvid3" type="password"><label>DedeUserID</label><input v-model="biliCredentials.DedeUserID" inputmode="numeric"><button class="login-button" :disabled="biliBusy">验证并保存手动凭证</button></form></details>
          <p v-if="biliError" class="login-error bili-error">{{ biliError }}</p>
        </section>
        <section v-if="settingsTab === 'sources'" class="settings-pane source-platform-pane">
          <div class="pane-heading"><div><h3>来源管理</h3><p>一个平台一个来源卡片。右键卡片打开详细设置，触屏设备可点击按钮。</p></div><span>3 个平台</span></div>
          <div class="platform-source-grid">
            <article v-for="platform in platformCards" :key="platform.key" :class="['platform-source-card', platform.key]" @contextmenu.prevent="openPlatformSettings(platform)">
              <div class="platform-card-head"><i :class="['source-icon', platform.icon]">{{ platform.short }}</i><span :class="['connection-dot', { online: platform.configured }]">{{ platform.configured ? '已连接' : '未连接' }}</span></div>
              <h4>{{ platform.label }}</h4><p>{{ platform.description }}</p>
              <dl><div><dt>账号</dt><dd>{{ platform.account }}</dd></div><div><dt>已配置来源</dt><dd>{{ platform.feeds.length }} 个</dd></div><div><dt>内容目录</dt><dd>{{ platform.path }}</dd></div></dl>
              <button @click="openPlatformSettings(platform)">详细设置 <span>→</span></button>
            </article>
          </div>
          <p class="source-context-tip">提示：也可以在任意平台卡片上点击鼠标右键。</p>
        </section>
        <section v-if="settingsTab === 'security'" class="settings-pane"><h3>登录安全</h3><p>修改后请使用新的账号和密码登录。密码至少 8 位。</p><form class="settings-form" @submit.prevent="saveSettings" autocomplete="off"><label>新账号</label><input v-model="settingsForm.username" required minlength="3" autocomplete="off"><label>当前密码</label><input v-model="settingsForm.currentPassword" type="password" required autocomplete="current-password"><label>新密码</label><input v-model="settingsForm.newPassword" type="password" required minlength="8" autocomplete="new-password"><button class="login-button" type="submit" :disabled="settingsBusy">{{ settingsBusy ? '保存中…' : '保存设置' }}</button></form></section>
        <p v-if="settingsError" class="login-error settings-page-error">{{ settingsError }}</p>
      </div>
    </main>
    <div v-if="selectedPlatform" class="modal-backdrop platform-detail-backdrop" @click.self="selectedPlatform = null"><div class="modal platform-detail-modal"><button class="modal-close" @click="selectedPlatform = null">×</button><div class="platform-detail-title"><i :class="['source-icon', selectedPlatform.icon]">{{ selectedPlatform.short }}</i><div><p class="eyebrow">PLATFORM SOURCE</p><h2>{{ selectedPlatform.label }}</h2></div><span :class="['connection-dot', { online: selectedPlatform.configured }]">{{ selectedPlatform.configured ? '已连接' : '未连接' }}</span></div><div class="platform-detail-summary"><div><span>当前账号</span><strong>{{ selectedPlatform.account }}</strong></div><div><span>内容目录</span><strong>{{ selectedPlatform.path }}</strong></div><div><span>作者来源</span><strong>{{ selectedPlatform.feeds.length }} 个</strong></div></div><div class="platform-detail-actions"><button class="secondary-button" @click="managePlatformCredentials(selectedPlatform.key)">{{ selectedPlatform.configured ? '管理账号凭证' : '连接平台账号' }}</button><button v-if="selectedPlatform.key === 'bilibili' && selectedPlatform.configured" class="login-button" @click="selectedPlatform = null; showSettings = false; openBilibili()">添加 UP 主</button></div><p v-if="sourceActionMessage" class="success-message source-action-message">{{ sourceActionMessage }}</p><p v-if="settingsError" class="login-error">{{ settingsError }}</p><div class="configured-source-list"><div class="configured-source-heading"><h3>已配置作者</h3><span>{{ selectedPlatform.feeds.length }} 个</span></div><article v-for="feed in selectedPlatform.feeds" :key="feed.id"><i :class="['source-icon', selectedPlatform.icon]">{{ selectedPlatform.short }}</i><div><strong>{{ feed.name }}</strong><span>{{ feed.handle }} · {{ feed.schedule }}</span><small>{{ feed.storagePath || `${selectedPlatform.path}/${feed.name}` }}</small></div><em :class="{ disabled: !feed.enabled }">{{ feed.enabled ? '同步中' : '已停用' }}</em><div class="source-row-actions"><button @click="syncSource(feed)" :disabled="sourceActionBusy === `sync:${feed.id}`">{{ sourceActionBusy === `sync:${feed.id}` ? '拉取中…' : '立即拉取' }}</button><button @click="openFeedSettings(feed)">设置</button><button class="delete-source-button" @click="deleteSource(feed)" :disabled="sourceActionBusy === `delete:${feed.id}`">{{ sourceActionBusy === `delete:${feed.id}` ? '删除中…' : '删除' }}</button></div></article><div v-if="!selectedPlatform.feeds.length" class="platform-empty"><span>＋</span><strong>还没有作者来源</strong><p>{{ selectedPlatform.key === 'bilibili' ? '点击“添加 UP 主”开始订阅图文与专栏。' : '作者订阅连接器将在后续版本开放。' }}</p></div></div></div></div>
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
