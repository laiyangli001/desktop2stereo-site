/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BehaviorCaptcha, type BehaviorCaptchaValue } from '../behavior-captcha'

const apiMocks = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/lib/api', () => ({ api: apiMocks }))

const captcha = {
  id: 'captcha-id',
  master_image: 'data:image/png;base64,master',
  tile_image: 'data:image/png;base64,tile',
  width: 320,
  height: 160,
  tile_width: 48,
  tile_height: 48,
  tile_start_y: 40,
}

describe('BehaviorCaptcha', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.get.mockResolvedValue({ data: { success: true, data: captcha } })
  })

  it('loads a challenge and emits coordinates after dragging the tile', async () => {
    const onChange = vi.fn<(value?: BehaviorCaptchaValue) => void>()
    render(<BehaviorCaptcha onChange={onChange} />)

    const tile = await screen.findByRole('img', { name: '可拖动拼图' })
    fireEvent.pointerDown(tile, { pointerId: 1, clientX: 20 })
    fireEvent.pointerMove(tile, { pointerId: 1, clientX: 105 })
    fireEvent.pointerUp(tile, { pointerId: 1, clientX: 105 })

    expect(onChange).toHaveBeenLastCalledWith({
      captcha_id: 'captcha-id',
      captcha_x: 85,
      captcha_y: 40,
    })
  })

  it('clears the previous value and requests a new challenge when refreshed', async () => {
    const onChange = vi.fn<(value?: BehaviorCaptchaValue) => void>()
    render(<BehaviorCaptcha value={{ captcha_id: 'old', captcha_x: 1, captcha_y: 2 }} onChange={onChange} />)

    await screen.findByRole('img', { name: '可拖动拼图' })
    fireEvent.click(screen.getByRole('button', { name: '刷新验证码' }))

    await waitFor(() => expect(apiMocks.get).toHaveBeenCalledTimes(2))
    expect(onChange).toHaveBeenCalledWith(undefined)
  })
})
