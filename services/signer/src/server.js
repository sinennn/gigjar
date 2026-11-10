import http from 'node:http'
import { createApp } from './app.js'
import { createSigner } from './signer.js'

const FULL_HOSTS = { 'tron-mainnet': 'https://api.trongrid.io', 'tron-nile': 'https://nile.trongrid.io' }

function log (level, entry) {
  process.stdout.write(JSON.stringify({ time: new Date().toISOString(), level, service: 'signer', ...entry }) + '\n')
}

function required (name) {
  const value = process.env[name]?.trim()
  if (!value) {
    log('ERROR', { msg: `${name} is required` })
    process.exit(1)
  }
  return value
}

const network = required('GIGJAR_CHAIN_NETWORK')
const signer = createSigner({
  seed: required('GIGJAR_SIGNER_SEED'),
  network,
  fullHost: process.env.GIGJAR_TRONGRID_URL?.trim() || FULL_HOSTS[network],
  apiKey: process.env.GIGJAR_TRONGRID_API_KEY?.trim()
})
delete process.env.GIGJAR_SIGNER_SEED

const handle = createApp({
  signer,
  token: required('GIGJAR_SIGNER_TOKEN'),
  maxTrxSun: BigInt(process.env.GIGJAR_SIGNER_MAX_TRX_SUN?.trim() || '30000000'),
  maxFeeLimitSun: BigInt(process.env.GIGJAR_SIGNER_MAX_FEE_LIMIT_SUN?.trim() || '30000000'),
  log: { info: (entry) => log('INFO', entry), error: (entry) => log('ERROR', entry) }
})

const port = Number(process.env.GIGJAR_SIGNER_PORT || 8081)
const server = http.createServer((request, response) => { handle(request, response) })
server.listen(port, async () => {
  log('INFO', { msg: 'signer listening', port, network, treasury: await signer.treasuryAddress() })
})

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    log('INFO', { msg: 'signer shutting down' })
    server.close(() => {
      signer.dispose()
      process.exit(0)
    })
  })
}
