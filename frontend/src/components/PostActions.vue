<script setup>
import { computed, ref, watch, nextTick, onUnmounted } from 'vue'
import { Pencil, ArrowLeft, ArrowRight, X, Plus } from '@lucide/vue'
import deleteIcon from '../../icon/删除.png'
const props = defineProps({ post: Object, icon: String, preview: { type: Function, default: value => value } })
const emit = defineEmits(['saved', 'delete', 'overlay'])
const trigger = ref(null)
const menuStyle = ref({})
const menu = ref(false)
const editing = ref(false)
const caption = ref('')
const images = ref([])
const busy = ref(false)
const error = ref('')
const upload = ref(null)
const confirmExit = ref(false)
const exitDialog = ref(null)
let initialCaption = ''
let initialImages = []
const hasChanges = computed(() => caption.value !== initialCaption
  || images.value.length !== initialImages.length
  || images.value.some((image, index) => image.file || image.url !== initialImages[index]))
function requestOutsideExit() {
  if (busy.value || confirmExit.value) return
  if (!hasChanges.value) { cancel(); return }
  confirmExit.value = true
  nextTick(() => exitDialog.value?.querySelector('button')?.focus({ preventScroll: true }))
}
let replaceIndex = -1
function toggleMenu() {
  const rect = trigger.value.getBoundingClientRect()
  menuStyle.value = { left: `${Math.max(12, Math.min(innerWidth - 110, rect.right - 98))}px`, top: `${Math.max(12, Math.min(innerHeight - 108, rect.bottom + 8))}px` }
  menu.value = !menu.value
}
watch([menu, editing], () => {
  emit('overlay', menu.value || editing.value ? { close: () => { menu.value = false; cancel() } } : null)
  nextTick(() => {
    const target = editing.value ? document.querySelector('.post-editor textarea') : menu.value ? document.querySelector('.post-action-menu button') : trigger.value
    target?.focus({preventScroll:true})
  })
})
function onKey(event) {
  if (event.key === 'Escape') {
    event.stopPropagation()
    menu.value = false
    cancel()
  }
  if (event.key !== 'Tab') return
  const controls = [...(confirmExit.value ? exitDialog.value : event.currentTarget).querySelectorAll('button:not(:disabled), textarea:not(:disabled)')]
  const first = controls[0], last = controls.at(-1)
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
function cleanup() {
  for (const image of images.value) if (image.file) URL.revokeObjectURL(image.url)
}
function edit() {
  menu.value = false
  cleanup()
  caption.value = props.post.caption || ''
  images.value = (props.post.media || []).map(url => ({ url }))
  initialCaption = caption.value
  initialImages = images.value.map(image => image.url)
  confirmExit.value = false
  error.value = ''
  editing.value = true
}
function cancel() {
  if (busy.value) return
  editing.value = false
  confirmExit.value = false
  cleanup()
  images.value = []
}
function pick(index = -1) {
  replaceIndex = index
  upload.value.value = ''
  upload.value.click()
}
function addFiles(event) {
  const files = [...event.target.files]
  const incoming = (replaceIndex >= 0 ? files.slice(0, 1) : files).map(file => ({file, url: URL.createObjectURL(file)}))
  if (replaceIndex >= 0 && incoming.length) {
    const old = images.value[replaceIndex]
    if (old.file) URL.revokeObjectURL(old.url)
    images.value.splice(replaceIndex, 1, ...incoming)
  } else images.value.push(...incoming)
}
function remove(index) {
  const [image] = images.value.splice(index, 1)
  if (image.file) URL.revokeObjectURL(image.url)
}
function move(index, step) {
  const next = index + step
  if (next < 0 || next >= images.value.length) return
  ;[images.value[index], images.value[next]] = [images.value[next], images.value[index]]
}
async function save() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const body = new FormData()
    body.append('caption', caption.value)
    let uploadIndex = 0
    body.append('media', JSON.stringify(images.value.map(image => {
      if (!image.file) return image.url
      body.append('images', image.file)
      return `upload:${uploadIndex++}`
    })))
    const response = await fetch(`/api/posts?id=${encodeURIComponent(props.post.id)}`, {method:'PUT', body})
    if (!response.ok) {
      const result = await response.json().catch(() => ({}))
      throw new Error(typeof result.error === 'string' ? result.error : result.error?.message || '保存失败，请重试')
    }
    emit('saved', await response.json())
    busy.value = false
    cancel()
  } catch (e) { error.value = e.message } finally { busy.value = false }
}
onUnmounted(() => { cleanup(); if (menu.value || editing.value) emit('overlay', null) })
</script>

