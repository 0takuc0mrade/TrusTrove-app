import { useState } from "react";
import { getNetworkDetails } from "@stellar/freighter-api";
import { useWalletStore } from "@/store/wallet";
import { connectFreighter, FreighterError } from "@/lib/freighter";
import { useBalances } from "./useBalances";
import { createErrorHandler } from "@/lib/errors";

const { captureError } = createErrorHandler("useWallet");
const REQUIRED_NETWORK = "testnet";

/**
 * Custom hook for managing Stellar wallet connection via Freighter.
 *
 * Provides wallet state and actions to connect or disconnect a Freighter wallet.
 * Connections are only allowed when Freighter is active on the application's
 * required network, currently Stellar testnet.
 *
 * @returns An object containing:
 *   - `address` — The connected wallet's public key, or `null` if not connected.
 *   - `connected` — Whether a wallet is currently connected.
 *   - `network` — The active network identifier (e.g. `'testnet'`).
 *   - `connectWallet` — Async function that opens Freighter and connects the wallet.
 *   - `disconnectWallet` — Function that disconnects the current wallet.
 *   - `loading` — `true` while a connection attempt is in progress.
 *   - `error` — Error message string if the last connection attempt failed, otherwise `null`.
 *
 * @throws Will catch errors from Freighter and surface them via the `error` return value
 *   rather than throwing to the caller.
 *
 * @example
 * const { address, connected, connectWallet, disconnectWallet, loading, error } = useWallet();
 */
export function useWallet() {
  const { address, connected, network, connect, disconnect } = useWalletStore();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const {
    balances,
    loading: balancesLoading,
    error: balancesError,
    refetch: refetchBalances,
  } = useBalances();

  /**
   * Initiates a Freighter wallet connection.
   *
   * Freighter's active network is checked after access is granted and before
   * the wallet is written to application state. This prevents balances,
   * authentication, and transactions from being used with a wallet connected
   * to a different Stellar network.
   */
  const connectWallet = async () => {
    setLoading(true);
    setError(null);
    setErrorCode(null);

    try {
      const addr = await connectFreighter();
      const networkDetails = await getNetworkDetails();
      const currentNetwork = networkDetails.network.trim().toLowerCase();

      if (currentNetwork !== REQUIRED_NETWORK) {
        throw new FreighterError(
          "wrong_network",
          `Freighter is connected to ${networkDetails.network}. Switch Freighter to Stellar Testnet and try again.`,
        );
      }

      connect(addr, REQUIRED_NETWORK);
    } catch (err: unknown) {
      const appError = captureError(err);
      setError(appError.message);
      if (err instanceof FreighterError) {
        setErrorCode(err.code);
      }
      disconnect();
    } finally {
      setLoading(false);
    }
  };

  /**
   * Disconnects the currently connected wallet by clearing wallet state from the store.
   */
  const disconnectWallet = () => {
    disconnect();
  };

  return {
    address,
    connected,
    network,
    connectWallet,
    disconnectWallet,
    loading,
    error,
    errorCode,
    balances,
    balancesLoading,
    balancesError,
    refetchBalances,
  };
}
