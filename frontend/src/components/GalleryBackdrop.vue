<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
const props = defineProps({ images: { type: Array, default: () => [] }, interactive: Boolean, mobile: Boolean, dark: Boolean })
const emit = defineEmits(['present'])
const layers = ref(['', ''])
const active = ref(0)
let timer
let version = 0
let previous = ''
const anchor = ref(null)
const host = ref(null)
const backdropStyle = ref({})
let observer
const presenting = ref(false)
const expanded = ref(false)
const closing = ref(false)
const presentationStyle = ref({})
const presentationElement = ref(null)
let closeTimer
let previousFocus
let touchStart = null
let settleTimer
const progress = ref(0)
const tracking = ref('')
const progressStyle = computed(() => {
  const initial = parseFloat(backdropStyle.value.height) || 320
  return {
    ...presentationStyle.value,
    height: `${initial + (window.innerHeight - initial) * progress.value}px`,
    opacity: Math.min(1, progress.value * 3),
    '--gallery-image-opacity': .5 + progress.value * .5,
    '--gallery-tint-opacity': 1 - progress.value
  }
})
let lockedUntil = 0
const excludedTarget = target => Boolean(target?.closest('button, a, input, textarea, video, .media-frame, .modal, .lightbox-layer'))

function presentationBounds() {
  const bounds = host.value?.getBoundingClientRect()
  if (!bounds) return
  presentationStyle.value = {
    left: `${props.mobile ? 0 : bounds.left}px`,
    width: `${props.mobile ? window.innerWidth : bounds.width}px`,
    '--gallery-collapsed-height': backdropStyle.value.height || '320px',
    '--gallery-fade-start': backdropStyle.value['--gallery-fade-start']
  }
}
function preparePresentation() {
  if (presenting.value) return true
  if (!props.interactive || !previous || window.scrollY > 2) return false
  previousFocus = document.activeElement
  presentationBounds()
  presenting.value = true
  closing.value = false
  document.documentElement.classList.add('gallery-presenting')
  host.value.inert = true
  progress.value = 0
  return true
}
function settleGesture(cancelled = false) {
  clearTimeout(settleTimer)
  if (!presenting.value || closing.value) return
  tracking.value = ''
  const keepOpen = cancelled ? expanded.value : expanded.value ? progress.value > .4 : progress.value >= .6
  if (!keepOpen) { closePresentation(); return }
  progress.value = 1
  lockedUntil = performance.now() + 650
  if (!expanded.value) {
    expanded.value = true
    emit('present', true)
  }
  void nextTick(() => {
    presentationElement.value?.focus({ preventScroll: true })
  })
}
function closePresentation() {
  if (!presenting.value || closing.value) return
  clearTimeout(settleTimer)
  tracking.value = ''
  progress.value = 0
  closing.value = true
  const wasExpanded = expanded.value
  expanded.value = false
  if (wasExpanded) emit('present', false)
  closeTimer = setTimeout(() => {
    presenting.value = false
    closing.value = false
    document.documentElement.classList.remove('gallery-presenting')
    if (host.value) host.value.inert = false
    previousFocus?.focus?.({ preventScroll: true })
    lockedUntil = performance.now() + 350
  }, matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 650)
}
async function onWheel(event) {
  if (event.ctrlKey || props.mobile || !props.interactive) return
  if (!presenting.value && (window.scrollY > 2 || excludedTarget(event.target) || event.deltaY >= 0 || !previous)) return
  event.preventDefault()
  event.stopPropagation()
  if (performance.now() < lockedUntil || closing.value) return
  const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? window.innerHeight : 1)
  const firstFrame = !presenting.value
  if (!preparePresentation()) return
  tracking.value = 'wheel'
  if (firstFrame) {
    await nextTick()
    if (!presenting.value || closing.value) return
    presentationElement.value?.getBoundingClientRect()
  }
  progress.value = Math.max(0, Math.min(1, progress.value - Math.max(-100, Math.min(100, delta)) / 480))
  clearTimeout(settleTimer)
  settleTimer = setTimeout(() => settleGesture(), 240)
}
function onTouchStart(event) {
  if (event.touches.length !== 1 && tracking.value === 'touch') settleGesture(true)
  touchStart = null
  if (!props.interactive || !props.mobile || event.touches.length !== 1 || closing.value || performance.now() < lockedUntil) return
  if (!presenting.value && (window.scrollY > 2 || excludedTarget(event.target) || !previous)) return
  const point = event.touches[0]
  touchStart = {
    x: point.clientX, y: point.clientY, progress: progress.value,
    time: performance.now(), distance: 0, canTap: expanded.value
  }
}
function onTouchMove(event) {
  if (!touchStart || event.touches.length !== 1) {
    if (tracking.value === 'touch') settleGesture(true)
    touchStart = null
    return
  }
  const point = event.touches[0]
  const dx = point.clientX - touchStart.x
  const dy = point.clientY - touchStart.y
  touchStart.distance = Math.max(touchStart.distance, Math.hypot(dx, dy))
  if (touchStart.distance <= 10 && tracking.value !== 'touch') return
  if (Math.abs(dx) > Math.abs(dy) && Math.abs(dx) > 10) {
    if (tracking.value === 'touch') settleGesture(true)
    touchStart = null
    return
  }
  if ((!presenting.value && dy > 10) || presenting.value) {
    if (event.cancelable) event.preventDefault()
    event.stopPropagation()
    if (!preparePresentation()) return
    tracking.value = 'touch'
    progress.value = Math.max(0, Math.min(1, touchStart.progress + dy / 280))
  }
}
function onTouchEnd(event) {
  if (!touchStart) return
  const gesture = touchStart
  touchStart = null
  const point = event.changedTouches[0]
  const distance = Math.max(gesture.distance, point ? Math.hypot(point.clientX - gesture.x, point.clientY - gesture.y) : 0)
  if (event.type !== 'touchcancel' && event.touches.length === 0 && gesture.canTap &&
      expanded.value && distance <= 10 && performance.now() - gesture.time <= 350) {
    // Consume the synthetic click so it cannot reach the restored timeline.
    if (event.cancelable) event.preventDefault()
    event.stopPropagation()
    closePresentation()
    return
  }
  if (tracking.value === 'touch') {
    event.stopPropagation()
    settleGesture(event.type === 'touchcancel')
  }
}
function onKey(event) {
  if (!presenting.value) return
  if (event.key === 'Escape' || event.key === 'ArrowDown') {
    event.preventDefault()
    event.stopImmediatePropagation()
    closePresentation()
  } else if (event.key === 'Tab') {
    event.preventDefault()
    presentationElement.value?.focus({ preventScroll: true })
  }
}
defineExpose({ close: closePresentation })
onMounted(() => {
  host.value = anchor.value?.closest('.content, .mobile-transition-page-content')
  if (!host.value) return
  const toolbar = host.value.querySelector('.section-heading')
  const measure = () => {
    const bounds = host.value.getBoundingClientRect()
    const bottom = toolbar?.getBoundingClientRect().bottom ?? bounds.top + 200
    const fadeStart = Math.max(120, bottom - bounds.top - 32)
    backdropStyle.value = { height: `${fadeStart + 120}px`, '--gallery-fade-start': `${fadeStart}px` }
  }
  observer = new ResizeObserver(measure)
  observer.observe(host.value)
  if (toolbar) observer.observe(toolbar)
  measure()
  if (props.interactive) {
    host.value.addEventListener('wheel', onWheel, { passive: false, capture: true })
    host.value.addEventListener('touchstart', onTouchStart, { passive: true, capture: true })
    host.value.addEventListener('touchmove', onTouchMove, { passive: false, capture: true })
    host.value.addEventListener('touchend', onTouchEnd, true)
    host.value.addEventListener('touchcancel', onTouchEnd, true)
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('resize', presentationBounds)
  }
})
async function advance(token) {
  const choices = props.images.filter(image => image !== previous)
  const source = previous ? choices[Math.floor(Math.random() * choices.length)] || props.images[0] : props.images[0]
  if (!source) return
  const image = new Image()
  image.src = source
  try { await image.decode() } catch { return }
  if (token !== version) return
  const next = 1 - active.value
  layers.value[next] = source
  active.value = next
  previous = source
}
function scheduleRotation() {
  clearInterval(timer)
  if (props.images.length > 1) {
    const token = version
    timer = setInterval(() => advance(token), expanded.value ? 7000 : 15000)
  }
}
watch(expanded, scheduleRotation)
watch(() => props.images.join('|'), () => {
  clearInterval(timer)
  const token = ++version
  layers.value = ['', '']
  previous = ''
  void advance(token)
  scheduleRotation()
}, { immediate: true })
onBeforeUnmount(() => {
  version++
  clearInterval(timer)
  clearTimeout(closeTimer)
  clearTimeout(settleTimer)
  observer?.disconnect()
  if (!props.interactive) return
  host.value?.removeEventListener('wheel', onWheel, true)
  host.value?.removeEventListener('touchstart', onTouchStart, true)
  host.value?.removeEventListener('touchmove', onTouchMove, true)
  host.value?.removeEventListener('touchend', onTouchEnd, true)
  host.value?.removeEventListener('touchcancel', onTouchEnd, true)
  window.removeEventListener('keydown', onKey, true)
  window.removeEventListener('resize', presentationBounds)
  document.documentElement.classList.remove('gallery-presenting')
  if (host.value) host.value.inert = false
  emit('present', false)
})
</script>

