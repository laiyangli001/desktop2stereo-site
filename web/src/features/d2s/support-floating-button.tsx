import { Link, useRouterState } from '@tanstack/react-router'
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from 'react'
import { useTranslation } from 'react-i18next'

const SUPPORT_POSITION_KEY = 'd2s-support-floating-position'
const DEFAULT_POSITION = { right: 16, bottom: 96 }

type Position = { right: number; bottom: number }

function clampPosition(
  position: Position,
  maxRight: number,
  maxBottom: number
) {
  return {
    right: Math.max(0, Math.min(position.right, maxRight)),
    bottom: Math.max(0, Math.min(position.bottom, maxBottom)),
  }
}

function readStoredPosition(): Position {
  if (typeof window === 'undefined') return DEFAULT_POSITION
  try {
    const stored = localStorage.getItem(SUPPORT_POSITION_KEY)
    if (stored) {
      const parsed: unknown = JSON.parse(stored)
      if (typeof parsed === 'object' && parsed !== null) {
        const candidate = parsed as Record<string, unknown>
        const right = candidate.right
        const bottom = candidate.bottom
        if (
          typeof right === 'number' &&
          Number.isFinite(right) &&
          typeof bottom === 'number' &&
          Number.isFinite(bottom)
        ) {
          return clampPosition(
            { right, bottom },
            window.innerWidth,
            window.innerHeight
          )
        }
      }
    }
  } catch {
    // Ignore unavailable or invalid local position data.
  }
  return DEFAULT_POSITION
}

export function D2SSupportFloatingButton() {
  const { t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const [openPath, setOpenPath] = useState<string | null>(null)
  const open = openPath === pathname
  const [wechatURL, setWechatURL] = useState('')
  const [position, setPosition] = useState<Position>(readStoredPosition)
  const positionRef = useRef(position)
  const containerRef = useRef<HTMLElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const dragStart = useRef<{
    pointerId: number
    x: number
    y: number
    position: Position
    moved: boolean
  } | null>(null)
  const suppressClick = useRef(false)

  const updatePosition = useCallback((next: Position) => {
    positionRef.current = next
    setPosition(next)
  }, [])

  const clampToViewport = useCallback((next: Position) => {
    const container = containerRef.current
    const button = buttonRef.current
    const maxRight = Math.max(
      0,
      window.innerWidth - (container?.offsetWidth ?? button?.offsetWidth ?? 0)
    )
    const maxBottom = Math.max(
      0,
      window.innerHeight - (button?.offsetHeight ?? 0)
    )
    return clampPosition(next, maxRight, maxBottom)
  }, [])

  const savePosition = useCallback((next: Position) => {
    try {
      localStorage.setItem(SUPPORT_POSITION_KEY, JSON.stringify(next))
    } catch {
      // Keep the control usable when browser storage is unavailable.
    }
  }, [])

  useEffect(() => {
    const keepInViewport = () =>
      updatePosition(clampToViewport(positionRef.current))
    window.addEventListener('resize', keepInViewport)
    return () => window.removeEventListener('resize', keepInViewport)
  }, [clampToViewport, updatePosition])

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

  const handlePointerDown = (event: PointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return
    dragStart.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      position: positionRef.current,
      moved: false,
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
  }

  const handlePointerMove = (event: PointerEvent<HTMLButtonElement>) => {
    const start = dragStart.current
    if (!start || start.pointerId !== event.pointerId) return
    const deltaX = event.clientX - start.x
    const deltaY = event.clientY - start.y
    if (!start.moved && Math.hypot(deltaX, deltaY) < 4) return
    start.moved = true
    updatePosition(
      clampToViewport({
        right: start.position.right - deltaX,
        bottom: start.position.bottom - deltaY,
      })
    )
  }

  const handlePointerUp = (event: PointerEvent<HTMLButtonElement>) => {
    const start = dragStart.current
    if (!start || start.pointerId !== event.pointerId) return
    if (start.moved) {
      suppressClick.current = true
      savePosition(positionRef.current)
    }
    dragStart.current = null
  }

  const handlePointerCancel = (event: PointerEvent<HTMLButtonElement>) => {
    const start = dragStart.current
    if (!start || start.pointerId !== event.pointerId) return
    updatePosition(start.position)
    dragStart.current = null
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (!event.altKey || !event.key.startsWith('Arrow')) return
    event.preventDefault()
    const next = { ...positionRef.current }
    if (event.key === 'ArrowLeft') next.right += 16
    if (event.key === 'ArrowRight') next.right -= 16
    if (event.key === 'ArrowUp') next.bottom -= 16
    if (event.key === 'ArrowDown') next.bottom += 16
    const clamped = clampToViewport(next)
    updatePosition(clamped)
    savePosition(clamped)
  }

  return (
    <aside
      ref={containerRef}
      className='fixed z-50'
      style={{ right: `${position.right}px`, bottom: `${position.bottom}px` }}
    >
      {open && (
        <section
          aria-label={t('Support contact options')}
          className='bg-background mb-3 max-h-[calc(100dvh-8rem)] w-64 space-y-3 overflow-y-auto rounded-xl border p-4 shadow-xl'
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
        ref={buttonRef}
        aria-expanded={open}
        aria-label={
          open ? t('Close support options') : t('Complaints and support')
        }
        className='bg-primary text-primary-foreground ml-auto flex min-h-12 cursor-grab touch-none items-center justify-center rounded-full px-4 text-sm font-medium shadow-lg select-none hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 active:cursor-grabbing'
        title={t('Drag to move; click to open support options')}
        onClick={(event) => {
          if (suppressClick.current) {
            suppressClick.current = false
            event.preventDefault()
            return
          }
          setOpenPath((current) => (current === pathname ? null : pathname))
        }}
        onKeyDown={handleKeyDown}
        onPointerCancel={handlePointerCancel}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        type='button'
      >
        {t('Complaints and support')}
      </button>
    </aside>
  )
}
