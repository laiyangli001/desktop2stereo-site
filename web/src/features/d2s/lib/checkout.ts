export function submitD2SCheckout(
  checkout: {
    checkout_url: string
    method?: 'GET' | 'POST'
    params?: Record<string, string>
    display_mode?: 'iframe'
    qr_pay_mode?: string
  },
  labels: { title: string; close: string; fallback: string; help: string }
) {
  if (
    checkout.display_mode === 'iframe' &&
    checkout.method === 'POST' &&
    checkout.params
  ) {
    const frameName = `d2s-alipay-${globalThis.crypto.randomUUID()}`
    const overlay = document.createElement('div')
    overlay.className =
      'fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4'
    overlay.setAttribute('role', 'dialog')
    overlay.setAttribute('aria-modal', 'true')
    overlay.setAttribute('aria-label', labels.title)

    const panel = document.createElement('section')
    panel.className =
      'flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl bg-background p-4 shadow-xl'
    const header = document.createElement('div')
    header.className = 'mb-3 flex items-center justify-between gap-3'
    const title = document.createElement('h2')
    title.className = 'text-lg font-semibold'
    title.textContent = labels.title
    const help = document.createElement('p')
    help.className = 'mb-3 text-sm text-muted-foreground'
    help.textContent = labels.help
    const actions = document.createElement('div')
    actions.className = 'flex items-center gap-2'
    const fallback = document.createElement('button')
    fallback.type = 'button'
    fallback.className = 'rounded-md border px-3 py-2 text-sm'
    fallback.textContent = labels.fallback
    const close = document.createElement('button')
    close.type = 'button'
    close.className = 'rounded-md border px-3 py-2 text-sm'
    close.textContent = labels.close
    const frame = document.createElement('iframe')
    frame.name = frameName
    frame.title = labels.title
    let frameMinHeight = 'min-h-[300px]'
    if (checkout.qr_pay_mode === '1') frameMinHeight = 'min-h-[600px]'
    if (checkout.qr_pay_mode === '3') frameMinHeight = 'min-h-[160px]'
    frame.className = `w-full flex-1 rounded-lg border bg-white ${frameMinHeight}`
    frame.setAttribute(
      'sandbox',
      'allow-forms allow-scripts allow-same-origin allow-popups allow-top-navigation-by-user-activation'
    )
    frame.setAttribute('allow', 'payment')
    const form = document.createElement('form')
    form.method = 'POST'
    form.action = checkout.checkout_url
    form.target = frameName
    form.hidden = true
    for (const [name, value] of Object.entries(checkout.params)) {
      const input = document.createElement('input')
      input.type = 'hidden'
      input.name = name
      input.value = value
      form.appendChild(input)
    }
    fallback.addEventListener('click', () => {
      const fallbackForm = form.cloneNode(true) as HTMLFormElement
      fallbackForm.target = '_blank'
      document.body.appendChild(fallbackForm)
      fallbackForm.submit()
      fallbackForm.remove()
    })
    close.addEventListener('click', () => {
      overlay.remove()
      form.remove()
    })
    actions.append(fallback, close)
    header.append(title, actions)
    panel.append(header, help, frame)
    overlay.appendChild(panel)
    document.body.append(overlay, form)
    form.submit()
    return
  }
  if (checkout.method === 'POST' && checkout.params) {
    const form = document.createElement('form')
    form.method = 'POST'
    form.action = checkout.checkout_url
    form.style.display = 'none'
    for (const [name, value] of Object.entries(checkout.params)) {
      const input = document.createElement('input')
      input.type = 'hidden'
      input.name = name
      input.value = value
      form.appendChild(input)
    }
    document.body.appendChild(form)
    form.submit()
    return
  }
  window.location.assign(checkout.checkout_url)
}
