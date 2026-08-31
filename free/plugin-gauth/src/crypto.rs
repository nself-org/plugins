// Package crypto: AES-256-GCM encrypt/decrypt for refresh token storage.
//
// Purpose: Encrypt refresh tokens before DB storage; decrypt on retrieval.
// Inputs: 32-byte key from GAUTH_ENCRYPTION_KEY env (hex-encoded).
// Outputs: Base64-encoded ciphertext (nonce prepended).
// Constraints: Key must be exactly 32 bytes. Nonce is random per encrypt call.

use aes_gcm::{
    aead::{Aead, AeadCore, KeyInit, OsRng},
    Aes256Gcm, Key, Nonce,
};
use base64::{engine::general_purpose::STANDARD as B64, Engine as _};

use crate::error::GauthError;

/// Encrypts plaintext using AES-256-GCM.
/// Returns base64(nonce || ciphertext).
pub fn encrypt(key_bytes: &[u8; 32], plaintext: &str) -> Result<String, GauthError> {
    let key = Key::<Aes256Gcm>::from_slice(key_bytes);
    let cipher = Aes256Gcm::new(key);
    let nonce = Aes256Gcm::generate_nonce(&mut OsRng);
    let ciphertext = cipher
        .encrypt(&nonce, plaintext.as_bytes())
        .map_err(|_| GauthError::CryptoError)?;
    let mut combined = nonce.to_vec();
    combined.extend(ciphertext);
    Ok(B64.encode(combined))
}

/// Decrypts base64(nonce || ciphertext) using AES-256-GCM.
pub fn decrypt(key_bytes: &[u8; 32], encoded: &str) -> Result<String, GauthError> {
    let combined = B64.decode(encoded).map_err(|_| GauthError::CryptoError)?;
    if combined.len() < 12 {
        return Err(GauthError::CryptoError);
    }
    let (nonce_bytes, ciphertext) = combined.split_at(12);
    let key = Key::<Aes256Gcm>::from_slice(key_bytes);
    let cipher = Aes256Gcm::new(key);
    let nonce = Nonce::from_slice(nonce_bytes);
    let plaintext = cipher
        .decrypt(nonce, ciphertext)
        .map_err(|_| GauthError::CryptoError)?;
    String::from_utf8(plaintext).map_err(|_| GauthError::CryptoError)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_encrypt_decrypt_roundtrip() {
        let key = [0u8; 32];
        let plaintext = "my-secret-refresh-token";
        let encrypted = encrypt(&key, plaintext).expect("encrypt");
        let decrypted = decrypt(&key, &encrypted).expect("decrypt");
        assert_eq!(decrypted, plaintext);
    }

    #[test]
    fn test_decrypt_wrong_key_fails() {
        let key1 = [1u8; 32];
        let key2 = [2u8; 32];
        let encrypted = encrypt(&key1, "token").expect("encrypt");
        assert!(decrypt(&key2, &encrypted).is_err());
    }

    #[test]
    fn test_encrypted_never_contains_plaintext() {
        let key = [42u8; 32];
        let secret = "super-secret-refresh-token";
        let encrypted = encrypt(&key, secret).expect("encrypt");
        assert!(
            !encrypted.contains(secret),
            "encrypted form must not contain plaintext"
        );
    }
}
