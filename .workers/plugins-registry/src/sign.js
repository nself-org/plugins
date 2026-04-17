/**
 * sign.js — Ed25519 signing utilities via Web Crypto API
 *
 * Private key stored as hex in env.SIGNING_PRIVATE_KEY (32 bytes = 64 hex chars)
 * Public key stored as hex in env.SIGNING_PUBLIC_KEY  (32 bytes = 64 hex chars)
 *
 * CF Workers support Ed25519 via Web Crypto since 2023.
 * Key format: raw (32-byte private key, 32-byte public key).
 */

// ---------------------------------------------------------------------------
// Hex helpers
// ---------------------------------------------------------------------------

function hexToBytes(hex) {
  if (hex.length % 2 !== 0) {
    throw new Error('Invalid hex string — odd length');
  }
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    bytes[i / 2] = parseInt(hex.slice(i, i + 2), 16);
  }
  return bytes;
}

function bytesToHex(bytes) {
  return Array.from(bytes)
    .map(b => b.toString(16).padStart(2, '0'))
    .join('');
}

// ---------------------------------------------------------------------------
// Sign a message string with the Ed25519 private key.
// Returns signature as hex string, or null if key not configured or on error.
// ---------------------------------------------------------------------------

export async function signMessage(message, privateKeyHex) {
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

    const encoder = new TextEncoder();
    const data = encoder.encode(message);
    const signatureBuffer = await crypto.subtle.sign('Ed25519', cryptoKey, data);
    return bytesToHex(new Uint8Array(signatureBuffer));
  } catch (err) {
    console.error('signMessage failed:', err.message);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Verify an Ed25519 signature.
// Returns true if valid, false otherwise.
// ---------------------------------------------------------------------------

export async function verifySignature(message, signatureHex, publicKeyHex) {
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

    const encoder = new TextEncoder();
    const data = encoder.encode(message);
    const signatureBytes = hexToBytes(signatureHex);

    return await crypto.subtle.verify('Ed25519', cryptoKey, signatureBytes, data);
  } catch (err) {
    console.error('verifySignature failed:', err.message);
    return false;
  }
}

// ---------------------------------------------------------------------------
// Compute canonical tarball signature string for a plugin.
// Format: "{name}@{version}:{tarball_url}"
// This is what gets signed — not the raw URL — so the signature covers both
// the identity (name@version) and the download location.
// ---------------------------------------------------------------------------

export function canonicalPluginString(name, version, tarballUrl) {
  return `${name}@${version}:${tarballUrl}`;
}
