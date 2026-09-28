<script setup>
import TimelineStats from './TimelineStats.vue'
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
    <TimelineStats :stats="stats" :favorites-only="favoritesOnly" />
  </header>
</template>
