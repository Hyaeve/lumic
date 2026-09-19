<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
const props = defineProps({ value: String, withTime: Boolean, hover: Boolean })
const position = ref(null)
const date = computed(() => new Date(props.value))
const parts = computed(() => {
  if (Number.isNaN(date.value.getTime())) return { day: '', clock: '', full: '' }
  const pad = value => String(value).padStart(2, '0')
  const day = `${pad(date.value.getMonth() + 1)}/${pad(date.value.getDate())}`
  const clock = `${pad(date.value.getHours())}:${pad(date.value.getMinutes())}`
  return { day, clock, full: `${date.value.getFullYear()}/${day} ${clock}` }
})
function hide() {
  position.value = null
  window.removeEventListener('scroll', hide, true)
}
function show(event) {
  if (!props.hover) return
  const rect = event.currentTarget.getBoundingClientRect()
  position.value = { left: `${Math.min(window.innerWidth - 90, Math.max(90, rect.left + rect.width / 2))}px`, top: `${rect.bottom + 7}px` }
  window.addEventListener('scroll', hide, { capture: true, passive: true })
}
onBeforeUnmount(hide)
</script>

<template>
  <time class="post-time" :datetime="value" :aria-label="parts.full" :tabindex="hover ? 0 : undefined" @click.stop @mouseenter="show" @mouseleave="hide" @focus="show" @blur="hide">{{ parts.day + (withTime ? ' ' + parts.clock : '') }}</time>
  <Teleport to="body"><span v-if="position" class="post-time-tooltip" role="tooltip" :style="position">{{ parts.full }}</span></Teleport>
</template>

<style>
.post-time { white-space: nowrap; font-variant-numeric: tabular-nums; color: var(--muted, #8b929b); font-size: 13px; line-height: 1.4; transition: color .16s; }
.post-time[tabindex]:hover, .post-time[tabindex]:focus-visible { color: #656ac0; background: linear-gradient(110deg, #506db5, #8462b8, #5d7ccc, #506db5); background-size: 200% 100%; background-clip: text; -webkit-background-clip: text; -webkit-text-fill-color: transparent; animation: time-starlight 2.4s ease-in-out infinite alternate; outline: none; }
.dark .post-time[tabindex]:hover, .dark .post-time[tabindex]:focus-visible { background-image: linear-gradient(110deg, #9ebcff, #c8b7ff, #a5b4ef, #9ebcff); filter: drop-shadow(0 0 5px #8a9fe066); }
@keyframes time-starlight { to { background-position: 100% 50%; } }
@media (prefers-reduced-motion: reduce) { .post-time[tabindex] { animation: none!important; } }
.post-time-tooltip { position: fixed; z-index: 20000; transform: translateX(-50%); padding: 8px 11px; border-radius: 9px; background: #30343bea; color: #fff; box-shadow: 0 4px 18px #0002; font-size: 12px; line-height: 1.4; pointer-events: none; white-space: nowrap; }
</style>
