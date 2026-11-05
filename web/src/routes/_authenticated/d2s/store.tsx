import { createFileRoute } from '@tanstack/react-router'

import { D2SStore } from '@/features/d2s/store'

export const Route = createFileRoute('/_authenticated/d2s/store')({
  component: D2SStore,
})
