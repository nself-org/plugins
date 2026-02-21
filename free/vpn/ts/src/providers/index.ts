/**
 * VPN Provider Factory
 * Central registry for all VPN provider implementations
 */

import { IVPNProvider, VPNProvider } from '../types.js';
import { NordVPNProvider } from './nordvpn.js';
import { PIAProvider } from './pia.js';
import { MullvadProvider } from './mullvad.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('vpn:providers');

// Provider registry
const providers: Map<VPNProvider, () => IVPNProvider> = new Map();

// Register implemented providers
providers.set('nordvpn', () => new NordVPNProvider());
providers.set('pia', () => new PIAProvider());
providers.set('mullvad', () => new MullvadProvider());

/**
 * Get provider instance by name
 */
export function getProvider(name: VPNProvider): IVPNProvider {
  const factory = providers.get(name);

  if (!factory) {
    throw new Error(
      `Provider '${name}' not implemented. Available providers: ${Array.from(providers.keys()).join(', ')}`
    );
  }

  return factory();
}

/**
 * Check if provider is supported
 */
export function isProviderSupported(name: string): name is VPNProvider {
  return providers.has(name as VPNProvider);
}

/**
 * Get list of all supported providers
 */
export function getSupportedProviders(): VPNProvider[] {
  return Array.from(providers.keys());
}

/**
 * Provider metadata for documentation
 */
export const providerMetadata: Record<
  VPNProvider,
  {
    name: string;
    cliRequired: boolean;
    portForwarding: boolean;
    p2pServers: string;
    notes: string;
  }
> = {
  nordvpn: {
    name: 'NordVPN',
    cliRequired: true,
    portForwarding: false,
    p2pServers: '5,500+ servers in 47 countries',
    notes: 'Requires nordvpn CLI. Install from https://nordvpn.com/download/linux/',
  },
  pia: {
    name: 'Private Internet Access',
    cliRequired: false,
    portForwarding: true,
    p2pServers: 'All servers (port forwarding on all except US)',
    notes: 'Port forwarding supported on most servers. Best for torrenting.',
  },
  mullvad: {
    name: 'Mullvad',
    cliRequired: true,
    portForwarding: false,
    p2pServers: 'All 674 servers support P2P',
    notes: 'Port forwarding removed July 2023. Strong privacy focus. Account number-based.',
  },
};

logger.info(`Provider factory initialized with ${providers.size} providers`);

export { BaseVPNProvider } from './base.js';
export { NordVPNProvider } from './nordvpn.js';
export { PIAProvider } from './pia.js';
export { MullvadProvider } from './mullvad.js';
