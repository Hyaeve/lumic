<script setup>
import { useId } from 'vue'
import { Heart } from '@lucide/vue'
defineProps({ liked: Boolean })
const gradientId = `favorite-${useId()}`
</script>

<template>
  <Heart :size="17" :fill="liked ? 'currentColor' : 'none'" :class="{ 'favorite-starlight': liked }" :style="{ '--favorite-paint': `url(#${gradientId}-stars)`, '--favorite-edge': `url(#${gradientId})` }" aria-hidden="true">
    <defs v-if="liked">
      <linearGradient :id="gradientId" x1="0" y1="1" x2="1" y2="0">
        <stop class="favorite-star-stop star-start" offset="0%" />
        <stop class="favorite-star-stop star-middle" offset="50%" />
        <stop class="favorite-star-stop star-end" offset="100%" />
      </linearGradient>
      <pattern :id="`${gradientId}-stars`" width="24" height="24" patternUnits="userSpaceOnUse">
        <rect width="24" height="24" :fill="`url(#${gradientId})`" />
        <circle cx="7" cy="9" r=".65" fill="#dce8ff" />
        <circle cx="16" cy="8" r=".55" fill="#d4bcff" />
        <circle cx="14" cy="16" r=".65" fill="#f7e6a5" />
      </pattern>
    </defs>
  </Heart>
</template>

<style>
.dark .favorite-starlight { fill: var(--favorite-paint); stroke: var(--favorite-edge); filter: drop-shadow(0 0 3px #8799cf66); }
.favorite-star-stop { stop-color: #708ccc; }
.favorite-star-stop.star-middle { stop-color: #8b78ba; }
.favorite-star-stop.star-end { stop-color: #a9a2db; }
.dark .favorite-star-stop { animation: favorite-star-flow 4.8s ease-in-out infinite; }
.dark .favorite-star-stop.star-middle { animation-delay: -1.6s; }
.dark .favorite-star-stop.star-end { animation-delay: -3.2s; }
@keyframes favorite-star-flow {
  0%, 100% { stop-color: #708ccc; }
  33% { stop-color: #8b78ba; }
  66% { stop-color: #a9a2db; }
}
@media (prefers-reduced-motion: reduce) {
  .dark .favorite-star-stop { animation: none; }
}
</style>
