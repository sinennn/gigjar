const TRON_ADDRESS = /^T[1-9A-HJ-NP-Za-km-z]{33}$/
const DIGITS = /^[1-9][0-9]{0,18}$/

export class PolicyError extends Error {
  constructor (message) {
    super(message)
    this.name = 'PolicyError'
  }
}

export function requireAddress (value, field, isValid) {
  if (typeof value !== 'string' || !TRON_ADDRESS.test(value) || !isValid(value)) {
    throw new PolicyError(`${field} must be a valid TRON address`)
  }
  return value
}

export function requireIndex (value, field) {
  if (!Number.isSafeInteger(value) || value < 1 || value > 2_000_000_000) {
    throw new PolicyError(`${field} must be a jar derivation index of at least 1`)
  }
  return value
}

export function requireAmount (value, field, maximum) {
  if (typeof value !== 'string' || !DIGITS.test(value)) {
    throw new PolicyError(`${field} must be a positive integer string in base units`)
  }
  const amount = BigInt(value)
  if (maximum !== undefined && amount > maximum) {
    throw new PolicyError(`${field} exceeds the configured maximum of ${maximum}`)
  }
  return amount
}

export function validateUsdtTransfer (body, { isValidAddress, maxFeeLimitSun, forbiddenRecipients }) {
  const fromIndex = requireIndex(body.from_index, 'from_index')
  const to = requireAddress(body.to, 'to', isValidAddress)
  if (forbiddenRecipients.has(to)) {
    throw new PolicyError('to must not be a token contract or the treasury')
  }
  const amount = requireAmount(body.amount, 'amount')
  const feeLimitSun = requireAmount(String(body.fee_limit_sun ?? ''), 'fee_limit_sun', maxFeeLimitSun)
  return { fromIndex, to, amount, feeLimitSun }
}

export function validateTrxTransfer (body, { isValidAddress, maxTrxSun, forbiddenRecipients }) {
  const to = requireAddress(body.to, 'to', isValidAddress)
  if (forbiddenRecipients.has(to)) {
    throw new PolicyError('to must not be a token contract or the treasury')
  }
  const amountSun = requireAmount(String(body.amount_sun ?? ''), 'amount_sun', maxTrxSun)
  if (typeof body.purpose !== 'string' || !/^[a-z_]{1,40}$/.test(body.purpose)) {
    throw new PolicyError('purpose must be a short lowercase label')
  }
  return { to, amountSun, purpose: body.purpose }
}
