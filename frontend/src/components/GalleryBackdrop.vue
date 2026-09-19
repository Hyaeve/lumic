<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
const props = defineProps({ images: { type: Array, default: () => [] } })
const layers = ref(['', ''])
const active = ref(0)
let timer
let version = 0
let previous = ''
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
onBeforeUnmount(() => { version++; clearInterval(timer) })
</script>

<template>
  <div class="gallery-backdrop" aria-hidden="true">
    <img v-for="(image, index) in layers" :key="index" :src="image || undefined" :class="{ active: image && index === active }" alt="">
    <span></span>
  </div>
</template>

<style>
.gallery-backdrop { position: absolute; inset: 0; overflow: hidden; border-radius: inherit; pointer-events: none; z-index: 0; }
.gallery-backdrop img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 1.4s ease; }
.gallery-backdrop img.active { opacity: .5; }
.gallery-backdrop span { position: absolute; inset: 0; background: linear-gradient(90deg, #f6f8f9ed, #f6f8f94a 78%, #f6f8f960); }
.dark .gallery-backdrop span { background: linear-gradient(90deg, #101319ed, #10131950 78%, #10131970); }
@media (prefers-reduced-motion: reduce) { .gallery-backdrop img { transition: none; } }
</style>
