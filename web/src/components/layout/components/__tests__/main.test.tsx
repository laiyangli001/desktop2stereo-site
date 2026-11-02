import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Main } from '../main'

describe('Main', () => {
  it('does not add a nested main landmark inside the authenticated shell', () => {
    const { container } = render(
      <main>
        <Main data-testid='content-layout'>Page content</Main>
      </main>
    )

    expect(container.querySelectorAll('main')).toHaveLength(1)
    expect(screen.getByTestId('content-layout').tagName).toBe('DIV')
  })
})
