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
      <div>
        <p class="eyebrow">{{ sourceLabel ? `AUTHOR TIMELINE · ${sourceLabel}` : favoritesOnly ? 'SAVED MOMENTS' : 'TAG TIMELINE' }}</p>
        <h1>{{ title }}</h1>
        <p v-if="favoritesOnly" class="scoped-timeline-description">把喜欢的瞬间，留给慢慢回看的日子。</p>
      </div>
    </div>
    <section class="scoped-timeline-stats" :class="{ 'favorites-only': favoritesOnly }" aria-label="动态统计">
      <div v-if="!favoritesOnly" class="scoped-stat" :aria-label="`全部动态 ${stats.total}`"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="7" width="16" height="14" rx="2"/><path d="M7 3h10M8 12h8M8 16h5"/></svg><strong>{{ stats.total }}</strong></div>
      <div v-if="!favoritesOnly" class="scoped-stat" :aria-label="`今日动态 ${stats.today}`"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg><strong>{{ stats.today }}</strong></div>
      <div class="scoped-stat" :aria-label="`收藏动态 ${stats.favorites}`"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z"/></svg><strong>{{ stats.favorites }}</strong></div>
    </section>
  </header>
</template>
