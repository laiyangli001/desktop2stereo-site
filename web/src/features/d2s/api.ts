import { api } from '@/lib/api'

import type {
  D2SBalanceAccount,
  D2SBalanceTransaction,
  D2SInviteInfo,
  D2SInviteReward,
  D2SLicense,
  D2SOrder,
  D2SPaymentEvent,
  D2SResponse,
  D2SWithdrawal,
  D2SSigningKey,
  D2SUnbindRequest,
  D2SReconciliationReport,
  D2SSupportTicket,
} from './types'

export type D2SDownloadTarget = 'release' | 'quark' | 'baidu'

export async function submitD2SSupportTicket(input: {
  category: string
  contact_email: string
  order_id: string
  subject: string
  message: string
  website: string
}) {
  const response = await api.post('/api/v1/support/tickets', input)
  return response.data as D2SResponse<{ ticket_id: string }>
}

export async function getD2SMySupportTickets(page = 1) {
  const response = await api.get('/api/v1/support/my-tickets', {
    params: { page, page_size: 50 },
  })
  return response.data as D2SResponse<{
    items: D2SSupportTicket[]
    total: number
    page: number
    page_size: number
  }>
}

export type D2SModeRequest = {
  license_id: string
  device_hash: string
  mode: 'online' | 'offline'
  offline_period_days: number
}

export async function trackD2SDownload(target: D2SDownloadTarget) {
  const response = await api.post(
    `/api/v1/downloads/${encodeURIComponent(target)}/track`
  )
  return response.data as D2SResponse<{
    target: D2SDownloadTarget
    logged: boolean
  }>
}

export async function getD2SAdminOrders(filters?: {
  status?: string
  user_id?: string
}) {
  const response = await api.get('/api/v1/admin/orders', {
    params: filters,
  })
  return response.data as D2SResponse<{ orders: D2SOrder[] }>
}

export async function getD2SAdminPaymentEvents(filters?: {
  provider?: string
  order_id?: string
  event_type?: string
}) {
  const response = await api.get('/api/v1/admin/payment-events', {
    params: filters,
  })
  return response.data as D2SResponse<{ events: D2SPaymentEvent[] }>
}

export type D2SAdminAlipayPCSettings = {
  qr_pay_mode: string
  config_source: 'admin' | 'environment'
  channel_enabled: boolean
  app_id: string
  seller_id: string
  gateway_url: string
  notify_url: string
  return_url: string
  merchant_private_key_configured: boolean
  alipay_public_key_configured: boolean
  ready: boolean
  updated_at: number
}

export async function getD2SAdminAlipayPCSettings() {
  const response = await api.get('/api/option/alipay/settings')
  return response.data as D2SResponse<D2SAdminAlipayPCSettings>
}

export async function setD2SAdminAlipayQRPayMode(qrPayMode: string) {
  const response = await api.put('/api/option/alipay/settings', {
    qr_pay_mode: qrPayMode,
  })
  return response.data as D2SResponse<D2SAdminAlipayPCSettings>
}

export async function saveD2SAdminAlipayPCConfig(input: {
  channel_enabled: boolean
  qr_pay_mode?: string
  app_id: string
  seller_id: string
  gateway_url: string
  notify_url: string
  return_url: string
  merchant_private_key?: string
  alipay_public_key?: string
}) {
  const response = await api.put('/api/option/alipay/config', input)
  return response.data as D2SResponse<D2SAdminAlipayPCSettings>
}

export async function useD2SAdminAlipayEnvironmentConfig() {
  const response = await api.put('/api/option/alipay/config', {
    use_environment_config: true,
  })
  return response.data as D2SResponse<D2SAdminAlipayPCSettings>
}

export async function getD2SAdminAlipayRefunds(orderID: string) {
  const response = await api.get(
    `/api/option/alipay/orders/${encodeURIComponent(orderID)}/refunds`
  )
  return response.data as D2SResponse<{
    refunds: Array<{
      id: string
      order_id: string
      out_request_no: string
      amount_minor: number
      reason: string
      status: string
      created_at: number
    }>
  }>
}