<template>
  <span class="post-actions" @click.stop @touchstart.stop @touchend.stop>
    <button ref="trigger" type="button" class="post-platform-action" aria-label="动态操作" :aria-expanded="menu" @click="!editing && toggleMenu()"><img :src="icon" alt=""></button>
    <Teleport to="body">
      <div v-if="menu" class="post-actions-scrim" @click.self="menu = false" @keydown="onKey">
        <div class="post-action-menu" :style="menuStyle" role="menu" aria-label="动态操作">
          <button type="button" role="menuitem" @click="edit"><Pencil :size="21" aria-hidden="true" />编辑</button>
          <button type="button" role="menuitem" @click="menu = false; emit('delete', post)"><span class="post-delete-symbol" :style="{'--delete-icon': `url(${deleteIcon})`}" aria-hidden="true"></span>删除</button>
        </div>
      </div>
      <div v-if="editing" class="post-editor-backdrop" @click.self="requestOutsideExit" @keydown="onKey" @wheel.stop @touchstart.stop @touchmove.stop @touchend.stop>
        <section class="post-editor" :inert="confirmExit" role="dialog" aria-modal="true" aria-label="编辑动态">
          <header><strong>编辑动态</strong></header>
          <div class="post-editor-content">
            <div class="post-edit-images">
              <div v-for="(image, index) in images" :key="image.url" class="post-edit-image"><button type="button" :disabled="busy" aria-label="替换图片" @click="pick(index)"><img :src="image.file ? image.url : props.preview(image.url)" alt="" loading="lazy" decoding="async"></button><div><button type="button" :disabled="busy || index === 0" aria-label="前移图片" @click="move(index,-1)"><ArrowLeft :size="20" /></button><button type="button" :disabled="busy" aria-label="移除图片" @click="remove(index)"><X :size="20" /></button><button type="button" :disabled="busy || index === images.length-1" aria-label="后移图片" @click="move(index,1)"><ArrowRight :size="20" /></button></div></div>
            </div>
            <textarea v-model="caption" aria-label="动态文本" rows="8" :disabled="busy"></textarea>
            <input ref="upload" type="file" accept="image/jpeg,image/png,image/gif,image/webp" multiple hidden @change="addFiles">
            <p v-if="error" role="alert">{{ error }}</p>
          </div>
          <footer class="post-editor-footer"><button type="button" class="post-add-images" :disabled="busy" @click="pick()"><Plus :size="20" /> 添加图片</button><span class="post-edit-actions"><button type="button" :disabled="busy" @click="cancel">取消</button><button type="button" :disabled="busy" @click="save">{{ busy ? '保存中' : '保存' }}</button></span></footer>
        </section>
        <div v-if="confirmExit" class="post-exit-backdrop">
          <section ref="exitDialog" class="post-exit-dialog" role="alertdialog" aria-modal="true" aria-label="保存修改？">
            <strong>保存修改？</strong>
            <p v-if="error" role="alert">{{ error }}</p>
            <div class="post-edit-actions"><button type="button" :disabled="busy" @click="cancel">不保存退出</button><button type="button" :disabled="busy" @click="save">{{ busy ? '保存中' : '保存' }}</button></div>
          </section>
        </div>
      </div>
    </Teleport>
  </span>
</template>

