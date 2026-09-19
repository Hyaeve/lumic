<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
const props = defineProps({ images: { type: Array, default: () => [] }, interactive: Boolean, mobile: Boolean, dark: Boolean })
const emit = defineEmits(['present', 'image', 'open-post'])
const anchor = ref(null)
const host = ref(null)
const presentationElement = ref(null)
const layers = ref(['', ''])
const active = ref(0)
const current = ref('')
const backdropStyle = ref({})
const boundsStyle = ref({})
const presenting = ref(false)
const expanded = ref(false)
const closing = ref(false)
const progress = ref(0)
const dragging = ref(false)
const shift = ref(0)
const fastSwitch = ref(false)
const entryHovered = ref(false)
const entryTop = ref(0)
const entryLeft = ref(0)
const entryWidth = ref(80)
const entryScreenTop = ref(0)
const presentationStartHeight = ref(320)
const viewportHeight = ref(window.innerHeight)
let observer, timer, closeTimer, tapTimer, frame
let previousFocus
let touch = null
let lastTap = null
let loadVersion = 0
let sequence = []
let sequenceIndex = -1
let lockedUntil = 0
let mounted = false
const excludedTarget = target => Boolean(target?.closest('button, a, input, textarea, video, .media-frame, .modal, .lightbox-layer'))
const presentationStyle = computed(() => {
  const initial = presentationStartHeight.value
  return {
    ...boundsStyle.value,
    height: `${initial + (viewportHeight.value - initial) * progress.value}px`,
    opacity: Math.min(1, progress.value * 3),
    '--gallery-image-opacity': .5 + progress.value * .5,
    '--gallery-tint-opacity': 1 - progress.value,
    '--gallery-chevron-top': `${entryScreenTop.value + (viewportHeight.value - 64 - entryScreenTop.value) * progress.value}px`,
    '--gallery-chevron-left': `${entryLeft.value + ((parseFloat(boundsStyle.value.width) - entryWidth.value) / 2 - entryLeft.value) * progress.value}px`,
    '--gallery-chevron-width': `${entryWidth.value}px`,
    '--gallery-chevron-angle': `${progress.value * 180}deg`
  }
})
function measure() {
  if (!host.value) return
  viewportHeight.value = window.innerHeight
  const bounds = host.value.getBoundingClientRect()
  const toolbarElement = host.value.querySelector('.section-heading')
  const toolbar = toolbarElement?.getBoundingClientRect()
  const filters = toolbarElement?.querySelector('.filters')?.getBoundingClientRect()
  const tools = toolbarElement?.querySelector('.timeline-tools')?.getBoundingClientRect()
  const bottom = toolbar?.bottom ?? bounds.top + 200
  const fadeStart = Math.max(120, bottom - bounds.top - 32)
  entryTop.value = (toolbar ? toolbar.top + toolbar.height / 2 : bottom - 22) - bounds.top - 24
  const gapLeft = filters?.right ?? bounds.left
  const gapRight = tools?.left ?? bounds.right
  entryWidth.value = Math.max(44, Math.min(80, gapRight - gapLeft - 8))
  entryLeft.value = (gapLeft + gapRight - entryWidth.value) / 2 - bounds.left
  backdropStyle.value = { height: `${fadeStart + 120}px`, '--gallery-fade-start': `${fadeStart}px` }
  boundsStyle.value = { left: `${props.mobile ? 0 : bounds.left}px`, width: `${props.mobile ? window.innerWidth : bounds.width}px`, '--gallery-fade-start': `${fadeStart}px` }
}
function updateHover(event) {
  if (props.mobile || presenting.value) return
  const bounds = host.value.getBoundingClientRect()
  const toolbar = host.value.querySelector('.section-heading')?.getBoundingClientRect()
  entryHovered.value = Boolean(current.value && toolbar && event.clientY >= bounds.top &&
    event.clientY <= toolbar.bottom && !event.target.closest('.scoped-timeline-identity, .scoped-timeline-stats, input, .timeline-toolbar-button'))
}
function clearHover() { entryHovered.value = false }
function preparePresentation() {
  if (presenting.value) return true
  if (!props.interactive || !current.value || (props.mobile && window.scrollY > 2)) return false
  previousFocus = document.activeElement
  measure()
  const top = host.value.getBoundingClientRect().top
  entryScreenTop.value = entryTop.value + top
  presentationStartHeight.value = Math.max(120, (parseFloat(backdropStyle.value.height) || 320) + top)
  presenting.value = true
  closing.value = false
  progress.value = 0
  document.documentElement.classList.add('gallery-presenting')
  host.value.inert = true
  return true
}
function commitPresentation() {
  progress.value = 1
  lockedUntil = performance.now() + 600
  if (!expanded.value) {
    expanded.value = true
    emit('present', true)
  }
  void nextTick(() => presentationElement.value?.focus({ preventScroll: true }))
}
async function openPresentation() {
  if (!preparePresentation()) return
  await nextTick()
  presentationElement.value?.getBoundingClientRect()
  frame = requestAnimationFrame(() => { if (presenting.value && !closing.value) commitPresentation() })
}
function settleGesture(cancelled = false) {
  if (!presenting.value || closing.value) return
  dragging.value = false
  const keepOpen = cancelled ? expanded.value : expanded.value ? progress.value > .4 : progress.value >= .6
  if (keepOpen) commitPresentation()
  else closePresentation()
}
function closePresentation(options = {}) {
  if (!presenting.value || closing.value) return
  clearTimeout(tapTimer)
  cancelAnimationFrame(frame)
  lastTap = null
  touch = null
  dragging.value = false
  shift.value = 0
  progress.value = 0
  closing.value = true
  if (expanded.value) {
    expanded.value = false
    emit('present', false)
  }
  const finish = () => {
    presenting.value = false
    closing.value = false
    entryHovered.value = false
    document.documentElement.classList.remove('gallery-presenting')
    if (host.value) host.value.inert = false
    if (!options.immediate) previousFocus?.focus?.({ preventScroll: true })
    lockedUntil = performance.now() + 250
  }
  if (options.immediate || matchMedia('(prefers-reduced-motion: reduce)').matches) finish()
  else closeTimer = setTimeout(finish, 600)
}
function openCurrentPost(event) {
  if (!expanded.value || closing.value || event?.target?.closest('button')) return
  clearTimeout(tapTimer)
  lastTap = null
  emit('open-post', current.value)
}
function scheduleRotation() {
  clearTimeout(timer)
  if (props.images.length > 1) {
    timer = setTimeout(() => {
      if (!touch && !dragging.value && !closing.value) void advance(1, false)
      else scheduleRotation()
    }, expanded.value ? (props.mobile ? 10000 : 9000) : 15000)
  }
}
async function advance(direction = 1, manual = false) {
  const choices = props.images.filter(image => image && image !== current.value)
  const nextIndex = sequenceIndex + direction
  const source = sequence[nextIndex] || choices[Math.floor(Math.random() * choices.length)] || props.images[0]
  if (!source || source === current.value) { scheduleRotation(); return }
  const token = ++loadVersion
  const image = new Image()
  image.src = source
  try { await image.decode() } catch { if (token === loadVersion) scheduleRotation(); return }
  if (token !== loadVersion) return
  if (nextIndex < 0) {
    sequence.unshift(source)
    sequenceIndex = 0
  } else {
    sequence[nextIndex] = source
    sequenceIndex = nextIndex
  }
  fastSwitch.value = manual
  const next = 1 - active.value
  layers.value[next] = source
  active.value = next
  current.value = source
  emit('image', source)
  scheduleRotation()
}
function onTouchStart(event) {
  clearTimeout(tapTimer)
  if (event.touches.length !== 1) lastTap = null
  if (touch && event.touches.length !== 1) {
    if (dragging.value) settleGesture(true)
    shift.value = 0
  }
  touch = null
  if (!props.interactive || !props.mobile || event.touches.length !== 1 || closing.value || performance.now() < lockedUntil) return
  if (!presenting.value && (window.scrollY > 2 || excludedTarget(event.target) || !current.value)) return
  const point = event.touches[0]
  touch = { x: point.clientX, y: point.clientY, progress: progress.value, time: performance.now(), distance: 0, canTap: expanded.value, axis: '', dx: 0 }
}
function onTouchMove(event) {
  if (!touch || event.touches.length !== 1) return
  const point = event.touches[0]
  const dx = point.clientX - touch.x
  const dy = point.clientY - touch.y
  touch.distance = Math.max(touch.distance, Math.hypot(dx, dy))
  if (touch.distance <= 10) return
  clearTimeout(tapTimer)
  lastTap = null
  if (!touch.axis) touch.axis = Math.abs(dx) > Math.abs(dy) ? 'x' : 'y'
  if (touch.axis === 'x') {
    if (!expanded.value) { touch = null; return }
    if (event.cancelable) event.preventDefault()
    event.stopPropagation()
    touch.dx = dx
    dragging.value = true
    shift.value = dx * .3
    return
  }
  if (!presenting.value && dy <= 10) { touch = null; return }
  if (event.cancelable) event.preventDefault()
  event.stopPropagation()
  if (!preparePresentation()) return
  dragging.value = true
  progress.value = Math.max(0, Math.min(1, touch.progress + dy / 280))
}
function onTouchEnd(event) {
  if (!touch) return
  const gesture = touch
  touch = null
  const cancelled = event.type === 'touchcancel'
  if (gesture.axis === 'x') {
    dragging.value = false
    shift.value = 0
    if (!cancelled && Math.abs(gesture.dx) > 55) void advance(gesture.dx < 0 ? 1 : -1, true)
    return
  }
  const point = event.changedTouches[0]
  const distance = Math.max(gesture.distance, point ? Math.hypot(point.clientX - gesture.x, point.clientY - gesture.y) : 0)
  if (!cancelled && event.touches.length === 0 && gesture.canTap && expanded.value &&
      distance <= 10 && performance.now() - gesture.time <= 350) {
    if (event.cancelable) event.preventDefault()
    event.stopPropagation()
    const now = performance.now()
    if (lastTap && now - lastTap.time < 300 && Math.hypot(gesture.x - lastTap.x, gesture.y - lastTap.y) < 36) {
      openCurrentPost()
    } else {
      clearTimeout(tapTimer)
      lastTap = { time: now, x: gesture.x, y: gesture.y }
      tapTimer = setTimeout(() => closePresentation(), 300)
    }
    return
  }
  if (dragging.value) {
    event.stopPropagation()
    settleGesture(cancelled)
  }
}
function onKey(event) {
  if (!presenting.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopImmediatePropagation()
    closePresentation()
  } else if (expanded.value && ['ArrowLeft', 'ArrowRight'].includes(event.key)) {
    event.preventDefault()
    event.stopImmediatePropagation()
    void advance(event.key === 'ArrowLeft' ? -1 : 1, true)
  } else if (event.key === 'Tab') {
    const buttons = [...(presentationElement.value?.querySelectorAll('button:not(:disabled)') || [])]
    event.preventDefault()
    if (!buttons.length) presentationElement.value?.focus()
    else {
      const index = buttons.indexOf(document.activeElement)
      buttons[(index + (event.shiftKey ? -1 : 1) + buttons.length) % buttons.length].focus()
    }
  }
}
defineExpose({ close: closePresentation })
onMounted(() => {
  mounted = true
  host.value = anchor.value?.closest('.content, .mobile-transition-page-content')
  if (!host.value) return
  observer = new ResizeObserver(measure)
  observer.observe(host.value)
  const toolbar = host.value.querySelector('.section-heading')
  if (toolbar) observer.observe(toolbar)
  measure()
  if (props.interactive) {
    host.value.addEventListener('pointermove', updateHover)
    host.value.addEventListener('pointerleave', clearHover)
    host.value.addEventListener('touchstart', onTouchStart, { passive: true, capture: true })
    host.value.addEventListener('touchmove', onTouchMove, { passive: false, capture: true })
    host.value.addEventListener('touchend', onTouchEnd, true)
    host.value.addEventListener('touchcancel', onTouchEnd, true)
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('resize', measure)
  }
})
watch([expanded, () => props.mobile], scheduleRotation)
watch(() => props.images.join('|'), () => {
  clearTimeout(timer)
  loadVersion++
  sequence = []
  sequenceIndex = -1
  layers.value = ['', '']
  current.value = ''
  if (mounted && presenting.value) closePresentation({ immediate: true })
  void advance(1)
}, { immediate: true })
onBeforeUnmount(() => {
  loadVersion++
  clearTimeout(timer)
  clearTimeout(closeTimer)
  clearTimeout(tapTimer)
  cancelAnimationFrame(frame)
  observer?.disconnect()
  if (!props.interactive) return
  host.value?.removeEventListener('pointermove', updateHover)
  host.value?.removeEventListener('pointerleave', clearHover)
  host.value?.removeEventListener('touchstart', onTouchStart, true)
  host.value?.removeEventListener('touchmove', onTouchMove, true)
  host.value?.removeEventListener('touchend', onTouchEnd, true)
  host.value?.removeEventListener('touchcancel', onTouchEnd, true)
  window.removeEventListener('keydown', onKey, true)
  window.removeEventListener('resize', measure)
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
    <button v-if="interactive && !mobile && !presenting && current" class="gallery-entry-chevron gallery-chevron" :class="{ visible: entryHovered }" :style="{ top: `${entryTop}px`, left: `${entryLeft}px`, width: `${entryWidth}px` }" type="button" aria-label="展开背景大图" @click="openPresentation">
      <svg viewBox="0 0 64 40" aria-hidden="true"><path d="m8 13 24 16 24-16"/></svg>
    </button>
  </Teleport>
  <Teleport to="body">
    <section v-if="presenting" ref="presentationElement" class="gallery-presentation" :class="{ expanded, closing, dark, dragging, 'fast-switch': fastSwitch }" :style="presentationStyle" tabindex="-1" role="dialog" aria-modal="true" aria-label="背景图片幻灯片" @wheel.prevent @dblclick="!mobile && openCurrentPost($event)" @touchstart.passive="onTouchStart" @touchmove="onTouchMove" @touchend="onTouchEnd" @touchcancel="onTouchEnd">
      <div class="gallery-presentation-images" :style="{ transform: `translateX(${shift}px)` }"><img v-for="(image, index) in layers" :key="index" :src="image || undefined" :class="{ active: image && index === active }" alt="" draggable="false"></div>
      <div class="gallery-presentation-tint"></div>
      <div class="gallery-presentation-fade"></div>
      <template v-if="!mobile">
        <button class="gallery-exit-chevron gallery-chevron" type="button" aria-label="收起背景大图" @click.stop="closePresentation()"><svg viewBox="0 0 64 40" aria-hidden="true"><path d="m8 13 24 16 24-16"/></svg></button>
        <button v-for="direction in [-1, 1]" :key="direction" class="gallery-edge-nav" :class="{ previous: direction < 0, next: direction > 0 }" :disabled="images.length < 2" :aria-label="direction < 0 ? '上一张背景图' : '下一张背景图'" @click.stop="advance(direction, true)"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="direction < 0 ? 'm15 4-8 8 8 8' : 'm9 4 8 8-8 8'"/></svg></button>
      </template>
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
html.gallery-presenting, html.gallery-presenting body { overflow: hidden; overscroll-behavior: none; }
.gallery-presentation { position: fixed; top: 0; z-index: 85; overflow: hidden; outline: none; touch-action: none; background: #f6f8f9; transition: height .6s cubic-bezier(.22,.8,.24,1), opacity .6s ease; }
.gallery-presentation.dark { background: #101319; }
.gallery-presentation-images { position: absolute; inset: 0; opacity: var(--gallery-image-opacity); transition: opacity .6s ease, transform .32s cubic-bezier(.2,.8,.25,1); }
.gallery-presentation-images img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 1.4s ease; user-select: none; }
.gallery-presentation-images img.active { opacity: 1; }
.gallery-presentation.fast-switch .gallery-presentation-images img { transition-duration: .32s; }
.gallery-presentation-tint { position: absolute; inset: 0; background: linear-gradient(90deg, #f6f8f9ed, #f6f8f94a 78%, #f6f8f960); }
.gallery-presentation.dark .gallery-presentation-tint { background: linear-gradient(90deg, #101319ed, #10131950 78%, #10131970); }
.gallery-presentation-fade { position: absolute; inset: 0; background: linear-gradient(to bottom, transparent var(--gallery-fade-start), #f6f8f9); }
.gallery-presentation.dark .gallery-presentation-fade { background: linear-gradient(to bottom, transparent var(--gallery-fade-start), #101319); }
.gallery-presentation :is(.gallery-presentation-tint, .gallery-presentation-fade) { opacity: var(--gallery-tint-opacity); transition: opacity .6s ease; pointer-events: none; }
.gallery-presentation.dragging, .gallery-presentation.dragging .gallery-presentation-images { transition: none; }
.gallery-presentation.dragging :is(.gallery-presentation-tint, .gallery-presentation-fade) { transition: none; }
.gallery-chevron { position: absolute; left: calc(50% - 40px); width: 80px; height: 48px; display: grid; place-items: center; padding: 0; border: 0; background: transparent; color: var(--ink, #fff); z-index: 4; }
.gallery-chevron svg { width: min(64px, 100%); height: 40px; fill: none; stroke: currentColor; stroke-width: 2.4; stroke-linecap: round; stroke-linejoin: round; animation: gallery-chevron-breathe 2.2s ease-in-out infinite; filter: drop-shadow(0 2px 5px #0005); }
.gallery-entry-chevron { opacity: 0; pointer-events: none; transition: opacity .25s ease; }
.gallery-entry-chevron.visible, .gallery-entry-chevron:focus-visible { opacity: 1; pointer-events: auto; }
.gallery-chevron:focus-visible { outline: 2px solid #a6a0ed; outline-offset: 2px; border-radius: 8px; }
.gallery-exit-chevron { top: var(--gallery-chevron-top); left: var(--gallery-chevron-left); width: var(--gallery-chevron-width); color: #fff; transition: top .6s cubic-bezier(.22,.8,.24,1), left .6s cubic-bezier(.22,.8,.24,1); }
.gallery-exit-chevron svg { rotate: var(--gallery-chevron-angle); transition: rotate .6s cubic-bezier(.22,.8,.24,1); }
.gallery-edge-nav { position: absolute; top: 0; bottom: 0; width: min(13%, 104px); padding: 0; display: grid; place-items: center; background: transparent; color: #fff; border: 0; opacity: 0; transition: opacity .2s ease; }
.gallery-edge-nav.previous { left: 0; }
.gallery-edge-nav.next { right: 0; }
.gallery-edge-nav svg { width: 40px; height: 40px; fill: none; stroke: currentColor; stroke-width: 1.6; stroke-linecap: round; stroke-linejoin: round; filter: drop-shadow(0 2px 5px #000a); }
.gallery-edge-nav:hover, .gallery-edge-nav:focus-visible { opacity: 1; }
.gallery-edge-nav:disabled { color: #8b8b8b; cursor: default; }
@keyframes gallery-chevron-breathe { 0%, 100% { opacity: .32; transform: translateY(2px); } 50% { opacity: .95; transform: translateY(-2px); } }
@media (min-width: 761px) {
  .content:has(> .scoped-timeline-header) > .section-heading { display: grid; grid-template-columns: auto minmax(64px, 1fr) minmax(110px, 300px); gap: 8px; align-items: center; }
  .content:has(> .scoped-timeline-header) > .section-heading > .timeline-tools { grid-column: 3; width: 100%; min-width: 0; margin-left: 0; }
  .content:has(> .scoped-timeline-header) > .section-heading .timeline-search { width: 100%; }
}
@media (prefers-reduced-motion: reduce) {
  .gallery-backdrop img, .gallery-presentation, .gallery-presentation *, .gallery-chevron, .gallery-chevron svg { transition: none!important; animation: none!important; }
}
</style>
