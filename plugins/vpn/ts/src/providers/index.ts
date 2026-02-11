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

// TODO: Implement remaining providers
// providers.set('surfshark', () => new SurfsharkProvider());
// providers.set('expressvpn', () => new ExpressVPNProvider());
// providers.set('protonvpn', () => new ProtonVPNProvider());
// providers.set('keepsolid', () => new KeepSolidProvider());
// providers.set('cyberghost', () => new CyberGhostProvider());
// providers.set('airvpn', () => new AirVPNProvider());
// providers.set('windscribe', () => new WindscribeProvider());

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
  surfshark: {
    name: 'Surfshark',
    cliRequired: false,
    portForwarding: false,
    p2pServers: 'All 4,500+ servers support P2P',
    notes: 'Uses manual WireGuard configs. All servers support P2P with automatic routing.',
  },
  expressvpn: {
    name: 'ExpressVPN',
    cliRequired: true,
    portForwarding: false,
    p2pServers: 'All 3,000+ servers support P2P',
    notes: 'Requires expressvpn CLI. May internally route to Switzerland/Netherlands for P2P.',
  },
  pia: {
    name: 'Private Internet Access',
    cliRequired: false,
    portForwarding: true,
    p2pServers: 'All servers (port forwarding on all except US)',
    notes: 'Port forwarding supported on most servers. Best for torrenting.',
  },
  protonvpn: {
    name: 'Proton VPN',
    cliRequired: true,
    portForwarding: true,
    p2pServers: '140+ dedicated P2P servers',
    notes: 'Requires protonvpn CLI. NAT-PMP port forwarding. Requires Plus plan for P2P.',
  },
  mullvad: {
    name: 'Mullvad',
    cliRequired: true,
    portForwarding: false,
    p2pServers: 'All 674 servers support P2P',
    notes: 'Port forwarding removed July 2023. Strong privacy focus. Account number-based.',
  },
  keepsolid: {
    name: 'KeepSolid VPN Unlimited',
    cliRequired: false,
    portForwarding: false,
    p2pServers: 'ONLY 5 servers (Canada, Romania, France, Luxembourg)',
    notes: 'SEVERELY LIMITED for P2P. US BitTorrent ban. Not recommended for torrenting.',
  },
  cyberghost: {
    name: 'CyberGhost',
    cliRequired: true,
    portForwarding: false,
    p2pServers: '87 P2P-optimized locations',
    notes: 'Requires cyberghostvpn CLI. Dedicated P2P servers.',
  },
  airvpn: {
    name: 'AirVPN',
    cliRequired: true,
    portForwarding: true,
    p2pServers: 'All 260 servers support P2P + port forwarding',
    notes: 'Requires Eddie CLI. 20 ports per account. Best for power users.',
  },
  windscribe: {
    name: 'Windscribe',
    cliRequired: true,
    portForwarding: true,
    p2pServers: 'Most servers (600+) support P2P',
    notes: 'Requires windscribe CLI. Port forwarding for Pro users. Excludes ~11 countries.',
  },
};

logger.info(`Provider factory initialized with ${providers.size} providers`);

export { BaseVPNProvider } from './base.js';
export { NordVPNProvider } from './nordvpn.js';
export { PIAProvider } from './pia.js';
export { MullvadProvider } from './mullvad.js';
