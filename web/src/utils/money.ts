/**
 * Formats integer minor units (paise) into an Indian Rupee string (e.g. ₹1,00,000.00).
 * All financial math is strictly preserved in integer paise.
 */
export function formatINR(minor: number | null | undefined): string {
  if (minor === null || minor === undefined) {
    return '₹0.00';
  }

  const isNegative = minor < 0;
  const absMinor = Math.abs(minor);
  const rupees = Math.floor(absMinor / 100);
  const paise = absMinor % 100;

  // Format integer rupees with Indian number format (e.g. 1,00,000)
  const rupeesStr = rupees.toLocaleString('en-IN');
  const paiseStr = paise.toString().padStart(2, '0');

  const formatted = `₹${rupeesStr}.${paiseStr}`;
  return isNegative ? `-${formatted}` : formatted;
}

/**
 * Parses user input string (e.g. "1,250.50" or "₹500") into integer minor paise.
 */
export function parseMinor(input: string): number {
  if (!input) return 0;
  const clean = input.replace(/[₹,\s]/g, '');
  const parsed = parseFloat(clean);
  if (isNaN(parsed)) return 0;
  return Math.round(parsed * 100);
}

