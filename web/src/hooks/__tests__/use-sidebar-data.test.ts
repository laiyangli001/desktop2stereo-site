import { renderHook } from '@testing-library/react'
import { expect, it } from 'vitest'

import { checkIsActive } from '@/components/layout/lib/url-utils'
import {
  filterSidebarNavGroupsByConfig,
  parseSidebarConfig,
} from '../use-sidebar-config'
import { useSidebarData } from '../use-sidebar-data'

it('places support tickets in the personal group with a separate active navigation item', () => {
  const { result } = renderHook(() => useSidebarData())
  const groups = result.current.navGroups
  const personal = groups.find((group) => group.id === 'personal')
  const tickets = personal?.items.find(
    (item) => item.url === '/d2s/support-tickets'
  )
  const licenses = personal?.items.find((item) => item.url === '/d2s')
  expect(tickets?.title).toBe('My support tickets')
  const store = personal?.items.find((item) => item.url === '/d2s/store')
  expect(store?.title).toBe('Store')
  expect(
    personal?.items.filter((item) => item.url === '/d2s/store')
  ).toHaveLength(1)
  expect(tickets?.items).toBeUndefined()
  expect(licenses).toBeDefined()
  if (!tickets || !licenses) {
    throw new Error('Missing personal navigation entries')
  }
  expect(checkIsActive('/d2s/support-tickets', tickets)).toBe(true)
  expect(checkIsActive('/d2s/support-tickets', licenses)).toBe(false)
  expect(personal?.items.some((item) => item.url === '/support')).toBe(false)

  const hidden = filterSidebarNavGroupsByConfig(
    groups,
    parseSidebarConfig({ personal: { enabled: false } })
  )
  expect(
    hidden
      .flatMap((group) => group.items)
      .some((item) => item.url === '/d2s/support-tickets')
  ).toBe(false)
})
