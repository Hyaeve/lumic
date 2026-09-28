<script setup>
defineProps({
  stats: { type: Object, required: true },
  favoritesOnly: Boolean
})
</script>

<template>
  <section class="scoped-timeline-stats" :class="{ 'favorites-only': favoritesOnly }" aria-label="动态统计">
    <div v-if="!favoritesOnly" class="scoped-stat" :aria-label="`全部动态 ${stats.total}`"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="7" width="16" height="14" rx="2"/><path d="M7 3h10M8 12h8M8 16h5"/></svg><strong>{{ stats.total }}</strong></div>
    <div v-if="!favoritesOnly" class="scoped-stat" :aria-label="`今日动态 ${stats.today}`"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg><strong>{{ stats.today }}</strong></div>
    <div class="scoped-stat" :aria-label="`收藏动态 ${stats.favorites}`"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z"/></svg><strong>{{ stats.favorites }}</strong></div>
  </section>
</template>
