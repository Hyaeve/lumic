<script setup>
import { useId } from 'vue'
import { Heart } from '@lucide/vue'
defineProps({ liked: Boolean })
const gradientId = `favorite-${useId()}`
</script>

<template>
  <Heart :size="17" :fill="liked ? 'currentColor' : 'none'" :class="{ 'favorite-starlight': liked }" :style="{ '--favorite-paint': `url(#${gradientId})` }" aria-hidden="true">
    <defs v-if="liked">
      <linearGradient :id="gradientId" x1="0" y1="1" x2="1" y2="0">
        <stop class="favorite-star-stop star-start" offset="0%" />
        <stop class="favorite-star-stop star-middle" offset="50%" />
        <stop class="favorite-star-stop star-end" offset="100%" />
      </linearGradient>
    </defs>
  </Heart>
</template>

<style>
.dark .favorite-starlight { fill: var(--favorite-paint); stroke: var(--favorite-paint); filter: drop-shadow(0 0 3px #9c8cee55); }
.favorite-star-stop { stop-color: #8cbaff; }
.favorite-star-stop.star-middle { stop-color: #c5a4ff; }
.favorite-star-stop.star-end { stop-color: #e0d8ff; }
.dark .favorite-star-stop { animation: favorite-star-flow 4.8s ease-in-out infinite; }
.dark .favorite-star-stop.star-middle { animation-delay: -1.6s; }
.dark .favorite-star-stop.star-end { animation-delay: -3.2s; }
@keyframes favorite-star-flow {
  0%, 100% { stop-color: #8cbaff; }
  33% { stop-color: #bf99fa; }
  66% { stop-color: #e0d8ff; }
}
@media (prefers-reduced-motion: reduce) {
  .dark .favorite-star-stop { animation: none; }
}
</style>
