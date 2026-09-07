import { api } from '@/lib/api'

import type {
  D2SBalanceAccount,
  D2SInviteInfo,
  D2SLicense,
  D2SOrder,
  D2SResponse,
  D2SWithdrawal,
  D2SSigningKey,
  D2SUnbindRequest,
  D2SReconciliationReport,
} from './types'

export type D2SModeRequest = {
  license_id: string
  device_hash: string
  mode: 'online' | 'offline'
  offline_period_days: number
}

export async function getD2SAdminOrders(status = '') {
  const response = await api.get('/api/v1/admin/orders', { params: { status } })
  return response.data as D2SResponse<{ orders: D2SOrder[] }>
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

export async function getD2SCheckoutProviders() {
  const response = await api.get('/api/v1/orders/providers')
  return response.data as D2SResponse<{ providers: string[] }>
}

export type D2SOrderRequest = {
  product: 'license' | 'paid_revoke' | 'offline_extension'
  license_id?: string
  provider: string
  balance_minor: number
  idempotency_key?: string
}

export async function previewD2SOrder(
  request: Omit<D2SOrderRequest, 'balance_minor' | 'idempotency_key'>
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
  }>
}

export async function getD2SBalance(): Promise<
  D2SResponse<{ accounts: D2SBalanceAccount[] }>
> {
  const response = await api.get('/api/v1/balance/info')
  return response.data
}

export async function getD2SInvite(): Promise<D2SResponse<D2SInviteInfo>> {
  const response = await api.get('/api/v1/invite/info')
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
