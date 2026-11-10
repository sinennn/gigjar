import WalletManagerTron from '@tetherto/wdk-wallet-tron'
import { TronWeb } from 'tronweb'

const TREASURY_PATH = "1'/0/0"
const EXPIRATION_EXTENSION_SECONDS = 540

export const USDT_CONTRACTS = {
  'tron-mainnet': 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
  'tron-nile': 'TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf'
}

export function createSigner ({ seed, network, fullHost, apiKey }) {
  const contract = USDT_CONTRACTS[network]
  if (!contract) {
    throw new Error(`unsupported network ${network}`)
  }
  const tronWeb = new TronWeb({ fullHost, headers: apiKey ? { 'TRON-PRO-API-KEY': apiKey } : {} })
  const wallet = new WalletManagerTron(seed, { provider: tronWeb })

  async function treasury () {
    return wallet.getAccountByPath(TREASURY_PATH)
  }

  async function finish (account, transaction) {
    const extended = await tronWeb.transactionBuilder.extendExpiration(transaction, EXPIRATION_EXTENSION_SECONDS)
    const signed = await account.signTransaction(extended)
    return { tx_id: signed.txID, signed_tx: signed, expires_at_ms: signed.raw_data.expiration }
  }

  return {
    network,
    contract,
    isValidAddress: (address) => TronWeb.isAddress(address),
    async jarAddress (index) {
      const account = await wallet.getAccount(index)
      return account.getAddress()
    },
    async treasuryAddress () {
      return (await treasury()).getAddress()
    },
    async signUsdtTransfer ({ fromIndex, to, amount, feeLimitSun }) {
      const account = await wallet.getAccount(fromIndex)
      const owner = await account.getAddress()
      const { transaction, result } = await tronWeb.transactionBuilder.triggerSmartContract(
        contract,
        'transfer(address,uint256)',
        { feeLimit: Number(feeLimitSun) },
        [{ type: 'address', value: to }, { type: 'uint256', value: amount.toString() }],
        tronWeb.address.toHex(owner)
      )
      if (!result?.result || !transaction) {
        throw new Error('node refused to build the USDT transfer')
      }
      return { from: owner, ...(await finish(account, transaction)) }
    },
    async signTrxTransfer ({ to, amountSun }) {
      const account = await treasury()
      const owner = await account.getAddress()
      const transaction = await tronWeb.transactionBuilder.sendTrx(to, Number(amountSun), owner)
      return { from: owner, ...(await finish(account, transaction)) }
    },
    dispose () {
      wallet.dispose()
    }
  }
}
