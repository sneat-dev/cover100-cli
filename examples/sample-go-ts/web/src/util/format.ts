/**
 * Formatting helpers.
 *
 * `formatMoney` is tested; `formatPercent` and `truncate` are not.
 */

/** Formats a number as US dollars. */
export function formatMoney(amount: number): string {
  return `$${amount.toFixed(2)}`
}

/** Formats a fraction in 0..1 as a percentage. */
export function formatPercent(fraction: number): string {
  return `${(fraction * 100).toFixed(1)}%`
}

/** Truncates a string to `max` characters, appending an ellipsis. */
export function truncate(value: string, max: number): string {
  if (value.length <= max) {
    return value
  }
  return `${value.slice(0, max - 1)}\u2026`
}
