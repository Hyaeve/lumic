<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
const props = defineProps({ images: { type: Array, default: () => [] } })
const layers = ref(['', ''])
const active = ref(0)
let timer
let version = 0
let previous = ''
const anchor = ref(null)
const host = ref(null)
const backdropStyle = ref({})
let observer
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
watch(() => props.images.join('|'), () => {
  clearInterval(timer)
  const token = ++version
  layers.value = ['', '']
  previous = ''
  void advance(token)
  if (props.images.length > 1) timer = setInterval(() => advance(token), 12000)
}, { immediate: true })
onBeforeUnmount(() => { version++; clearInterval(timer); observer?.disconnect() })
</script>

<template>
  <span ref="anchor" class="gallery-backdrop-anchor" aria-hidden="true"></span>
  <Teleport v-if="host" :to="host">
  <div class="gallery-backdrop" :style="backdropStyle" aria-hidden="true">
    <img v-for="(image, index) in layers" :key="index" :src="image || undefined" :class="{ active: image && index === active }" alt="">
    <span></span>
  </div>
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
</style>
