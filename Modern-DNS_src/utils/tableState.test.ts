import { beforeEach, describe, expect, it } from 'vitest'
import { loadTableState, saveTableState } from './tableState'

describe('tableState', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
  })

  it('returns fallback state when nothing is stored', () => {
    const fallback = { page: 1, pageSize: 20 }

    expect(loadTableState('table:missing', fallback)).toEqual(fallback)
  })

  it('merges stored state into fallback state', () => {
    const fallback = { page: 1, pageSize: 20, keyword: '' }

    window.sessionStorage.setItem('table:search', JSON.stringify({ page: 3, keyword: 'dns' }))

    expect(loadTableState('table:search', fallback)).toEqual({
      page: 3,
      pageSize: 20,
      keyword: 'dns',
    })
  })

  it('persists state to sessionStorage', () => {
    const state = { page: 2, pageSize: 50 }

    saveTableState('table:list', state)

    expect(window.sessionStorage.getItem('table:list')).toBe(JSON.stringify(state))
  })
})