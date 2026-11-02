import { createFileRoute } from '@tanstack/react-router'

import { D2SMySupportTickets } from '@/features/d2s/my-support-tickets'

export const Route = createFileRoute('/_authenticated/d2s/support-tickets')({
  component: D2SMySupportTickets,
})
