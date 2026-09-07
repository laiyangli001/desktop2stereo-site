import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { LicenseCard } from '..'
import type { D2SLicense } from '../types'

const activeBoundLicense: D2SLicense = {
  id: 'license-1',
  license_code: 'D2S-TEST-0001',
  kind: 'paid',
  status: 'active',
  mode: 'online',
  device_hash: 'a'.repeat(64),
  fingerprint_version: 1,
  offline_period_days: 7,
  offline_valid_until: 0,
  expires_at: 0,
}

describe('LicenseCard', () => {
  it('submits the selected offline period when switching mode', () => {
    const onChangeMode = vi.fn()
    render(
      <LicenseCard
        license={activeBoundLicense}
        actionPending={false}
        onChangeMode={onChangeMode}
        onFreeRevoke={vi.fn()}
        onManualUnbind={vi.fn()}
      />
    )

    fireEvent.change(screen.getByRole('combobox', { name: 'Offline period' }), {
      target: { value: '30' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Offline mode' }))

    expect(onChangeMode).toHaveBeenCalledWith('offline', 30)
  })

  it('does not expose device actions for an unbound license', () => {
    render(
      <LicenseCard
        license={{ ...activeBoundLicense, device_hash: undefined }}
        actionPending={false}
        onChangeMode={vi.fn()}
        onFreeRevoke={vi.fn()}
        onManualUnbind={vi.fn()}
      />
    )

    expect(screen.queryByRole('button', { name: 'Offline mode' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Free revoke' })).toBeNull()
  })

  it('exposes a reason field for permanent-license manual unbind', () => {
    const onManualUnbind = vi.fn()
    render(
      <LicenseCard
        license={{ ...activeBoundLicense, mode: 'permanent' }}
        actionPending={false}
        onChangeMode={vi.fn()}
        onFreeRevoke={vi.fn()}
        onManualUnbind={onManualUnbind}
      />
    )

    fireEvent.change(
      screen.getByRole('textbox', { name: 'Manual unbind reason' }),
      {
        target: { value: 'Hardware replacement' },
      }
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Request manual unbind' })
    )

    expect(onManualUnbind).toHaveBeenCalledWith('Hardware replacement')
  })

  it('disables duplicate manual-unbind requests while one is pending', () => {
    render(
      <LicenseCard
        license={{ ...activeBoundLicense, mode: 'permanent' }}
        actionPending={false}
        manualUnbindStatus='pending'
        onChangeMode={vi.fn()}
        onFreeRevoke={vi.fn()}
        onManualUnbind={vi.fn()}
      />
    )

    expect(screen.getByText('Manual unbind status:')).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Request manual unbind' })
    ).toBeDisabled()
  })
})
