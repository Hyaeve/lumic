<script setup>
import { ref, watch } from 'vue'
import ImageLoadRing from './ImageLoadRing.vue'

const props = defineProps({ source: String, preview: String, alt: String })
defineEmits(['open'])
const loaded = ref(false)
const failed = ref(false)
const attempt = ref(0)
let version = 0
watch(() => props.source, () => {
  version++
  loaded.value = false
  failed.value = false
})
async function reveal(event) {
  const image = event.target
  const request = version
  try { await image.decode() } catch {
    if (request === version && image.isConnected) failed.value = true
    return
  }
  if (request === version && image.isConnected) loaded.value = true
}
function retry() {
  version++
  failed.value = false
  loaded.value = false
  attempt.value++
}
</script>

<template>
  <img class="mobile-detail-preview-image" :src="preview" :alt="alt" decoding="async" draggable="false" @click="$emit('open')">
  <img :key="`${source}:${attempt}`" class="mobile-detail-original-image" :class="{ loaded }" :src="source" alt="" aria-hidden="true" decoding="async" draggable="false" @load="reveal" @error="failed = true">
  <ImageLoadRing v-if="!loaded" :failed="failed" @retry="retry" />
</template>
