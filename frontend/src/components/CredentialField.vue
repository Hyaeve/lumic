<script setup>
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { Eye, EyeOff } from '@lucide/vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, required: true },
  multiline: Boolean,
  wide: Boolean,
  disabled: Boolean,
  readonly: Boolean,
  saved: Boolean,
  loadSavedValue: Function,
  savedRevision: [Number, String],
  rows: { type: Number, default: 3 }
})
const emit = defineEmits(['update:modelValue'])

const inputId = useId()
const editor = ref(null)
const visible = ref(false)
const manualEditing = ref(false)
const savedMask = '\u2022'.repeat(8)
const savedValue = ref('')
const loadingSaved = ref(false)
const displayedValue = computed(() => props.modelValue || savedValue.value)
const showSavedMask = computed(() => props.saved && !visible.value && !props.modelValue)
let revealRequest = null
let multilineSelection = null
let composing = false
let compositionValue = null

function restoreValue(event) {
  if (event.target.value !== displayedValue.value) event.target.value = displayedValue.value
}

function cancelSavedRequest() {
  revealRequest?.abort()
  revealRequest = null
  loadingSaved.value = false
}

function hideValue() {
  cancelSavedRequest()
  if (composing || savedValue.value) manualEditing.value = false
  composing = false
  compositionValue = null
  visible.value = false
  savedValue.value = ''
}

function commitValue(value) {
  savedValue.value = ''
  emit('update:modelValue', value)
}

function beginManualEdit(event) {
  if (!event.isTrusted || props.disabled || props.readonly) return
  if (revealRequest) cancelSavedRequest()
  manualEditing.value = true
  // Set the DOM flag before pointer focus opens the mobile keyboard.
  event.target.readOnly = false
}

function beforeInput(event) {
  if (event.isTrusted && manualEditing.value && !props.disabled && !props.readonly) return
  event.preventDefault()
  restoreValue(event)
}

function updateValue(event) {
  if (!event.isTrusted || !manualEditing.value || props.disabled || props.readonly) {
    restoreValue(event)
    return
  }
  if (event.isComposing) {
    compositionValue = event.target.value
    return
  }
  commitValue(event.target.value)
}

function beginComposition(event) {
  beginManualEdit(event)
  if (!event.isTrusted || !manualEditing.value || props.disabled || props.readonly) return
  composing = true
  compositionValue = null
}

function finishComposition(event) {
  // Commit only text seen in a trusted input, including IMEs with synthetic end events.
  if (composing && compositionValue != null && manualEditing.value && !props.disabled && !props.readonly) {
    event.target.value = compositionValue
    commitValue(compositionValue)
  } else restoreValue(event)
  composing = false
  compositionValue = null
}

function finishManualEdit(event) {
  if (composing) finishComposition(event)
  manualEditing.value = false
  event.target.readOnly = true
  nextTick(() => restoreValue(event))
}

async function toggleVisibility() {
  if (props.disabled) return
  if (loadingSaved.value) {
    cancelSavedRequest()
    return
  }

  const previous = editor.value
  let focused = previous === document.activeElement
  const editing = manualEditing.value && !composing
  let selection = previous ? { start: previous.selectionStart, end: previous.selectionEnd, direction: previous.selectionDirection } : null
  let scroll = previous ? { top: previous.scrollTop, left: previous.scrollLeft } : null
  if (props.multiline && visible.value) multilineSelection = { value: props.modelValue, ...selection, scroll }
  else if (props.multiline && multilineSelection?.value === props.modelValue) {
    selection = multilineSelection
    scroll = multilineSelection.scroll
  }
  if (visible.value) {
    hideValue()
  } else if (!props.modelValue && props.saved && props.loadSavedValue) {
    const request = new AbortController()
    revealRequest = request
    loadingSaved.value = true
    try {
      const value = await props.loadSavedValue(request.signal)
      if (revealRequest !== request || request.signal.aborted || props.disabled || !props.saved || props.modelValue || typeof value !== 'string' || !value) return
      focused = focused && previous === document.activeElement
      savedValue.value = value
      visible.value = true
    } catch {
      // A failed reveal leaves the saved value masked.
      return
    } finally {
      if (revealRequest === request) {
        revealRequest = null
        loadingSaved.value = false
      }
    }
  } else {
    visible.value = true
  }
  await nextTick()
  if (focused && !props.disabled) {
    manualEditing.value = editing
    await nextTick()
    editor.value?.focus({ preventScroll: true })
  }
  if (editor.value && selection?.start != null) editor.value.setSelectionRange(selection.start, selection.end, selection.direction)
  if (editor.value && scroll) {
    editor.value.scrollTop = scroll.top
    editor.value.scrollLeft = scroll.left
  }
}

watch(() => props.modelValue, value => {
  if (revealRequest) cancelSavedRequest()
  if (!value) hideValue()
})
watch(() => props.savedRevision, hideValue, { flush: 'sync' })
watch(() => props.saved, saved => {
  if (!saved && (savedValue.value || revealRequest)) hideValue()
}, { flush: 'sync' })
watch(() => props.disabled, disabled => {
  if (disabled) hideValue()
}, { flush: 'sync' })
onBeforeUnmount(hideValue)
</script>

<template>
  <div :class="['credential-field', { 'credential-wide': wide }]">
    <label :for="inputId">{{ label }}</label>
    <div :class="['credential-value-control', { 'credential-value-multiline': multiline }]">
      <textarea v-if="multiline && visible" ref="editor" v-bind="$attrs" :id="inputId" :rows="rows" :value="displayedValue" :disabled="disabled" :readonly="!manualEditing || readonly" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" data-bwignore="true" data-lpignore="true" data-1p-ignore="true" @pointerdown="beginManualEdit" @keydown="beginManualEdit" @paste="beginManualEdit" @compositionstart="beginComposition" @beforeinput="beforeInput" @input="updateValue" @compositionend="finishComposition" @focus="restoreValue" @blur="finishManualEdit"></textarea>
      <input v-else ref="editor" v-bind="$attrs" :id="inputId" :type="visible ? 'text' : 'password'" :value="displayedValue" :placeholder="showSavedMask ? savedMask : $attrs.placeholder" :disabled="disabled" :readonly="!manualEditing || readonly" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" data-bwignore="true" data-lpignore="true" data-1p-ignore="true" @pointerdown="beginManualEdit" @keydown="beginManualEdit" @paste="beginManualEdit" @compositionstart="beginComposition" @beforeinput="beforeInput" @input="updateValue" @compositionend="finishComposition" @focus="restoreValue" @blur="finishManualEdit">
      <button class="credential-visibility-toggle" type="button" :aria-label="`${visible ? '隐藏' : '显示'}${label}`" :aria-pressed="visible" :aria-controls="inputId" :aria-busy="loadingSaved || undefined" data-tooltip-disabled="true" :disabled="disabled" @pointerdown.prevent @click="toggleVisibility"><EyeOff v-if="visible" :size="18" aria-hidden="true" /><Eye v-else :size="18" aria-hidden="true" /></button>
    </div>
  </div>
</template>
