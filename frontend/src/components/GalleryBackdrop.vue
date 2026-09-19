<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
let expandFrame
let previousFocus
let touchStart = null
let wheelDistance = 0
let wheelTime = 0
let wheelEvents = 0
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
async function openPresentation() {
  if (!props.interactive || presenting.value || !previous || window.scrollY > 2) return
  previousFocus = document.activeElement
  presentationBounds()
  presenting.value = true
  closing.value = false
  document.documentElement.classList.add('gallery-presenting')
  host.value.inert = true
  emit('present', true)
  lockedUntil = performance.now() + 850
  await nextTick()
  if (!presenting.value || closing.value || !presentationElement.value) return
  // Commit the header-sized first frame before expanding it into the viewport.
  presentationElement.value?.getBoundingClientRect()
  expandFrame = requestAnimationFrame(() => {
    expanded.value = true
    presentationElement.value?.focus({ preventScroll: true })
  })
}
function closePresentation() {
  if (!presenting.value || closing.value) return
  cancelAnimationFrame(expandFrame)
  closing.value = true
  expanded.value = false
  emit('present', false)
  closeTimer = setTimeout(() => {
    presenting.value = false
    closing.value = false
    document.documentElement.classList.remove('gallery-presenting')
    if (host.value) host.value.inert = false
    previousFocus?.focus?.({ preventScroll: true })
    lockedUntil = performance.now() + 350
  }, matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 800)
}
function onWheel(event) {
  if (event.ctrlKey || props.mobile || !props.interactive) return
  if (!presenting.value && (window.scrollY > 2 || excludedTarget(event.target) || event.deltaY >= 0 || !previous)) return
  event.preventDefault()
  event.stopPropagation()
  if (performance.now() < lockedUntil || closing.value) return
  const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? window.innerHeight : 1)
  const time = performance.now()
  if (time - wheelTime > 500 || Math.sign(delta) !== Math.sign(wheelDistance)) {
    wheelDistance = 0
    wheelEvents = 0
  }
  wheelTime = time
  wheelEvents++
  wheelDistance += Math.max(-100, Math.min(100, delta))
  if (wheelEvents >= 3 && ((!presenting.value && wheelDistance < -280) || (presenting.value && wheelDistance > 280))) {
    if (presenting.value) closePresentation()
    else void openPresentation()
    wheelDistance = 0
  }
}
function onTouchStart(event) {
  touchStart = null
  if (!props.interactive || !props.mobile || event.touches.length !== 1 || closing.value) return
  if (!presenting.value && (window.scrollY > 2 || excludedTarget(event.target) || !previous)) return
  const point = event.touches[0]
  touchStart = { x: point.clientX, y: point.clientY, distance: 0 }
}
function onTouchMove(event) {
  if (!touchStart || event.touches.length !== 1) { touchStart = null; return }
  const point = event.touches[0]
  const dx = point.clientX - touchStart.x
  const dy = point.clientY - touchStart.y
  if (Math.abs(dx) > Math.abs(dy) && Math.abs(dx) > 10) { touchStart = null; return }
  if ((!presenting.value && dy > 10) || presenting.value) {
    if (event.cancelable) event.preventDefault()
    event.stopPropagation()
    touchStart.distance = dy
  }
}
function onTouchEnd(event) {
  if (!touchStart) return
  const distance = touchStart.distance
  touchStart = null
  if (event.type === 'touchcancel' || performance.now() < lockedUntil) return
  if ((!presenting.value && distance > 160) || (presenting.value && distance < -160)) {
    event.stopPropagation()
    if (presenting.value) closePresentation()
    else void openPresentation()
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
    timer = setInterval(() => advance(token), presenting.value && !closing.value ? 7000 : 15000)
  }
}
watch([presenting, closing], scheduleRotation)
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
  cancelAnimationFrame(expandFrame)
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
    <section v-if="presenting" ref="presentationElement" class="gallery-presentation" :class="{ expanded, closing, dark }" :style="presentationStyle" tabindex="-1" role="dialog" aria-modal="true" aria-label="背景图片幻灯片" @wheel="onWheel" @touchstart.passive="onTouchStart" @touchmove="onTouchMove" @touchend="onTouchEnd" @touchcancel="onTouchEnd">
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
.gallery-presentation.expanded { height: 100dvh; opacity: 1; }
.gallery-presentation-images { position: absolute; inset: 0; opacity: .5; transition: opacity .8s ease; }
.gallery-presentation-images img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 1.4s ease; }
.gallery-presentation-images img.active { opacity: 1; }
.gallery-presentation-tint { position: absolute; inset: 0; background: linear-gradient(90deg, #f6f8f9ed, #f6f8f94a 78%, #f6f8f960); opacity: 1; transition: opacity .8s ease; }
.gallery-presentation.dark .gallery-presentation-tint { background: linear-gradient(90deg, #101319ed, #10131950 78%, #10131970); }
.gallery-presentation-fade { position: absolute; inset: 0; background: linear-gradient(to bottom, transparent var(--gallery-fade-start), #f6f8f9); opacity: 1; transition: opacity .8s ease; }
.gallery-presentation.dark .gallery-presentation-fade { background: linear-gradient(to bottom, transparent var(--gallery-fade-start), #101319); }
.gallery-presentation.expanded .gallery-presentation-images { opacity: 1; }
.gallery-presentation.expanded :is(.gallery-presentation-tint, .gallery-presentation-fade) { opacity: 0; }
@media (prefers-reduced-motion: reduce) { .gallery-presentation, .gallery-presentation * { transition: none!important; } }
</style>
