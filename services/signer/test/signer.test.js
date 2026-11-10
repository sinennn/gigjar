import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Readable } from 'node:stream'
import { createApp } from '../src/app.js'
import { createSigner } from '../src/signer.js'

const TEST_MNEMONIC = 'abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about'
const TOKEN = 'x'.repeat(40)
const JAR = 'TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA'

function fakeSigner () {
  const calls = []
  return {
    calls,
    network: 'tron-nile',
    contract: 'TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf',
    isValidAddress: (address) => address === JAR || address === 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t' || address === 'TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf' || address === 'TUmbnbtQcMJKtjbrXdqauaMMQBV2X5LDax',
    jarAddress: async (index) => `jar-${index}`,
    treasuryAddress: async () => 'TUmbnbtQcMJKtjbrXdqauaMMQBV2X5LDax',
    signUsdtTransfer: async (intent) => { calls.push(['usdt', intent]); return { tx_id: 'a'.repeat(64), signed_tx: {}, expires_at_ms: 1 } },
    signTrxTransfer: async (intent) => { calls.push(['trx', intent]); return { tx_id: 'b'.repeat(64), signed_tx: {}, expires_at_ms: 1 } }
  }
}

async function call (handle, method, path, body, token = TOKEN) {
  const request = Readable.from(body === undefined ? [] : [Buffer.from(JSON.stringify(body))])
  request.method = method
  request.url = path
  request.headers = token ? { authorization: `Bearer ${token}` } : {}
  let status
  let payload = ''
  const response = {
    writeHead: (code) => { status = code },
    end: (chunk) => { payload = chunk }
  }
  await handle(request, response)
  return { status, body: JSON.parse(payload) }
}

function app (signer, overrides = {}) {
  const logs = []
  const handle = createApp({ signer, token: TOKEN, maxTrxSun: 30_000_000n, maxFeeLimitSun: 30_000_000n, log: { info: (e) => logs.push(e), error: (e) => logs.push(e) }, ...overrides })
  handle.logs = logs
  return handle
}

test('derives TRON addresses matching the BIP-44 test vector', async () => {
  const signer = createSigner({ seed: TEST_MNEMONIC, network: 'tron-nile', fullHost: 'https://nile.trongrid.io' })
  assert.equal(await signer.jarAddress(0), 'TUEZSdKsoDHQMeZwihtdoBiN46zxhGWYdH')
  const first = await signer.jarAddress(1)
  assert.notEqual(first, await signer.jarAddress(2))
  assert.equal(first, await signer.jarAddress(1))
  assert.notEqual(await signer.treasuryAddress(), first)
  signer.dispose()
})

test('rejects a weak token at startup', () => {
  assert.throws(() => createApp({ signer: fakeSigner(), token: 'short', log: {} }))
})

test('requires the bearer token for everything except health', async () => {
  const handle = app(fakeSigner())
  assert.equal((await call(handle, 'GET', '/healthz', undefined, null)).status, 200)
  assert.equal((await call(handle, 'POST', '/v1/addresses', { index: 1 }, null)).status, 401)
  assert.equal((await call(handle, 'POST', '/v1/addresses', { index: 1 }, 'y'.repeat(40))).status, 401)
  assert.deepEqual((await call(handle, 'POST', '/v1/addresses', { index: 7 })).body, { index: 7, address: 'jar-7' })
})

test('validates USDT transfer intents', async () => {
  const signer = fakeSigner()
  const handle = app(signer)
  const good = { from_index: 3, to: JAR, amount: '11000000', fee_limit_sun: 15000000 }
  assert.equal((await call(handle, 'POST', '/v1/sign/usdt-transfer', good)).status, 200)
  for (const bad of [
    { ...good, from_index: 0 },
    { ...good, from_index: '3' },
    { ...good, to: 'not-an-address' },
    { ...good, to: 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t' },
    { ...good, to: 'TUmbnbtQcMJKtjbrXdqauaMMQBV2X5LDax' },
    { ...good, amount: 11000000 },
    { ...good, amount: '-5' },
    { ...good, amount: '0' },
    { ...good, amount: '1.5' },
    { ...good, fee_limit_sun: 31000000 }
  ]) {
    const result = await call(handle, 'POST', '/v1/sign/usdt-transfer', bad)
    assert.equal(result.status, 422, JSON.stringify(bad))
    assert.equal(result.body.error, 'policy_violation')
  }
  assert.equal(signer.calls.length, 1)
  assert.equal(signer.calls[0][1].amount, 11000000n)
})

test('caps treasury TRX transfers', async () => {
  const signer = fakeSigner()
  const handle = app(signer)
  assert.equal((await call(handle, 'POST', '/v1/sign/trx-transfer', { to: JAR, amount_sun: 1500000, purpose: 'activation' })).status, 200)
  assert.equal((await call(handle, 'POST', '/v1/sign/trx-transfer', { to: JAR, amount_sun: 30000001, purpose: 'activation' })).status, 422)
  assert.equal((await call(handle, 'POST', '/v1/sign/trx-transfer', { to: JAR, amount_sun: 1000, purpose: 'Drain Everything!' })).status, 422)
  assert.equal(signer.calls.length, 1)
})

test('never echoes internal errors or secrets', async () => {
  const signer = fakeSigner()
  signer.signUsdtTransfer = async () => { throw new Error('boom with secret detail') }
  const handle = app(signer)
  const result = await call(handle, 'POST', '/v1/sign/usdt-transfer', { from_index: 1, to: JAR, amount: '1', fee_limit_sun: 1 })
  assert.equal(result.status, 502)
  assert.deepEqual(result.body, { error: 'signing_failed' })
})
