import { describe, expect, it } from 'vitest'
import { parseYuanToFen } from './money'

describe('parseYuanToFen', () => {
  it.each([
    ['1', 100],
    ['1.2', 120],
    [' 19.99 ', 1999],
  ])('converts %s to integer fen', (input, expected) => {
    expect(parseYuanToFen(input)).toBe(expected)
  })

  it.each(['', '0', '-1', '1.999', '1e2', 'abc'])('rejects %s', (input) => {
    expect(parseYuanToFen(input)).toBeNull()
  })
})
