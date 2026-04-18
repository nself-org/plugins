/**
 * sign.ts — Ed25519 signing for plugin registry tarballs (TypeScript).
 *
 * Spec: S43-T02 (P93 Wave 5). Cloudflare Worker-side signing of the canonical
 * plugin release string: "{name}@{version}:{tarball_url}".
 *
 * Private key hex (32 bytes / 64 chars) lives in env.SIGNING_PRIVATE_KEY.
 * Public key hex (32 bytes / 64 chars) lives in env.SIGNING_PUBLIC_KEY.
 *
 * Workers expose Ed25519 through the Web Crypto API. This module also works on
 * Node 19+ (crypto.subtle) for local tooling and unit tests.
 *
 * Companion implementation: plugins-pro/tools/build/sign.go (offline signer).
 */

export interface SignEnv {
  SIGNING_PRIVATE_KEY?: string;
  SIGNING_PUBLIC_KEY?: string;
}

// ---------------------------------------------------------------------------
// Hex helpers
// ---------------------------------------------------------------------------

export function hexToBytes(hex: string): Uint8Array {
  if (hex.length % 2 !== 0) {
    throw new Error('hexToBytes: odd-length hex string');
  }
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    out[i / 2] = parseInt(hex.slice(i, i + 2), 16);
  }
  return out;
}

export function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

// ---------------------------------------------------------------------------
// Canonical message format
// ---------------------------------------------------------------------------

/**
 * canonicalPluginString builds the exact byte sequence that is signed.
 * Must match plugins-pro/tools/build/sign.go CanonicalString() and the CLI's
 * internal/plugin/verify.go CanonicalPluginString() byte-for-byte.
 */
export function canonicalPluginString(
  name: string,
  version: string,
  tarballURL: string,
): string {
  return `${name}@${version}:${tarballURL}`;
}

// ---------------------------------------------------------------------------
// Sign a message with the Ed25519 private key.
// Returns signature hex, or null when not configured / on error.
// ---------------------------------------------------------------------------

export async function signMessage(
  message: string,
  privateKeyHex: string | undefined,
): Promise<string | null> {
  if (!privateKeyHex) return null;

  try {
    const keyBytes = hexToBytes(privateKeyHex);
    const cryptoKey = await crypto.subtle.importKey(
      'raw',
      keyBytes,
      { name: 'Ed25519' },
      false,
      ['sign'],
    );
    const data = new TextEncoder().encode(message);
    const signatureBuffer = await crypto.subtle.sign('Ed25519', cryptoKey, data);
    return bytesToHex(new Uint8Array(signatureBuffer));
  } catch (err) {
    console.error('signMessage failed:', (err as Error).message);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Verify an Ed25519 signature. Returns true when valid, false otherwise.
// ---------------------------------------------------------------------------

export async function verifySignature(
  message: string,
  signatureHex: string | undefined,
  publicKeyHex: string | undefined,
): Promise<boolean> {
  if (!publicKeyHex || !signatureHex) return false;

  try {
    const keyBytes = hexToBytes(publicKeyHex);
    const cryptoKey = await crypto.subtle.importKey(
      'raw',
      keyBytes,
      { name: 'Ed25519' },
      false,
      ['verify'],
    );
    const data = new TextEncoder().encode(message);
    const signatureBytes = hexToBytes(signatureHex);
    return await crypto.subtle.verify('Ed25519', cryptoKey, signatureBytes, data);
  } catch (err) {
    console.error('verifySignature failed:', (err as Error).message);
    return false;
  }
}

// ---------------------------------------------------------------------------
// Convenience wrapper: sign a plugin release entry.
// ---------------------------------------------------------------------------

export interface PluginReleaseRef {
  name: string;
  version: string;
  tarballURL: string;
}

export interface SignedRelease extends PluginReleaseRef {
  signature: string;
  keyHint?: string;
}

export async function signPluginRelease(
  ref: PluginReleaseRef,
  env: SignEnv,
): Promise<SignedRelease | null> {
  const message = canonicalPluginString(ref.name, ref.version, ref.tarballURL);
  const signature = await signMessage(message, env.SIGNING_PRIVATE_KEY);
  if (!signature) return null;
  return {
    ...ref,
    signature,
    keyHint: env.SIGNING_PUBLIC_KEY
      ? env.SIGNING_PUBLIC_KEY.slice(0, 8)
      : undefined,
  };
}

export async function verifyPluginRelease(
  release: SignedRelease,
  env: SignEnv,
): Promise<boolean> {
  const message = canonicalPluginString(release.name, release.version, release.tarballURL);
  return verifySignature(message, release.signature, env.SIGNING_PUBLIC_KEY);
}
