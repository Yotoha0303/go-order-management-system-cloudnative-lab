import type { CreateOrderPayload } from './types'

export type PendingOrderSubmission = {
  fingerprint: string
  idempotencyKey: string
}

function fingerprintOrder(payload: CreateOrderPayload) {
  return JSON.stringify(payload.items)
}

export function prepareOrderSubmission(
  payload: CreateOrderPayload,
  previous?: PendingOrderSubmission | null,
  createKey: () => string = () => crypto.randomUUID()
): PendingOrderSubmission {
  const fingerprint = fingerprintOrder(payload)
  if (previous?.fingerprint === fingerprint) return previous

  return {
    fingerprint,
    idempotencyKey: createKey(),
  }
}
