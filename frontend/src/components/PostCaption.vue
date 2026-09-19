<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
const props = defineProps({ text: String, expanded: Boolean })
const emit = defineEmits(['update:expanded'])
const element = ref(null)
const overflowing = ref(false)
let observer
function measure() {
  const node = element.value
  if (!node || props.expanded) return
  overflowing.value = node.scrollHeight > node.clientHeight + 1
}
onMounted(() => {
  observer = new ResizeObserver(measure)
  observer.observe(element.value)
  measure()
})
onBeforeUnmount(() => observer?.disconnect())
watch(() => [props.text, props.expanded], () => nextTick(measure))
</script>

<template>
  <div class="post-caption-block">
    <p ref="element" class="caption" :class="{ 'caption-collapsed': !expanded }">{{ text }}</p>
    <button v-if="overflowing || expanded" class="caption-expand" type="button" :aria-expanded="expanded" @click.stop="emit('update:expanded', !expanded)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m4 8.5 8 4.3 8-4.3"/></svg>{{ expanded ? '收起' : '展开' }}</button>
  </div>
</template>

<style>
.post-caption-block .caption { margin-bottom: 0; }
.post-caption-block .caption-collapsed { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 10; overflow: hidden; }
.caption-expand { display: inline-flex; align-items: center; gap: 4px; border: 0; border-radius: 6px; background: transparent; color: #347bc5; padding: 6px 0; font-size: 13px; cursor: pointer; transition: color .18s, filter .18s; }
.dark .caption-expand { color: #89c5ff; }
.caption-expand svg { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; transition: transform .2s; }
.caption-expand[aria-expanded="true"] svg { transform: rotate(180deg); }
.caption-expand:hover, .caption-expand:focus-visible { filter: drop-shadow(0 0 5px #5ba8ffa0); color: #258bea; }
</style>
