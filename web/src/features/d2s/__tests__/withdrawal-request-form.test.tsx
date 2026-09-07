import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { WithdrawalRequestForm } from '../components/withdrawal-request-form'

const apiMocks = vi.hoisted(() => ({
  createD2SWithdrawal: vi.fn(),
}))

vi.mock('../api', () => apiMocks)

describe('WithdrawalRequestForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.createD2SWithdrawal.mockResolvedValue({ success: true, data: {} })
  })

  it('disables submission when the available CNY balance is below the minimum', () => {
    render(<WithdrawalRequestForm availableMinor={4999} onSuccess={vi.fn()} />)

    expect(
      screen.getByRole('button', { name: 'Request withdrawal' })
    ).toBeDisabled()
  })

  it('submits the validated payout fields and refreshes after success', async () => {
    const onSuccess = vi.fn()
    render(
      <WithdrawalRequestForm availableMinor={8000} onSuccess={onSuccess} />
    )

    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Amount (minor units)' }),
      {
        target: { value: '6000' },
      }
    )
    fireEvent.change(screen.getByRole('textbox', { name: 'Alipay account' }), {
      target: { value: 'user@example.com' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: 'Real name' }), {
      target: { value: 'Test User' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Request withdrawal' }))

    await waitFor(() => {
      expect(apiMocks.createD2SWithdrawal).toHaveBeenCalledWith({
        amount_minor: 6000,
        alipay_account: 'user@example.com',
        real_name: 'Test User',
      })
    })
    expect(onSuccess).toHaveBeenCalledOnce()
  })
})