export async function createD2SAdminAlipayRefund(
  orderID: string,
  input: { amount_minor: number; reason: string; idempotency_key: string }
) {
  const response = await api.post(
    `/api/option/alipay/orders/${encodeURIComponent(orderID)}/refunds`,
    input
  )
  return response.data as D2SResponse<{
    status: string
    out_request_no: string
  }>
}

export async function reconcileD2SAdminAlipayRefund(outRequestNo: string) {
  const response = await api.post(
    `/api/option/alipay/refunds/${encodeURIComponent(outRequestNo)}/reconcile`
  )
  return response.data as D2SResponse<{ status: string }>
}

export async function closeD2SAdminAlipayOrder(orderID: string) {
  const response = await api.post(
    `/api/option/alipay/orders/${encodeURIComponent(orderID)}/close`
  )
  return response.data as D2SResponse<unknown>
}

export async function downloadD2SAdminAlipayBill(date: string) {
  const response = await api.get('/api/option/alipay/bill', {
    params: { date },
    responseType: 'blob',
  })
  return response.data as Blob
}

export async function getD2SAdminLicenses() {
  const response = await api.get('/api/v1/admin/licenses')
  return response.data as D2SResponse<{ licenses: D2SLicense[] }>
}

export async function getD2SAdminBalances(negative = true) {
  const response = await api.get('/api/v1/admin/balances', {
    params: { negative },
  })
  return response.data as D2SResponse<{ accounts: D2SBalanceAccount[] }>
}

export async function getD2SAdminWithdrawals() {
  const response = await api.get('/api/v1/admin/withdrawals')
  return response.data as D2SResponse<{ withdrawals: D2SWithdrawal[] }>
}

export type D2SWithdrawalPayoutDetails = {
  withdrawal_id: string
  alipay_account: string
  real_name: string
}

export async function getD2SAdminWithdrawalPayout(
  withdrawalID: string
): Promise<D2SResponse<D2SWithdrawalPayoutDetails>> {
  const response = await api.get(
    `/api/v1/admin/withdrawals/${encodeURIComponent(withdrawalID)}/payout`
  )
  return response.data
}

export async function getD2SAdminUnbindRequests() {
  const response = await api.get('/api/v1/admin/unbind-requests')
  return response.data as D2SResponse<{ requests: D2SUnbindRequest[] }>
}

export async function getD2SAdminSigningKeys() {
  const response = await api.get('/api/v1/admin/signing-keys')
  return response.data as D2SResponse<{ keys: D2SSigningKey[] }>
}

export async function retireD2SAdminSigningKey(keyID: string) {
  const response = await api.put(`/api/v1/admin/signing-keys/${keyID}`, {
    status: 'retired',
  })
  return response.data as D2SResponse<{ keys: D2SSigningKey[] }>
}

export async function getD2SAdminReconciliation() {
  const response = await api.get('/api/v1/admin/reconciliation')
  return response.data as D2SResponse<D2SReconciliationReport>
}

export async function reviewD2SWithdrawal(
  id: string,
  status: string,
  note: string
) {
  const response = await api.put(`/api/v1/admin/withdrawals/${id}`, {
    status,
    note,
  })
  return response.data as D2SResponse<unknown>
}

export async function reviewD2SUnbind(
  id: string,
  status: string,
  note: string
) {
  const response = await api.put(`/api/v1/admin/unbind-requests/${id}`, {
    status,
    note,
  })
  return response.data as D2SResponse<unknown>
}

export async function setD2SUserRegion(userID: number, region: string) {
  const response = await api.put(`/api/v1/admin/users/${userID}/region`, {
    region,
  })
  return response.data as D2SResponse<unknown>
}

export async function getD2SLicenses(): Promise<
  D2SResponse<{ licenses: D2SLicense[] }>
> {
  const response = await api.get('/api/v1/license/list')
  return response.data
}

export async function getD2SOrders(): Promise<
  D2SResponse<{ orders: D2SOrder[] }>
> {
  const response = await api.get('/api/v1/orders')
  return response.data
}

export type D2SStoreProduct = {
  id: 'license' | 'paid_revoke' | 'offline_extension'
  prices: Partial<Record<'CNY' | 'USD', number>>
  starting_at: boolean
}

export async function getD2SStoreProducts() {
  const response = await api.get('/api/v1/store/products')
  return response.data as D2SResponse<{ products: D2SStoreProduct[] }>
}

