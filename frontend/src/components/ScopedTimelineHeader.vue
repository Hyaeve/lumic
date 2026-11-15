<script setup>
import { BellPlus, BellRing, RotateCcw } from '@lucide/vue'
import TimelineStats from './TimelineStats.vue'
defineProps({
  title: String,
  avatar: String,
  sourceLabel: String,
  stats: { type: Object, default: () => ({ total: 0, today: 0, favorites: 0 }) },
  favoritesOnly: Boolean,
  subscriptionState: String,
  subscriptionBusy: Boolean,
  preview: Boolean
})
defineEmits(['avatar-load', 'avatar-error', 'subscription-click'])
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
    <div v-if="subscriptionState" class="author-header-actions">
      <TimelineStats :stats="stats" />
      <button
        class="author-subscribe-button"
        type="button"
        :aria-pressed="subscriptionState === 'subscribed'"
        :aria-busy="subscriptionBusy || subscriptionState === 'loading'"
        :aria-label="subscriptionState === 'subscribed' ? '已订阅，取消订阅' : subscriptionState === 'unavailable' ? '无法确认作者账号' : undefined"
        :tabindex="preview ? -1 : undefined"
        :disabled="!preview && (subscriptionBusy || subscriptionState === 'loading' || subscriptionState === 'unavailable')"
        @click.stop="!preview && $emit('subscription-click')"
      >
        <BellRing v-if="subscriptionState === 'subscribed'" aria-hidden="true" />
        <RotateCcw v-else-if="subscriptionState === 'error'" aria-hidden="true" />
        <BellPlus v-else aria-hidden="true" />
        <span>{{ subscriptionState === 'subscribed' ? '已订阅' : subscriptionState === 'error' ? '重试' : subscriptionState === 'loading' ? '查询中' : '订阅' }}</span>
      </button>
    </div>
    <TimelineStats v-else :stats="stats" :favorites-only="favoritesOnly" />
  </header>
</template>
