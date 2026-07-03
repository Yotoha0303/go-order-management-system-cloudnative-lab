import { ORDER_STATUS } from './format'

export type OrderAction = 'pay' | 'finish' | 'cancel'

export function isOrderActionAllowed(status: number, action: OrderAction) {
  if (action === 'pay' || action === 'cancel') {
    return status === ORDER_STATUS.PENDING
  }
  return status === ORDER_STATUS.PAID
}

export function requiresOrderActionConfirmation(action: OrderAction) {
  return action === 'finish' || action === 'cancel'
}
