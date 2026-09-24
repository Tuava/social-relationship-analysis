import { defineStore } from 'pinia'

/**
 * Persists lightweight list-page state (search query, page number, filters)
 * across navigation so users don't lose their place when switching pages.
 * Uses sessionStorage so state survives SPA navigation but is cleared on
 * a fresh browser session.
 */
type ListState = {
  query?: string
  page?: number
  pageSize?: number
  filters?: Record<string, unknown>
  [key: string]: unknown
}

const NAMESPACE = 'sra.list-state.v1'

function readAll(): Record<string, ListState> {
  try {
    return JSON.parse(sessionStorage.getItem(NAMESPACE) || '{}')
  } catch {
    return {}
  }
}

function writeAll(data: Record<string, ListState>) {
  try {
    sessionStorage.setItem(NAMESPACE, JSON.stringify(data))
  } catch {
    /* quota or privacy mode — silently ignore */
  }
}

export const useListStateStore = defineStore('listState', {
  state: () => ({} as Record<string, ListState>),
  actions: {
    /** Load persisted state for a page key, merged over the given defaults. */
    load(pageKey: string, defaults: ListState = {}): ListState {
      const all = readAll()
      return { ...defaults, ...all[pageKey] }
    },
    /** Persist state for a page key. */
    save(pageKey: string, state: ListState) {
      const all = readAll()
      all[pageKey] = state
      writeAll(all)
    },
    /** Clear persisted state for a page key. */
    clear(pageKey: string) {
      const all = readAll()
      delete all[pageKey]
      writeAll(all)
    },
  },
})
