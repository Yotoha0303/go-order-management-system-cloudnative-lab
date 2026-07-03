export function parseYuanToFen(value: string): number | null {
  const normalized = value.trim()
  if (!/^\d+(\.\d{1,2})?$/.test(normalized)) return null

  const [yuan, decimal = ''] = normalized.split('.')
  const fen = Number(yuan) * 100 + Number(decimal.padEnd(2, '0'))
  return Number.isSafeInteger(fen) && fen > 0 ? fen : null
}