export async function getD2SCheckoutProviders() {
  const response = await api.get('/api/v1/orders/providers')
  return response.data as D2SResponse<{
    enabled: boolean
    providers: string[]
  }>
}

export type D2SOrderRequest = {
  product: 'license' | 'paid_revoke' | 'offline_extension'
  license_id?: string
  provider: string
  balance_minor: number
  expected_amount_minor: number
  expected_currency: string
  expected_region: string
  idempotency_key?: string
}

export async function previewD2SOrder(
  request: Omit<
    D2SOrderRequest,
    | 'balance_minor'
    | 'idempotency_key'
    | 'expected_amount_minor'
    | 'expected_currency'
    | 'expected_region'
  >
) {
  const response = await api.post('/api/v1/orders/preview', request)
  return response.data as D2SResponse<{
    product: string
    provider: string
    region: string
    currency: string
    amount_minor: number
  }>
}

export async function createD2SOrder(request: D2SOrderRequest) {
  const response = await api.post('/api/v1/orders/create', request)
  return response.data as D2SResponse<D2SOrder>
}

export async function createD2SOrderCheckout(orderID: string) {
  const response = await api.post(`/api/v1/orders/${orderID}/checkout`)
  return response.data as D2SResponse<{
    order_id: string
    checkout_url: string
    method?: 'GET' | 'POST'
    params?: Record<string, string>
    display_mode?: 'iframe'
    qr_pay_mode?: string
  }>
}

export async function getD2SBalance(): Promise<
  D2SResponse<{
    accounts: D2SBalanceAccount[]
    min_withdrawal_minor?: number
  }>
> {
  const response = await api.get('/api/v1/balance/info')
  return response.data
}

export async function getD2SBalanceTransactions(): Promise<
  D2SResponse<{ transactions: D2SBalanceTransaction[] }>
> {
  const response = await api.get('/api/v1/balance/transactions')
  return response.data
}

export async function getD2SInvite(): Promise<D2SResponse<D2SInviteInfo>> {
  const response = await api.get('/api/v1/invite/info')
  return response.data
}

export async function getD2SInviteRecords(): Promise<
  D2SResponse<{ records: D2SInviteReward[] }>
> {
  const response = await api.get('/api/v1/invite/records')
  return response.data
}

export async function getD2SWithdrawals(): Promise<
  D2SResponse<{ withdrawals: D2SWithdrawal[] }>
> {
  const response = await api.get('/api/v1/withdrawal/status')
  return response.data
}

export type D2SWithdrawalRequest = {
  amount_minor: number
  alipay_account: string
  real_name: string
}

export async function createD2SWithdrawal(
  request: D2SWithdrawalRequest
): Promise<D2SResponse<D2SWithdrawal>> {
  const response = await api.post('/api/v1/withdrawal/request', request)
  return response.data
}

export async function changeD2SMode(
  request: D2SModeRequest
): Promise<D2SResponse<unknown>> {
  const response = await api.post('/api/v1/license/change-mode', request)
  return response.data
}

export async function confirmD2SPermanent(request: {
  license_id: string
  device_hash: string
  confirmation: 'PERMANENT'
}): Promise<D2SResponse<unknown>> {
  const response = await api.post('/api/v1/license/permanent/confirm', request)
  return response.data
}

export async function freeRevokeD2SLicense(request: {
  license_id: string
  device_hash: string
  fingerprint_version: number
}): Promise<D2SResponse<unknown>> {
  const response = await api.post('/api/v1/license/revoke/free', request)
  return response.data
}

export async function createD2SManualUnbind(request: {
  license_id: string
  reason: string
  proof_ref?: string
}): Promise<D2SResponse<D2SUnbindRequest>> {
  const response = await api.post('/api/v1/license/manual-unbind', request)
  return response.data
}

export async function getD2SManualUnbindRequests() {
  const response = await api.get('/api/v1/license/manual-unbind')
  return response.data as D2SResponse<{ requests: D2SUnbindRequest[] }>
}

export async function approveD2SDevice(userCode: string) {
  const response = await api.post('/api/v1/device/approve', {
    user_code: userCode,
  })
  return response.data as D2SResponse<{
    approved: boolean
    user_code: string
    device_hash: string
    client_name: string
    platform: string
  }>
}
