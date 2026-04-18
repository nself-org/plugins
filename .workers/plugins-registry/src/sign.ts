/**
 * sign.ts — Ed25519 signing and verification utilities via Web Crypto API
 *
 * Private key: stored as hex in env.SIGNING_PRIVATE_KEY (32-byte seed = 64 hex chars)
 * Public key:  stored as hex in env.PUBLIC_KEY_HEX      (32-byte key  = 64 hex chars)
 *
 * Cloudflare Workers support Ed25519 via Web Crypto (crypto.subtle) since 2023.
 * Key format: "raw" (32-byte private seed, 32-byte public key).
 *
 * Canonical signing string format: "{name}@{version}:{tarball_url}"
 * This format must match the Go CLI implementation exactly so signatures
 * verified offline by the CLI remain valid.
 */

// ---------------------------------------------------------------------------
// Hex encoding helpers
// ---------------------------------------------------------------------------

function hexToBytes(hex: string): Uint8Array {
  if (hex.length % 2 !== 0) {
    throw new Error(`Invalid hex string — odd length (${hex.length})`);
  }
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    const byte = parseInt(hex.slice(i, i + 2), 16);
    if (isNaN(byte)) throw new Error(`Invalid hex byte at position ${i}: "${hex.slice(i, i + 2)}"`);
    bytes[i / 2] = byte;
  }
  return bytes;
}

function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

// ---------------------------------------------------------------------------
// Canonical signing string
// Format: "{name}@{version}:{tarball_url}"
// ---------------------------------------------------------------------------

export function canonicalPluginString(
  name: string,
  version: string,
  tarballURL: string,
): string {
  return `${name}@${version}:${tarballURL}`;
}

// ---------------------------------------------------------------------------
// Sign a message with an Ed25519 private key.
// Returns hex-encoded signature, or null if key absent or on error.
// ---------------------------------------------------------------------------

export async function signPlugin(
  name: string,
  version: string,
  tarballURL: string,
  privateKeyHex: string,
): Promise<string | null> {
  if (!privateKeyHex) return null;

  try {
    const keyBytes = hexToBytes(privateKeyHex);
    const cryptoKey = await crypto.subtle.importKey(
      "raw",
      keyBytes,
      { name: "Ed25519" },
      false,
      ["sign"],
    );

    const message = canonicalPluginString(name, version, tarballURL);
    const encoder = new TextEncoder();
    const signatureBuffer = await crypto.subtle.sign("Ed25519", cryptoKey, encoder.encode(message));
    return bytesToHex(new Uint8Array(signatureBuffer));
  } catch (err) {
    console.error("signPlugin failed:", (err as Error).message);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Verify an Ed25519 signature.
// Returns true if valid, false otherwise.
// ---------------------------------------------------------------------------

export async function verifyPlugin(
  name: string,
  version: string,
  tarballURL: string,
  signatureHex: string,
  publicKeyHex: string,
): Promise<boolean> {
  if (!publicKeyHex || !signatureHex) return false;

  try {
    const keyBytes = hexToBytes(publicKeyHex);
    const cryptoKey = await crypto.subtle.importKey(
      "raw",
      keyBytes,
      { name: "Ed25519" },
      false,
      ["verify"],
    );

    const message = canonicalPluginString(name, version, tarballURL);
    const encoder = new TextEncoder();
    const signatureBytes = hexToBytes(signatureHex);

    return await crypto.subtle.verify("Ed25519", cryptoKey, signatureBytes, encoder.encode(message));
  } catch (err) {
    console.error("verifyPlugin failed:", (err as Error).message);
    return false;
  }
}
