<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import ImageLoadRing from './ImageLoadRing.vue'

const props = defineProps({ source: { type: String, required: true }, active: { type: Boolean, default: true }, imageWidth: Number, imageHeight: Number })
const frame = ref(null)
const frameSize = ref({ width: 0, height: 0 })
let observer
const videoStyle = computed(() => {
  if (!props.imageWidth || !props.imageHeight || !frameSize.value.width) return {}
  const scale = Math.min(frameSize.value.width / props.imageWidth, frameSize.value.height / props.imageHeight)
  return { width: `${props.imageWidth * scale}px`, height: `${props.imageHeight * scale}px` }
})
const video = ref(null)
const playing = ref(false)
const loading = ref(false)
const failed = ref(false)
let generation = 0

function stop() {
  generation++
  playing.value = false
  loading.value = false
  video.value?.pause()
}
async function replay() {
  const element = video.value
  if (!element || !props.active) return
  const request = ++generation
  const retry = failed.value
  failed.value = false
  loading.value = true
  try {
    if (retry) element.load()
    element.currentTime = 0
    element.muted = true
    await element.play()
    if (request !== generation || !props.active) return
    loading.value = false
  } catch {
    if (request !== generation) return
    loading.value = false
    playing.value = false
    failed.value = true
  }
}
function finish() {
  stop()
}
function showFrame() {
  if (!props.active) return
  playing.value = true
  loading.value = false
}
function fail() {
  stop()
  failed.value = true
}
function handleVisibility() { if (document.hidden) stop() }
function autoplay() {
  if (!matchMedia('(prefers-reduced-motion: reduce)').matches) replay()
}
watch(() => props.active, active => { if (active) autoplay(); else stop() })
onMounted(() => {
  observer = new ResizeObserver(() => { frameSize.value = { width: frame.value.clientWidth, height: frame.value.clientHeight } })
  observer.observe(frame.value)
  autoplay()
  document.addEventListener('visibilitychange', handleVisibility)
})
onBeforeUnmount(() => { stop(); observer?.disconnect(); document.removeEventListener('visibilitychange', handleVisibility) })
defineExpose({ replay, stop })
</script>

<template>
  <div ref="frame" class="live-photo-playback" :class="{ playing }" :data-state="failed ? 'failed' : loading ? 'loading' : playing ? 'playing' : 'still'">
    <video ref="video" :src="source" :style="videoStyle" muted playsinline webkit-playsinline preload="metadata" disablepictureinpicture disableremoteplayback aria-hidden="true" @playing="showFrame" @ended="finish" @error="fail" @waiting="loading = true" />
    <ImageLoadRing v-if="loading || failed" :failed="failed" loading-label="正在加载实况照片" retry-label="重新播放实况照片" @retry="replay" />
  </div>
</template>

<style scoped>
.live-photo-playback { position: absolute; inset: 0; z-index: 2; pointer-events: none; }
.live-photo-playback video { display: block; position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%); width: 100%; height: 100%; object-fit: cover; object-position: center; opacity: 0; transition: opacity .12s ease; }
.live-photo-playback.playing video { opacity: 1; }
.live-photo-playback :deep(.image-load-retry) { pointer-events: auto; }
@media (prefers-reduced-motion: reduce) { .live-photo-playback video { transition: none; } }
</style>
