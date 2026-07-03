import { describe, expect, it } from 'vitest'
import { ORDER_STATUS } from './format'
import {
  isOrderActionAllowed,
  requiresOrderActionConfirmation,
  type OrderAction,
} from './order-actions'

describe('order action rules', () => {
  it.each<[{ status: number; action: OrderAction; allowed: boolean }]>([
    [{ status: ORDER_STATUS.PENDING, action: 'pay', allowed: true }],
    [{ status: ORDER_STATUS.PENDING, action: 'cancel', allowed: true }],
    [{ status: ORDER_STATUS.PENDING, action: 'finish', allowed: false }],
    [{ status: ORDER_STATUS.PAID, action: 'finish', allowed: true }],
    [{ status: ORDER_STATUS.PAID, action: 'cancel', allowed: false }],
    [{ status: ORDER_STATUS.FINISHED, action: 'finish', allowed: false }],
  ])('$action for status $status', ({ status, action, allowed }) => {
    expect(isOrderActionAllowed(status, action)).toBe(allowed)
  })

  it('requires confirmation for inventory-affecting or terminal actions', () => {
    expect(requiresOrderActionConfirmation('pay')).toBe(false)
    expect(requiresOrderActionConfirmation('cancel')).toBe(true)
    expect(requiresOrderActionConfirmation('finish')).toBe(true)
  })
})
