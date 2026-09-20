<script setup>
import { ref, watch, onMounted } from 'vue'
const props = defineProps({ snapshot: Object })
const host = ref(null)
function render() {
  if (host.value) host.value.replaceChildren(...(props.snapshot ? [props.snapshot.cloneNode(true)] : []))
}
onMounted(render)
watch(() => props.snapshot, render)
</script>

<template>
  <div ref="host" class="page-snapshot" aria-hidden="true" inert></div>
</template>

<style scoped>
.page-snapshot { position: absolute; inset: 0; overflow: hidden; pointer-events: none; }
</style>
