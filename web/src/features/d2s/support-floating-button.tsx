import { Link, useRouterState } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

interface FloatingButtonPosition {
  left: number
  top: number
}

interface PointerDrag {
  pointerId: number
  startX: number
  startY: number
  startLeft: number
  startTop: number
  moved: boolean
}

const positionStorageKey = 'd2s-support-floating-button-position-v1'
const viewportMargin = 8
const dragThreshold = 5

function clampPosition(
  position: FloatingButtonPosition,
  width: number,
  height: number
): FloatingButtonPosition {
  const button = document.querySelector<HTMLElement>(
    '[data-support-floating-button]'
  )
  const buttonWidth = button?.offsetWidth ?? 180
  const buttonHeight = button?.offsetHeight ?? 48

  return {
    left: Math.min(
      Math.max(viewportMargin, position.left),
      Math.max(viewportMargin, width - buttonWidth - viewportMargin)
    ),
    top: Math.min(
      Math.max(viewportMargin, position.top),
      Math.max(viewportMargin, height - buttonHeight - viewportMargin)
    ),
  }
}

function getSavedPosition(): FloatingButtonPosition | null {
  if (typeof window === 'undefined') return null

  try {
    const saved = window.localStorage.getItem(positionStorageKey)
    if (!saved) return null
    const parsed = JSON.parse(saved) as FloatingButtonPosition
    if (!Number.isFinite(parsed.left) || !Number.isFinite(parsed.top)) {
      return null
    }
    return clampPosition(parsed, window.innerWidth, window.innerHeight)
  } catch {
    return null
  }
}

