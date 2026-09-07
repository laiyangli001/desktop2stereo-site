import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { D2SDeviceApprovalPage } from '../device'

const apiMocks = vi.hoisted(() => ({
  approveD2SDevice: vi.fn(),
}))

vi.mock('@/features/d2s/api', () => apiMocks)

describe('D2SDeviceApprovalPage', () => {
  it('exposes an accessible user-code field and announces approval', async () => {
    apiMocks.approveD2SDevice.mockResolvedValue({
      success: true,
      data: { approved: true },
    })

    render(<D2SDeviceApprovalPage />)

    const input = screen.getByRole('textbox', { name: 'User code' })
    expect(input).toHaveAttribute('autocomplete', 'one-time-code')
    expect(input).toHaveAttribute('pattern', '[A-Za-z0-9]{4}-[A-Za-z0-9]{4}')

    fireEvent.change(input, { target: { value: 'abcd-efgh' } })
    fireEvent.submit(input.closest('form') as HTMLFormElement)

    await waitFor(() => {
      expect(apiMocks.approveD2SDevice).toHaveBeenCalledWith('ABCD-EFGH')
    })
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Device approved. You can return to the launcher.'
    )
  })

  it('keeps the approval form available when the server rejects the code', async () => {
    apiMocks.approveD2SDevice.mockResolvedValue({
      success: false,
      error: { message: 'invalid_device_code' },
    })

    render(<D2SDeviceApprovalPage />)
    const input = screen.getByRole('textbox', { name: 'User code' })
    fireEvent.change(input, { target: { value: 'ABCD-EFGH' } })
    fireEvent.submit(input.closest('form') as HTMLFormElement)

    await waitFor(() => expect(apiMocks.approveD2SDevice).toHaveBeenCalled())
    expect(
      screen.getByRole('textbox', { name: 'User code' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})
