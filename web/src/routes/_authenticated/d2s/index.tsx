import { createFileRoute } from '@tanstack/react-router'

import { D2SWorkspace } from '@/features/d2s'

export const Route = createFileRoute('/_authenticated/d2s/')({
  component: D2SWorkspace,
})