<template>
  <span ref="anchor" class="gallery-backdrop-anchor" aria-hidden="true"></span>
  <Teleport v-if="host" :to="host">
  <div class="gallery-backdrop" :style="backdropStyle" aria-hidden="true">
    <img v-for="(image, index) in layers" :key="index" :src="image || undefined" :class="{ active: image && index === active }" alt="">
    <span></span>
  </div>
  </Teleport>
  <Teleport to="body">
    <section v-if="presenting" ref="presentationElement" class="gallery-presentation" :class="{ expanded, closing, dark, 'tracking-wheel': tracking === 'wheel', 'tracking-touch': tracking === 'touch' }" :style="progressStyle" tabindex="-1" role="dialog" aria-modal="true" aria-label="背景图片幻灯片" @wheel="onWheel" @touchstart.passive="onTouchStart" @touchmove="onTouchMove" @touchend="onTouchEnd" @touchcancel="onTouchEnd">
      <div class="gallery-presentation-images"><img v-for="(image, index) in layers" :key="index" :src="image || undefined" :class="{ active: image && index === active }" alt=""></div>
      <div class="gallery-presentation-tint"></div>
      <div class="gallery-presentation-fade"></div>
    </section>
  </Teleport>
</template>

<style>
.gallery-backdrop-anchor { display: none!important; }
.content:has(.gallery-backdrop), .mobile-transition-page-content:has(.gallery-backdrop) { position: relative; isolation: isolate; }
.gallery-backdrop { position: absolute; top: 0; left: 0; width: 100%; overflow: hidden; pointer-events: none; z-index: -1; mask-image: linear-gradient(to bottom, #000 0, #000 var(--gallery-fade-start), transparent 100%); -webkit-mask-image: linear-gradient(to bottom, #000 0, #000 var(--gallery-fade-start), transparent 100%); }
.gallery-backdrop img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 1.4s ease; }
.gallery-backdrop img.active { opacity: .5; }
.gallery-backdrop span { position: absolute; inset: 0; background: linear-gradient(90deg, #f6f8f9ed, #f6f8f94a 78%, #f6f8f960); }
.dark .gallery-backdrop span { background: linear-gradient(90deg, #101319ed, #10131950 78%, #10131970); }
@media (prefers-reduced-motion: reduce) { .gallery-backdrop img { transition: none; } }
html.gallery-presenting, html.gallery-presenting body { overflow: hidden; overscroll-behavior: none; }
.gallery-presentation { position: fixed; top: 0; height: var(--gallery-collapsed-height); z-index: 85; overflow: hidden; outline: none; touch-action: none; background: #f6f8f9; opacity: 0; transition: height .8s cubic-bezier(.22,.8,.24,1), opacity .8s ease; }
.gallery-presentation.dark { background: #101319; }
.gallery-presentation-images { position: absolute; inset: 0; opacity: var(--gallery-image-opacity); transition: opacity .65s ease; }
.gallery-presentation-images img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 1.4s ease; }
.gallery-presentation-images img.active { opacity: 1; }
.gallery-presentation-tint { position: absolute; inset: 0; background: linear-gradient(90deg, #f6f8f9ed, #f6f8f94a 78%, #f6f8f960); opacity: 1; transition: opacity .8s ease; }
.gallery-presentation.dark .gallery-presentation-tint { background: linear-gradient(90deg, #101319ed, #10131950 78%, #10131970); }
.gallery-presentation-fade { position: absolute; inset: 0; background: linear-gradient(to bottom, transparent var(--gallery-fade-start), #f6f8f9); opacity: 1; transition: opacity .8s ease; }
.gallery-presentation.dark .gallery-presentation-fade { background: linear-gradient(to bottom, transparent var(--gallery-fade-start), #101319); }
.gallery-presentation :is(.gallery-presentation-tint, .gallery-presentation-fade) { opacity: var(--gallery-tint-opacity); transition-duration: .65s; }
.gallery-presentation { transition-duration: .65s; }
.gallery-presentation.tracking-wheel,
.gallery-presentation.tracking-wheel :is(.gallery-presentation-images, .gallery-presentation-tint, .gallery-presentation-fade) { transition-duration: .1s; transition-timing-function: linear; }
.gallery-presentation.tracking-touch,
.gallery-presentation.tracking-touch :is(.gallery-presentation-images, .gallery-presentation-tint, .gallery-presentation-fade) { transition: none; }
@media (prefers-reduced-motion: reduce) { .gallery-presentation, .gallery-presentation * { transition: none!important; } }
</style>
