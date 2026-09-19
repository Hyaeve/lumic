<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
const props = defineProps({ text: String, expanded: Boolean })
const emit = defineEmits(['update:expanded'])
const element = ref(null)
const overflowing = ref(false)
let observer
function measure() {
  const node = element.value
  if (!node) return
  const line = parseFloat(getComputedStyle(node).lineHeight)
  overflowing.value = node.scrollHeight > line * 10 + 2
}
onMounted(() => {
  observer = new ResizeObserver(measure)
  observer.observe(element.value)
  measure()
})
onBeforeUnmount(() => observer?.disconnect())
watch(() => props.text, () => nextTick(measure))
</script>

<template>
  <div class="post-caption-block">
    <p ref="element" class="caption" :class="{ 'caption-collapsed': !expanded }">{{ text }}</p>
    <button v-if="overflowing || expanded" class="caption-expand" type="button" :aria-expanded="expanded" @click.stop="emit('update:expanded', !expanded)">{{ expanded ? '收起' : '展开' }}</button>
  </div>
</template>

<style>
.post-caption-block .caption { margin-bottom: 0; }
.post-caption-block .caption-collapsed { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 10; overflow: hidden; }
.caption-expand { border: 0; background: transparent; color: var(--accent, #459d97); padding: 6px 0; font-size: 12px; cursor: pointer; }
</style>
