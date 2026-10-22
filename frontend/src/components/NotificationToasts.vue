<script setup>
import { CircleCheck, CircleAlert, Info, X } from '@lucide/vue'
import { notifications } from '../notifications'

const { items, dismiss, pause } = notifications
</script>

<template>
  <Teleport to="body">
    <div class="notification-stack" aria-label="操作结果通知">
      <TransitionGroup name="notice" tag="div" class="notification-list">
        <div v-for="item in items" :key="item.id" :class="['notification-toast', `notification-${item.tone}`]" @mouseenter="pause(item.id, 'hover', true)" @mouseleave="pause(item.id, 'hover', false)" @focusin="pause(item.id, 'focus', true)" @focusout="pause(item.id, 'focus', false)">
          <span class="notification-content" :role="item.tone === 'error' ? 'alert' : 'status'" aria-atomic="true"><component :is="item.tone === 'error' ? CircleAlert : item.tone === 'success' ? CircleCheck : Info" :size="20" aria-hidden="true" /><span>{{ item.message }}</span></span>
          <button type="button" aria-label="关闭通知" data-tooltip-disabled="true" @click="dismiss(item.id)"><X :size="18" aria-hidden="true" /></button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style>
.notification-stack { position: fixed; z-index: 30000; top: max(20px, env(safe-area-inset-top)); right: max(20px, env(safe-area-inset-right)); width: min(360px, calc(100vw - 32px)); pointer-events: none; }
.notification-list { display: grid; gap: 10px; max-height: 80dvh; overflow-y: auto; scrollbar-width: none; }
.notification-list::-webkit-scrollbar { display: none; }
.notification-toast { --notice-accent: #356bba; display: flex; align-items: center; gap: 8px; min-height: 56px; padding: 6px 4px 6px 14px; color: #252d3c; background: rgba(250, 251, 255, .94); border: 1px solid #b1bdd533; border-radius: 8px; box-shadow: 0 8px 30px #18274020; backdrop-filter: blur(24px); -webkit-backdrop-filter: blur(24px); pointer-events: auto; font: 14px/1.5 'Microsoft YaHei', sans-serif; letter-spacing: 0; }
.notification-content { display: flex; align-items: center; gap: 10px; flex: 1; min-width: 0; }
.notification-content > svg { flex: none; color: var(--notice-accent); }
.notification-content > span { overflow-wrap: anywhere; white-space: pre-line; max-height: min(240px, 35dvh); overflow-y: auto; }
.notification-error { --notice-accent: #c33f56; }
.notification-info { --notice-accent: #6b5caa; }
.notification-toast > button { display: grid; place-items: center; flex: none; width: 44px; height: 44px; border: 0; border-radius: 6px; background: transparent; color: inherit; cursor: pointer; }
.notification-toast > button:hover { background: #8899bb20; }
.notification-toast > button:focus-visible { outline: 2px solid var(--notice-accent); outline-offset: -3px; }
html[data-theme='dark'] .notification-toast { --notice-accent: #afa8f1; color: #eceef7; background: rgba(31, 35, 47, .95); border-color: #aebbe126; box-shadow: 0 8px 30px #0004; }
html[data-theme='dark'] .notification-error { --notice-accent: #ff9caf; }
html[data-theme='dark'] .notification-info { --notice-accent: #a9bbf4; }
.notice-enter-active { transition: opacity 220ms ease, transform 300ms cubic-bezier(.16, 1, .3, 1); }
.notice-leave-active { transition: opacity 160ms ease, transform 180ms ease; }
.notice-move { transition: transform 240ms ease; }
.notice-enter-from, .notice-leave-to { opacity: 0; transform: translateY(-10px) scale(.98); }
@media (max-width: 760px) { .notification-stack { top: calc(max(12px, env(safe-area-inset-top)) + 64px); right: max(16px, env(safe-area-inset-right)); } }
@media (prefers-reduced-motion: reduce) { .notice-enter-active, .notice-leave-active, .notice-move { transition: none; } }
</style>
