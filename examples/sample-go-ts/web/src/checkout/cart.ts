/** A line item in a cart. */
export interface CartItem {
  sku: string
  quantity: number
  unitPrice: number
}

/**
 * A shopping cart.
 *
 * `addItem` and `total` are exercised by cart.test.ts; `removeItem` and
 * `discount` are deliberately not, so the example covers a partial range.
 */
export class Cart {
  private readonly items: CartItem[] = []

  addItem(item: CartItem): void {
    const existing = this.items.find((i) => i.sku === item.sku)
    if (existing) {
      existing.quantity += item.quantity
      return
    }
    this.items.push({ ...item })
  }

  removeItem(sku: string): boolean {
    const index = this.items.findIndex((i) => i.sku === sku)
    if (index < 0) {
      return false
    }
    this.items.splice(index, 1)
    return true
  }

  total(): number {
    return this.items.reduce((sum, item) => sum + item.quantity * item.unitPrice, 0)
  }

  discount(percent: number): number {
    if (percent <= 0 || percent >= 100) {
      throw new RangeError(`discount out of range: ${percent}`)
    }
    return this.total() * (1 - percent / 100)
  }
}
