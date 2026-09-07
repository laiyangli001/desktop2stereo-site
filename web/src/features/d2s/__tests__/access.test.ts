import { describe, expect, it } from 'vitest'

import { ROLE } from '@/lib/roles'

import { canAccessD2SAdmin } from '../access'

describe('canAccessD2SAdmin', () => {
  it('rejects unauthenticated and common users', () => {
    expect(canAccessD2SAdmin(null)).toBe(false)
    expect(canAccessD2SAdmin(undefined)).toBe(false)
    expect(canAccessD2SAdmin({ role: ROLE.USER })).toBe(false)
    expect(canAccessD2SAdmin({ role: 50 })).toBe(false)
  })

  it('allows administrators and super administrators', () => {
    expect(canAccessD2SAdmin({ role: ROLE.ADMIN })).toBe(true)
    expect(canAccessD2SAdmin({ role: ROLE.SUPER_ADMIN })).toBe(true)
  })
})
