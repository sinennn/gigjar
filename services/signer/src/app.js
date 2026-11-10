import { timingSafeEqual } from 'node:crypto'
import { PolicyError, requireIndex, validateTrxTransfer, validateUsdtTransfer } from './policy.js'

const MAX_BODY_BYTES = 16 * 1024

function send (response, status, body) {
  const payload = JSON.stringify(body)
  response.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
  response.end(payload)
}

function authorized (request, token) {
  const header = request.headers.authorization ?? ''
  const expected = Buffer.from(`Bearer ${token}`)
  const actual = Buffer.from(header)
  return actual.length === expected.length && timingSafeEqual(actual, expected)
}

async function readJSON (request) {
  let size = 0
  const chunks = []
  for await (const chunk of request) {
    size += chunk.length
    if (size > MAX_BODY_BYTES) {
      throw new PolicyError('request body too large')
    }
    chunks.push(chunk)
  }
  try {
    const value = JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}')
    if (value === null || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error('not an object')
    }
    return value
  } catch {
    throw new PolicyError('request body must be a JSON object')
  }
}

export function createApp ({ signer, token, maxTrxSun, maxFeeLimitSun, log }) {
  if (typeof token !== 'string' || token.length < 32) {
    throw new Error('signer token must be at least 32 characters')
  }
  let forbidden
  async function forbiddenRecipients () {
    if (!forbidden) {
      forbidden = new Set([signer.contract, await signer.treasuryAddress(), 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t', 'TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf'])
    }
    return forbidden
  }

  return async function handle (request, response) {
    const url = new URL(request.url, 'http://signer')
    try {
      if (request.method === 'GET' && url.pathname === '/healthz') {
        return send(response, 200, { status: 'ok', network: signer.network })
      }
      if (!authorized(request, token)) {
        return send(response, 401, { error: 'unauthorized' })
      }
      if (request.method === 'GET' && url.pathname === '/v1/treasury') {
        return send(response, 200, { address: await signer.treasuryAddress() })
      }
      if (request.method !== 'POST') {
        return send(response, 404, { error: 'not_found' })
      }
      const body = await readJSON(request)
      switch (url.pathname) {
        case '/v1/addresses': {
          const index = requireIndex(body.index, 'index')
          return send(response, 200, { index, address: await signer.jarAddress(index) })
        }
        case '/v1/sign/usdt-transfer': {
          const intent = validateUsdtTransfer(body, { isValidAddress: signer.isValidAddress, maxFeeLimitSun, forbiddenRecipients: await forbiddenRecipients() })
          const signed = await signer.signUsdtTransfer(intent)
          log.info({ msg: 'signed usdt transfer', from_index: intent.fromIndex, to: intent.to, amount: intent.amount.toString(), tx_id: signed.tx_id })
          return send(response, 200, signed)
        }
        case '/v1/sign/trx-transfer': {
          const intent = validateTrxTransfer(body, { isValidAddress: signer.isValidAddress, maxTrxSun, forbiddenRecipients: await forbiddenRecipients() })
          const signed = await signer.signTrxTransfer(intent)
          log.info({ msg: 'signed trx transfer', to: intent.to, amount_sun: intent.amountSun.toString(), purpose: intent.purpose, tx_id: signed.tx_id })
          return send(response, 200, signed)
        }
        default:
          return send(response, 404, { error: 'not_found' })
      }
    } catch (error) {
      if (error instanceof PolicyError) {
        return send(response, 422, { error: 'policy_violation', message: error.message })
      }
      log.error({ msg: 'signer request failed', path: url.pathname, error: String(error?.message ?? error).slice(0, 300) })
      return send(response, 502, { error: 'signing_failed' })
    }
  }
}
