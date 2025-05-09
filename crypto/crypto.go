package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    crand "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "math"
    "math/big"
    rand "math/rand"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890"

func DecryptString(ct string, key string, iv string) (string, error) {
    // Key Decode, and Verification`
    key_bytes, err := base64.StdEncoding.DecodeString(key)
    if err != nil {
        return "", fmt.Errorf("invalid key: %v", err)
    }
    if len(key_bytes) != 16 && len(key_bytes) != 24 && len(key_bytes) != 32 {
        return "", fmt.Errorf("invalid key length: %d (must be 16, 24, or 32 bytes)", len(key_bytes))
    }

    // IV Verification
    iv_bytes, err := base64.StdEncoding.DecodeString(iv)
    if err != nil {
        return "", fmt.Errorf("invalid iv: %v", err)
    }
    if len(iv_bytes) != aes.BlockSize {
        return "", fmt.Errorf("invalid iv length: %d (must be %d bytes)", len(iv_bytes), aes.BlockSize)
    }

    // Decode crypted text
    ct_bytes, err := base64.StdEncoding.DecodeString(ct)
    if err != nil {
        return "", fmt.Errorf("invalid ciphertext: %v", err)
    }
    if len(ct_bytes)%aes.BlockSize != 0 {
        return "", fmt.Errorf("ciphertext length %d is not a multiple of block size %d", len(ct_bytes), aes.BlockSize)
    }

    // AES-256 Decryption
    block, err := aes.NewCipher(key_bytes)
    if err != nil {
        return "", fmt.Errorf("failed to create cipher: %v", err)
    }
    mode := cipher.NewCBCDecrypter(block, iv_bytes)
    pt_bytes := make([]byte, len(ct_bytes))
    mode.CryptBlocks(pt_bytes, ct_bytes)

    // Delete paddings
    if len(pt_bytes) == 0 {
        return "", fmt.Errorf("decrypted plaintext is empty")
    }
    padding := int(pt_bytes[len(pt_bytes)-1])
    if padding < 1 || padding > aes.BlockSize {
        return "", fmt.Errorf("invalid padding value: %d", padding)
    }
    for i := len(pt_bytes) - padding; i < len(pt_bytes); i++ {
        if pt_bytes[i] != byte(padding) {
            return "", fmt.Errorf("invalid padding bytes")
        }
    }
    pt_bytes = pt_bytes[:len(pt_bytes)-padding]

    return string(pt_bytes), nil
}

// SHA hash, currently not using this, it may be useful for remote CLI manager
// which will be developed in summer
func sha256_hash(password string) string {
    hasher := sha256.New()
    hasher.Write([]byte(password))
    hashedBytes := hasher.Sum(nil)
    hashedString := base64.StdEncoding.EncodeToString(hashedBytes)
    return hashedString
}

func RandStringBytes(n int) string {
    seed, _ := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
    rand.Seed(seed.Int64())

    b := make([]byte, n)
    for i := range b {
        b[i] = letterBytes[rand.Intn(len(letterBytes))]
    }
    return string(b)
}
