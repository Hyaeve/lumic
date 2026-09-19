<script setup>
defineProps({
  title: String,
  avatar: String,
  sourceLabel: String,
  stats: { type: Object, default: () => ({ total: 0, today: 0, favorites: 0 }) },
  favoritesOnly: Boolean
})
defineEmits(['avatar-load', 'avatar-error'])
</script>

<template>
  <header class="topbar scoped-gallery-header scoped-timeline-header" :class="{ 'author-page-header': avatar }">
    <div class="author-profile-main scoped-timeline-identity">
      <img v-if="avatar" :key="avatar" :src="avatar" :alt="title" data-fallback-index="0" referrerpolicy="no-referrer" @load="$emit('avatar-load', $event)" @error="$emit('avatar-error', $event)">
      <div><p v-if="sourceLabel" class="eyebrow">{{ sourceLabel }}</p><h1>{{ title }}</h1></div>
    </div>
    <section class="scoped-timeline-stats" :class="{ 'favorites-only': favoritesOnly }" aria-label="动态统计">
      <div v-if="!favoritesOnly" class="stat-card"><span>全部动态</span><strong>{{ stats.total }}</strong></div>
      <div v-if="!favoritesOnly" class="stat-card"><span>今日动态</span><strong>{{ stats.today }}</strong></div>
      <div class="stat-card"><span>收藏动态</span><strong>{{ stats.favorites }}</strong></div>
    </section>
  </header>
</template>
