const ipv4Segment = '(25[0-5]|2[0-4]\\d|1\\d{2}|[1-9]?\\d)'
const ipv4Pattern = new RegExp(`^${ipv4Segment}(\\.${ipv4Segment}){3}$`)

function stripHex(value: string): string {
  return value.replace(/[^a-fA-F0-9]/g, '').toUpperCase()
}

export function isIPv4(value: string): boolean {
  if (!value) return false
  return ipv4Pattern.test(value.trim())
}

export function isCidr(value: string): boolean {
  if (!value) return false
  const [ip, mask] = value.split('/')
  if (!ip || mask === undefined) return false
  if (!isIPv4(ip.trim())) return false
  const maskNumber = Number(mask)
  return Number.isInteger(maskNumber) && maskNumber >= 0 && maskNumber <= 32
}

export function isIpRange(value: string): boolean {
  if (!value.includes('-')) return false
  const [start, end] = value.split('-').map((part) => part.trim())
  if (!start || !end) return false
  return isIPv4(start) && isIPv4(end)
}

export function isValidMacPrefix(value: string): boolean {
  const hex = stripHex(value)
  return hex.length >= 2 && hex.length <= 12 && hex.length % 2 === 0
}

export function normalizeMacPrefix(value: string): string {
  const hex = stripHex(value).slice(0, 12)
  const pairs = hex.match(/.{1,2}/g) ?? []
  return pairs.join(':')
}

export function clampNumber(value: number, min: number, max: number): number {
  if (Number.isNaN(value)) return min
  if (value < min) return min
  if (value > max) return max
  return value
}

export function hasValue(value: unknown): boolean {
  if (value === null || value === undefined) return false
  if (typeof value === 'string') {
    return value.trim().length > 0
  }
  return true
}
