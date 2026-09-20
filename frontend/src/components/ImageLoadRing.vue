<script setup>
import { RotateCw } from '@lucide/vue'
defineProps({ failed: Boolean })
defineEmits(['retry'])
</script>

<template>
  <button v-if="failed" class="image-load-retry" type="button" aria-label="重试加载原图" @pointerdown.stop @pointerup.stop @click.stop="$emit('retry', $event)"><RotateCw :size="21" /></button>
  <span v-else class="image-load-ring" role="status" aria-label="正在加载原图"><span></span></span>
</template>

<style>
.image-load-ring, .image-load-retry { position: absolute; left: 50%; top: 50%; z-index: 5; width: 44px; height: 44px; margin: -22px 0 0 -22px; display: grid; place-items: center; border: 0; border-radius: 50%; color: #fff; background: #12162266; box-shadow: 0 2px 14px #0002; }
.image-load-ring { pointer-events: none; }
.image-load-ring > span { width: 26px; height: 26px; border: 2px solid #ffffff40; border-top-color: #fff; border-right-color: #c3c9ff; border-radius: 50%; animation: image-ring-spin .85s linear infinite; }
@keyframes image-ring-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .image-load-ring > span { animation-duration: 2s; } }
</style>
