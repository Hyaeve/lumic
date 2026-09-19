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
.post-time { white-space: nowrap; font-variant-numeric: tabular-nums; color: var(--muted, #8b929b); font-size: 11px; line-height: 1.4; transition: color .16s; }
.post-time[tabindex]:hover, .post-time[tabindex]:focus-visible { color: var(--accent, #459d97); outline: none; }
.post-time-tooltip { position: fixed; z-index: 20000; transform: translateX(-50%); padding: 8px 11px; border-radius: 9px; background: #30343bea; color: #fff; box-shadow: 0 4px 18px #0002; font-size: 12px; line-height: 1.4; pointer-events: none; white-space: nowrap; }
</style>
