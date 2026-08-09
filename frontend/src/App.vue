<script setup>
import { computed, onMounted, ref } from 'vue'

const authenticated = ref(false)
const loginError = ref('')
const loginBusy = ref(false)
const credentials = ref({ username: '', password: '' })
const activeNav = ref('all')
const activeSource = ref('all')
const isDark = ref(false)
const showAdd = ref(false)
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
    const [postResponse, feedResponse] = await Promise.all([fetch('/api/posts'), fetch('/api/feeds')])
    if (!postResponse.ok || !feedResponse.ok) throw new Error('api unavailable')
    posts.value = await postResponse.json()
    feeds.value = await feedResponse.json()
  } catch { posts.value = fallbackPosts; feeds.value = [] }
}
async function syncNow() {
  syncing.value = true
  try { await fetch('/api/sync', { method: 'POST' }) } catch {}
  setTimeout(() => { syncing.value = false }, 900)
}
async function checkSession() {
  try {
    const response = await fetch('/api/posts')
    if (!response.ok) throw new Error('unauthorized')
    posts.value = await response.json()
    authenticated.value = true
    const feedResponse = await fetch('/api/feeds')
    if (feedResponse.ok) feeds.value = await feedResponse.json()
  } catch { authenticated.value = false }
}
onMounted(checkSession)
</script>

<template>
  <div v-if="!authenticated" class="login-shell">
    <div class="login-panel">
      <div class="login-brand"><span class="brand-mark">✦</span><strong>Lumic</strong><small>拾光</small></div>
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
      <div class="brand"><span class="brand-mark">✦</span><span>Lumic</span><small>拾光</small></div>
      <nav class="main-nav">
        <button :class="{ active: activeNav === 'all' }" @click="activeNav = 'all'; activeSource = 'all'"><span>⌂</span> 全部动态 <b>{{ posts.length }}</b></button>
        <button :class="{ active: activeNav === 'liked' }" @click="activeNav = 'liked'; activeSource = 'weibo'"><span>♡</span> 我的点赞 <b>24</b></button>
        <div class="nav-label">来源</div>
        <button v-for="(meta, key) in sourceMeta" :key="key" :class="{ active: activeSource === key }" @click="activeNav = 'source'; activeSource = key"><i :class="['source-icon', meta.icon]">{{ meta.icon === 'wb' ? '微' : meta.icon === 'px' ? 'P' : '哔' }}</i>{{ meta.label }} <b>{{ posts.filter(p => p.source === key).length }}</b></button>
      </nav>
      <div class="sidebar-bottom">
        <button><span>⚙</span> 设置</button>
        <div class="profile"><div class="profile-avatar">拾</div><div><strong>我的空间</strong><small>本地收藏</small></div><span>⋮</span></div>
      </div>
    </aside>

    <main class="content">
      <header class="topbar"><div><p class="eyebrow">SAVED MOMENTS · {{ new Date().toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' }) }}</p><h1>早上好，拾光者 <span>☼</span></h1><p class="subtitle">这里有你关注的世界，和刚刚发生的一切。</p></div><div class="header-actions"><button class="icon-button" @click="isDark = !isDark" :title="isDark ? '切换日间主题' : '切换夜间主题'">{{ isDark ? '☀' : '☾' }}</button><button class="sync-button" @click="syncNow"><span :class="{ spin: syncing }">↻</span> {{ syncing ? '同步中' : '立即同步' }}</button></div></header>
      <section class="stats"><div class="stat-card"><div class="stat-icon mint">✦</div><div><span>今日新动态</span><strong>{{ posts.length }} <em>+12%</em></strong></div><small>较昨日</small></div><div class="stat-card"><div class="stat-icon rose">♡</div><div><span>已收藏动态</span><strong>128</strong></div><small>持续增长中</small></div><div class="stat-card"><div class="stat-icon sand">◷</div><div><span>关注来源</span><strong>{{ sourceCount }} <em class="neutral">个</em></strong></div><small>同步正常</small></div></section>
      <div class="section-heading"><div><h2>最新动态</h2><p>按时间顺序排列 · 自动同步于 10 分钟前</p></div><div class="filters"><button v-for="(meta, key) in { all: { label: '全部', icon: '✦' }, ...sourceMeta }" :key="key" :class="{ selected: activeSource === key }" @click="activeSource = key"><span v-if="key === 'all'">✦</span><i v-else :class="['source-icon', meta.icon]">{{ meta.icon === 'wb' ? '微' : meta.icon === 'px' ? 'P' : '哔' }}</i>{{ meta.label }}</button><button class="view-button">▦</button></div></div>
      <section class="feed-list"><article v-for="post in filteredPosts" :key="post.id" class="post-card"><div class="post-head"><img :src="post.avatar" :alt="post.author"><div class="author"><strong>{{ post.author }}</strong><span>{{ relativeTime(post.published) }}</span></div><span :class="['source-pill', sourceMeta[post.source].color]"><i :class="['source-icon', sourceMeta[post.source].icon]">{{ sourceMeta[post.source].icon === 'wb' ? '微' : sourceMeta[post.source].icon === 'px' ? 'P' : '哔' }}</i>{{ sourceMeta[post.source].label }}</span><button class="more">···</button></div><p class="caption">{{ post.caption }}</p><div v-if="post.media?.length" class="media-grid"><img v-for="media in post.media" :key="media" :src="media" alt="动态图片"></div><div class="tag-row"><span v-for="tag in post.tags" :key="tag"># {{ tag }}</span></div><div class="post-foot"><span>◷ {{ new Date(post.published).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }}</span><div><button>♡</button><button>↗</button><button>⌑</button></div></div></article><div v-if="!filteredPosts.length" class="empty">还没有这个来源的动态</div></section>
    </main>
    <button class="add-fab" @click="showAdd = true">＋ <span>添加来源</span></button>
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false"><div class="modal"><button class="modal-close" @click="showAdd = false">×</button><p class="eyebrow">NEW SOURCE</p><h2>添加一个新来源</h2><p>连接账号后，Lumic 会帮你把喜欢的内容收进同一条时间线。</p><div class="connect-options"><button @click="showAdd = false"><i class="source-icon wb">微</i>连接微博</button><button @click="showAdd = false"><i class="source-icon px">P</i>连接 pixiv</button><button @click="showAdd = false"><i class="source-icon bl">哔</i>连接哔哩哔哩</button></div></div></div>
  </div>
</template>
