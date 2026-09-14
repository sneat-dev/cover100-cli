import { describe, expect, it } from 'vitest'

import { formatMoney } from './format'

describe('formatMoney', () => {
  it('renders two decimal places', () => {
    expect(formatMoney(3.5)).toBe('$3.50')
  })

  it('renders whole numbers with cents', () => {
    expect(formatMoney(12)).toBe('$12.00')
  })
})