export function D2SSupportFloatingButton() {
  const { t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const [openPath, setOpenPath] = useState<string | null>(null)
  const open = openPath === pathname
  const [wechatURL, setWechatURL] = useState('')
  const [position, setPosition] = useState<FloatingButtonPosition | null>(
    getSavedPosition
  )
  const buttonRef = useRef<HTMLButtonElement>(null)
  const dragRef = useRef<PointerDrag | null>(null)
  const suppressClickRef = useRef(false)

  useEffect(() => {
    const keepPositionInViewport = () => {
      setPosition((current) =>
        current
          ? clampPosition(current, window.innerWidth, window.innerHeight)
          : current
      )
    }
    window.addEventListener('resize', keepPositionInViewport)
    return () => window.removeEventListener('resize', keepPositionInViewport)
  }, [])

  useEffect(() => {
    let active = true
    fetch('/api/v1/support/config', { cache: 'no-store' })
      .then((response) => response.json())
      .then((result: { data?: { enterprise_wechat_url?: string } }) => {
        if (active && typeof result.data?.enterprise_wechat_url === 'string') {
          setWechatURL(result.data.enterprise_wechat_url)
        }
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [])

  if (pathname.startsWith('/d2s-admin-console')) return null

  const viewportWidth = typeof window === 'undefined' ? 1024 : window.innerWidth
  const viewportHeight =
    typeof window === 'undefined' ? 768 : window.innerHeight
  const buttonTop =
    position?.top ?? Math.max(viewportMargin, (viewportHeight - 48) / 2)
  const panelOpensDown = buttonTop < viewportHeight / 2
  const panelAlignsLeft = position ? position.left < viewportWidth / 2 : false
  const panelMaxHeight = Math.max(
    0,
    panelOpensDown ? viewportHeight - buttonTop - 68 : buttonTop - 20
  )

  const savePosition = (nextPosition: FloatingButtonPosition) => {
    setPosition(nextPosition)
    try {
      window.localStorage.setItem(
        positionStorageKey,
        JSON.stringify(nextPosition)
      )
    } catch {
      // Keep the current position for this page even when storage is unavailable.
    }
  }

  const handlePointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return
    const bounds = event.currentTarget.getBoundingClientRect()
    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      startLeft: bounds.left,
      startTop: bounds.top,
      moved: false,
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
  }

  const handlePointerMove = (event: React.PointerEvent<HTMLButtonElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return

    const deltaX = event.clientX - drag.startX
    const deltaY = event.clientY - drag.startY
    if (!drag.moved && Math.hypot(deltaX, deltaY) < dragThreshold) return

    drag.moved = true
    suppressClickRef.current = true
    setPosition(
      clampPosition(
        { left: drag.startLeft + deltaX, top: drag.startTop + deltaY },
        window.innerWidth,
        window.innerHeight
      )
    )
  }

  const handlePointerUp = (event: React.PointerEvent<HTMLButtonElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return

    if (drag.moved) {
      savePosition(
        clampPosition(
          {
            left: drag.startLeft + event.clientX - drag.startX,
            top: drag.startTop + event.clientY - drag.startY,
          },
          window.innerWidth,
          window.innerHeight
        )
      )
    }
    dragRef.current = null
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    if (drag.moved) {
      window.setTimeout(() => {
        suppressClickRef.current = false
      }, 0)
    }
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    const directions: Record<string, FloatingButtonPosition> = {
      ArrowDown: { left: 0, top: 12 },
      ArrowLeft: { left: -12, top: 0 },
      ArrowRight: { left: 12, top: 0 },
      ArrowUp: { left: 0, top: -12 },
    }
    const direction = directions[event.key]
    if (!direction) return

    event.preventDefault()
    const bounds = buttonRef.current?.getBoundingClientRect()
    if (!bounds) return
    savePosition(
      clampPosition(
        {
          left: bounds.left + direction.left,
          top: bounds.top + direction.top,
        },
        window.innerWidth,
        window.innerHeight
      )
    )
  }

  return (
    <aside
      className={
        position ? 'fixed z-50' : 'fixed top-1/2 right-4 z-50 -translate-y-1/2'
      }
      data-support-floating-button-container
      style={position ? { left: position.left, top: position.top } : undefined}
    >
      {open && (
        <section
          aria-label={t('Support contact options')}
          className={`bg-background absolute z-10 w-64 max-w-[calc(100vw-1rem)] space-y-3 overflow-y-auto rounded-xl border p-4 shadow-xl ${
            panelOpensDown ? 'top-full mt-3' : 'bottom-full mb-3'
          } ${panelAlignsLeft ? 'left-0' : 'right-0'}`}
          style={{ maxHeight: panelMaxHeight }}
        >
          <h2 className='font-semibold'>{t('How can we help?')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Sign in with a registered account to submit a ticket. Enterprise WeChat is also available.'
            )}
          </p>
          <Link
            className='hover:bg-muted block rounded-md border px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-offset-2'
            to='/support'
          >
            {t('Submit a complaint or question')}
          </Link>
          {wechatURL ? (
            <a
              className='bg-primary text-primary-foreground block rounded-md px-3 py-2 text-center text-sm hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2'
              href={wechatURL}
              rel='noopener noreferrer'
              target='_blank'
            >
              {t('Add enterprise WeChat support')}
            </a>
          ) : (
            <p className='text-muted-foreground text-xs'>
              {t('Enterprise WeChat support link is not configured yet.')}
            </p>
          )}
        </section>
      )}
      <button
        aria-expanded={open}
        aria-keyshortcuts='ArrowUp ArrowDown ArrowLeft ArrowRight'
        aria-label={
          open ? t('Close support options') : t('Complaints and support')
        }
        className='bg-primary text-primary-foreground ml-auto flex min-h-12 cursor-grab touch-none items-center justify-center rounded-full px-4 text-sm font-medium shadow-lg hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 active:cursor-grabbing'
        data-support-floating-button
        onClick={() => {
          if (suppressClickRef.current) {
            suppressClickRef.current = false
            return
          }
          setOpenPath((current) => (current === pathname ? null : pathname))
        }}
        onKeyDown={handleKeyDown}
        onPointerCancel={handlePointerUp}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        ref={buttonRef}
        type='button'
      >
        {t('Complaints and support')}
      </button>
    </aside>
  )
}
