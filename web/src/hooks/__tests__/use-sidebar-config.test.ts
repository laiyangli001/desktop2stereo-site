import { describe, expect, it } from 'vitest'

import type { NavGroup } from '@/components/layout/types'

import {
  filterSidebarNavGroupsByConfig,
  parseSidebarConfig,
} from '../use-sidebar-config'

const groups: NavGroup[] = [
  {
    id: 'chat',
    title: 'Chat',
    items: [
      { title: 'Playground', url: '/playground' },
      { title: 'Chat', type: 'chat-presets' },
    ],
  },
  {
    id: 'general',
    title: 'General',
    items: [
      { title: 'Dashboard', url: '/dashboard/models' },
      { title: 'API Keys', url: '/keys' },
      {
        title: 'Task Logs',
        url: '/usage-logs/task',
        configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
      },
    ],
  },
]

describe('sidebar admin configuration', () => {
  it('hides disabled sections and modules for ordinary users', () => {
    const config = parseSidebarConfig(
      JSON.stringify({
        chat: { enabled: false, playground: true, chat: true },
        console: {
          enabled: true,
          detail: false,
          token: true,
          log: false,
          midjourney: false,
          task: false,
        },
      })
    )

    const filtered = filterSidebarNavGroupsByConfig(groups, config)

    expect(filtered).toHaveLength(1)
    expect(filtered[0]?.items.map((item) => item.title)).toEqual(['API Keys'])
  })

  it('accepts an already decoded status object', () => {
    const config = parseSidebarConfig({
      chat: { enabled: false },
      console: { enabled: true, token: false, midjourney: false, task: false },
    })

    const filtered = filterSidebarNavGroupsByConfig(groups, config)
    expect(filtered[0]?.items.map((item) => item.title)).toEqual(['Dashboard'])
  })
})
