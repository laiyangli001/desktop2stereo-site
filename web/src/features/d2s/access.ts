import { ROLE } from '@/lib/roles'

export const D2S_ADMIN_ROLES = [ROLE.ADMIN, ROLE.SUPER_ADMIN] as const

export function canAccessD2SAdmin(user: { role?: number } | null | undefined) {
  return Boolean(
    user &&
    D2S_ADMIN_ROLES.includes(user.role as (typeof D2S_ADMIN_ROLES)[number])
  )
}
