import { createFileRoute, redirect } from '@tanstack/react-router'

import { canAccessD2SAdmin } from '@/features/d2s/access'
import { D2SAdminWorkspace } from '@/features/d2s/admin'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/d2s-admin/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!canAccessD2SAdmin(auth.user)) {
      throw redirect({ to: '/403' })
    }
  },
  component: D2SAdminWorkspace,
})