<style>
.post-actions { display: inline-flex; flex: none; }
.post-platform-action { display: grid; place-items: center; width: 38px; height: 38px; padding: 3px; border: 0; background: transparent; }
.post-actions .post-platform-action { border-radius: 0; overflow: visible; }
.post-actions .post-platform-action img { width: 100%; height: 100%; object-fit: contain; border-radius: 0; }
.post-actions-scrim, .post-editor-backdrop { --editor-ink: #303641; --editor-surface: #fff; position: fixed; inset: 0; z-index: 115; background: #0004; display: grid; place-items: center; padding: max(16px, env(safe-area-inset-top)) 16px max(16px, env(safe-area-inset-bottom)); color: var(--editor-ink); }
html[data-theme="dark"] :is(.post-actions-scrim, .post-editor-backdrop) { --editor-ink: #e4e8f2; --editor-surface: #1b1e26; }
.post-editor-backdrop::before { content: ''; position: absolute; inset: 0; pointer-events: none; backdrop-filter: blur(20px); -webkit-backdrop-filter: blur(20px); }
.post-action-menu, .post-editor { background: var(--editor-surface); box-shadow: 0 18px 70px #0004; border-radius: 16px; }
.post-action-menu { position: absolute; padding: 4px; width: 98px; background: #ffffffc4; backdrop-filter: blur(48px) saturate(115%); -webkit-backdrop-filter: blur(48px) saturate(115%); border: 1px solid #ffffff30; border-radius: 12px; }
html[data-theme="dark"] .post-action-menu { background: #20242cc4; }
.post-action-menu button { display: flex; gap: 8px; align-items: center; width: 100%; min-height: 44px; background: transparent; color: inherit; border-radius: 8px; padding: 8px; font-size: 14px; }
.post-action-menu button:hover, .post-edit-actions button:hover { background: #8a8dd322; }
.post-delete-symbol { width: 21px; height: 21px; background: currentColor; mask: var(--delete-icon) center / contain no-repeat; }
.edit-symbol { width: 21px; font-size: 23px; }
.post-editor { position: relative; width: min(680px,100%); max-height: calc(100dvh - max(16px, env(safe-area-inset-top)) - max(16px, env(safe-area-inset-bottom))); display: flex; flex-direction: column; overflow: hidden; background: #ffffffd9; backdrop-filter: blur(56px) saturate(115%); -webkit-backdrop-filter: blur(56px) saturate(115%); border: 1px solid #ffffff40; }
html[data-theme="dark"] .post-editor { background: #1b1e26e3; border-color: #ffffff1c; }
.post-editor header { display: flex; flex: none; justify-content: space-between; align-items: center; padding: 14px 18px; border-bottom: 1px solid #8883; }
.post-editor-footer { display: flex; flex: none; justify-content: space-between; align-items: center; gap: 8px; padding: 10px 18px; border-top: 1px solid #8883; }
.post-edit-actions { display: inline-flex; gap: 8px; }
.post-edit-actions button { min-height: 44px; padding: 8px 12px; background: transparent; color: inherit; border-radius: 10px; }
.post-edit-actions button:last-child { color: #5149a6; background: #7065c318; }
html[data-theme="dark"] .post-edit-actions button:last-child { color: #c8c1ff; background: #a69bf126; }
.post-edit-actions button:last-child:hover { background: #8a7ddb38; }
.post-add-images { padding: 8px 12px; border-radius: 10px; transition: background .18s ease, color .18s ease; }
.post-add-images:hover { background: #8a8dd322; }
.post-editor-content { min-height: 0; padding: 18px; overflow: auto; overscroll-behavior: contain; }
.post-editor-content, .post-editor textarea { scrollbar-width: none; }
.post-editor-content::-webkit-scrollbar, .post-editor textarea::-webkit-scrollbar { display: none; width: 0; height: 0; }
.post-editor textarea { width: 100%; resize: vertical; min-height: 140px; max-height: 40dvh; border: 1px solid #8885; border-radius: 10px; padding: 12px; color: inherit; background: transparent; font: inherit; font-size: 16px; }
.post-edit-images { display: grid; grid-template-columns: repeat(auto-fill,minmax(132px,1fr)); gap: 10px; min-height: 150px; margin: 0 0 16px; }
.post-edit-images:empty { border: 1px dashed #8885; border-radius: 8px; background: #88888808; }
.post-edit-image > button { display: block; width: 100%; padding: 0; background: transparent; }
.post-edit-image img { width: 100%; aspect-ratio: 1; object-fit: cover; border-radius: 8px; }
.post-edit-image > div { display: flex; justify-content: space-between; }
.post-edit-image button, .post-add-images { min-height: 44px; color: inherit; background: transparent; }
.post-edit-image > div button { min-width: 44px; }
.phone-ui .post-platform-action { width: 44px; height: 44px; padding: 6px; }
.post-edit-image button, .post-add-images { display: inline-flex; align-items: center; justify-content: center; gap: 6px; }
.post-actions-scrim { background: transparent; }
html:has(.post-editor-backdrop, .post-actions-scrim) { overflow: hidden; }
.post-editor button:disabled { opacity: .4; }
.post-editor textarea:focus-visible { outline: 2px solid #9388ce; outline-offset: 1px; }
html[data-theme] .post-editor { backdrop-filter: blur(96px) saturate(125%); -webkit-backdrop-filter: blur(96px) saturate(125%); }
.post-editor [role=alert] { color: #dd5757; }
.post-exit-backdrop { position: absolute; inset: 0; display: grid; place-items: center; padding: 20px; background: #0003; backdrop-filter: blur(8px); }
.post-exit-dialog { width: min(300px, 100%); padding: 20px; border-radius: 16px; color: var(--editor-ink); background: #ffffffdf; backdrop-filter: blur(48px); box-shadow: 0 18px 60px #0004; }
html[data-theme="dark"] .post-exit-dialog { background: #1b1e26e3; }
.post-exit-dialog > .post-edit-actions { display: flex; justify-content: flex-end; margin-top: 18px; }
.post-exit-dialog [role=alert] { color: #dd5757; font-size: 13px; }
.post-exit-dialog button:disabled { opacity: .5; }
</style>
