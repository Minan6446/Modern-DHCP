// confirmSensitive — Promise-based imperative mounter for the
// SensitiveConfirmModal component.
//
// Used by the axios response interceptor when the backend returns the
// 4401 sentinel: we pop a dialog, await the operator's password or TOTP
// code, then resolve with the headers the interceptor should attach on
// retry. Cancelling rejects the original request with a typed error so
// the calling page can show a friendly message instead of treating it
// like an HTTP failure.
//
// We borrow Element-Plus's pattern: dynamically create an app root,
// mount the modal, await user input, then unmount + return the result.
// Concurrent calls share a single dialog by queuing — see `pending`
// below — so a burst of failed requests doesn't spawn N stacked dialogs.

import { createApp, h } from 'vue'
import type { App } from 'vue'
import ElementPlus from 'element-plus'
import i18n from '@/i18n'
import SensitiveConfirmModal from '@/components/SensitiveConfirmModal.vue'

export interface SensitiveHeaders {
  password?: string
  totp?: string
}

export class SensitiveCancelled extends Error {
  constructor() {
    super('sensitive confirm cancelled')
    this.name = 'SensitiveCancelled'
  }
}

interface PendingResolver {
  resolve: (headers: SensitiveHeaders) => void
  reject: (err: Error) => void
}

// Single-flight: while one dialog is up, queue further requests so they
// reuse the same operator response (typing password / TOTP twice in a
// row when 5 requests fail at once is awful UX). All queued resolvers
// fire with the same headers when the dialog confirms.
let activeApp: App | null = null
let activeRoot: HTMLElement | null = null
let pending: PendingResolver[] = []

const teardown = (): void => {
  if (activeApp) {
    activeApp.unmount()
    activeApp = null
  }
  if (activeRoot) {
    activeRoot.remove()
    activeRoot = null
  }
}

export function confirmSensitive(opts: { mfaEnabled: boolean; message?: string }): Promise<SensitiveHeaders> {
  return new Promise<SensitiveHeaders>((resolve, reject) => {
    pending.push({ resolve, reject })
    if (activeApp) {
      // Dialog already open — the existing one will resolve everyone.
      return
    }

    activeRoot = document.createElement('div')
    document.body.appendChild(activeRoot)

    const flushResolve = (h: SensitiveHeaders): void => {
      const queued = pending
      pending = []
      teardown()
      queued.forEach(p => p.resolve(h))
    }
    const flushReject = (): void => {
      const queued = pending
      pending = []
      teardown()
      queued.forEach(p => p.reject(new SensitiveCancelled()))
    }

    activeApp = createApp({
      render: () => h(SensitiveConfirmModal, {
        mfaEnabled: opts.mfaEnabled,
        message: opts.message,
        onConfirm: (payload: SensitiveHeaders) => flushResolve(payload),
        onCancel: () => flushReject(),
      }),
    })
    // Mounted apps need their own ElementPlus + i18n instances since
    // they live outside the main Vue root tree.
    activeApp.use(ElementPlus)
    activeApp.use(i18n)
    activeApp.mount(activeRoot)
  })
}
