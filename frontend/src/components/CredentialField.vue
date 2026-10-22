<script setup>
import { ref, useId, watch } from 'vue'
import { Eye, EyeOff } from '@lucide/vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, required: true },
  multiline: Boolean,
  wide: Boolean,
  rows: { type: Number, default: 3 }
})
defineEmits(['update:modelValue'])

const inputId = useId()
const visible = ref(false)
watch(() => props.modelValue, value => { if (!value) visible.value = false })
</script>

<template>
  <div :class="['credential-field', { 'credential-wide': wide }]">
    <label :for="inputId">{{ label }}</label>
    <div :class="['credential-value-control', { 'credential-value-multiline': multiline }]">
      <textarea v-if="multiline && visible" v-bind="$attrs" :id="inputId" :rows="rows" :value="modelValue" spellcheck="false" @input="$emit('update:modelValue', $event.target.value)"></textarea>
      <input v-else v-bind="$attrs" :id="inputId" :type="visible ? 'text' : 'password'" :value="modelValue" spellcheck="false" @input="$emit('update:modelValue', $event.target.value)">
      <button class="credential-visibility-toggle" type="button" :aria-label="`${visible ? '隐藏' : '显示'}${label}`" :aria-pressed="visible" :aria-controls="inputId" data-tooltip-disabled="true" :disabled="$attrs.disabled" @click="visible = !visible"><EyeOff v-if="visible" :size="18" aria-hidden="true" /><Eye v-else :size="18" aria-hidden="true" /></button>
    </div>
  </div>
</template>
