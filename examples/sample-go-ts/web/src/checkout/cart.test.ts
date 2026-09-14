import { describe, expect, it } from 'vitest'

import { Cart } from './cart'

describe('Cart', () => {
  it('starts empty', () => {
    expect(new Cart().total()).toBe(0)
  })

  it('adds items and sums their totals', () => {
    const cart = new Cart()
    cart.addItem({ sku: 'tea', quantity: 2, unitPrice: 3.5 })
    cart.addItem({ sku: 'mug', quantity: 1, unitPrice: 12 })
    expect(cart.total()).toBe(19)
  })

  it('merges a repeated sku instead of duplicating it', () => {
    const cart = new Cart()
    cart.addItem({ sku: 'tea', quantity: 1, unitPrice: 3.5 })
    cart.addItem({ sku: 'tea', quantity: 3, unitPrice: 3.5 })
    expect(cart.total()).toBe(14)
  })
})
