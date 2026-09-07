/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, beforeAll } from 'vitest'

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key: string) {
      return values.get(String(key)) ?? null
    },
    key(index: number) {
      return [...values.keys()][index] ?? null
    },
    removeItem(key: string) {
      values.delete(String(key))
    },
    setItem(key: string, value: string) {
      values.set(String(key), String(value))
    },
  }
}

function hasStorageMethods(value: unknown): value is Storage {
  if (!value || typeof value !== 'object') return false
  const storage = value as Partial<Storage>
  return (
    typeof storage.getItem === 'function' &&
    typeof storage.setItem === 'function' &&
    typeof storage.removeItem === 'function' &&
    typeof storage.clear === 'function' &&
    typeof storage.key === 'function'
  )
}

function isUsableStorage(storage: Storage): boolean {
  const probeKey = `__vitest_storage_probe__${Math.random().toString(36).slice(2)}`
  try {
    storage.setItem(probeKey, '1')
    storage.removeItem(probeKey)
    storage.clear()
    return true
  } catch {
    return false
  }
}

function installTestStorage(name: 'localStorage' | 'sessionStorage'): void {
  const globalOwner = globalThis as unknown as Record<string, unknown>
  const windowOwner = window as unknown as Record<string, unknown>
  let storage: Storage | undefined

  try {
    const candidate = windowOwner[name]
    if (hasStorageMethods(candidate) && isUsableStorage(candidate)) {
      storage = candidate
    }
  } catch {
    storage = undefined
  }

  storage ??= createMemoryStorage()
  Object.defineProperty(globalOwner, name, {
    configurable: true,
    value: storage,
  })
  Object.defineProperty(windowOwner, name, {
    configurable: true,
    value: storage,
  })
}

installTestStorage('localStorage')
installTestStorage('sessionStorage')

beforeAll(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    resources: {
      en: {
        translation: {},
      },
    },
  })
})

afterEach(() => {
  cleanup()
})

Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }),
})

window.requestAnimationFrame = (callback: FrameRequestCallback) =>
  window.setTimeout(() => callback(performance.now()), 0)
window.cancelAnimationFrame = (handle: number) => window.clearTimeout(handle)

class ResizeObserverMock {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

Object.defineProperty(globalThis, 'ResizeObserver', {
  configurable: true,
  value: ResizeObserverMock,
})

Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
  configurable: true,
  value: () => undefined,
})
