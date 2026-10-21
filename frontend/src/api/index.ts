import { createMockApi } from './mock'
import type { ForumApi } from './types'

declare global {
  interface Window {
    /** Injected by the Wails runtime inside the desktop window. */
    go?: { main?: { App?: unknown } }
  }
}

function hasWailsBindings(): boolean {
  return typeof window !== 'undefined' && window.go?.main?.App !== undefined
}

/** Resolves the backend once: the Wails bindings in the app window, the in-memory mock in a browser. */
export async function createApi(): Promise<ForumApi> {
  if (hasWailsBindings()) {
    const { wailsApi } = await import('./wails')
    return wailsApi
  }
  return createMockApi()
}

export type { ForumApi } from './types'
