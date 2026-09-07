import { describe, expect, it } from 'vitest'

import type { NavGroup } from '@/components/layout/types'
import { D2S_ADMIN_ROLES } from '@/features/d2s/access'
import { ROLE } from '@/lib/roles'

import { filterSidebarNavGroupsByRole } from '../use-sidebar-view'

const navGroups: NavGroup[] = [
  {
    id: 'personal',
    title: 'Personal',
    items: [{ title: 'Desktop2Stereo', url: '/d2s' }],
  },
  {
    id: 'admin',
    title: 'Admin',
    items: [
      {
        title: 'Desktop2Stereo Admin',
        url: '/d2s-admin',
        requiredRole: ROLE.ADMIN,
        requiredRoles: D2S_ADMIN_ROLES,
      },
      {
        title: 'System Info',
        url: '/system-info',
        requiredRole: ROLE.SUPER_ADMIN,
      },
    ],
  },
]

describe('filterSidebarNavGroupsByRole', () => {
  it('hides all admin navigation for unauthenticated users', () => {
    const groups = filterSidebarNavGroupsByRole(navGroups)

    expect(groups.map((group) => group.id)).toEqual(['personal'])
  })

  it('shows D2S admin only to administrators and reserves system info for super admins', () => {
    const adminGroups = filterSidebarNavGroupsByRole(navGroups, ROLE.ADMIN)
    expect(adminGroups[1]?.items.map((item) => item.url)).toEqual([
      '/d2s-admin',
    ])

    const superAdminGroups = filterSidebarNavGroupsByRole(
      navGroups,
      ROLE.SUPER_ADMIN
    )
    expect(superAdminGroups[1]?.items.map((item) => item.url)).toEqual([
      '/d2s-admin',
      '/system-info',
    ])
  })
})
