package crypto


import (
    "crypto/aes"
    "fmt"
    "crypto/cipher"
    "crypto/sha256"
    crand "crypto/rand"
    rand "math/rand"
    "encoding/base64"
    "math"
    "math/big"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890"


func DecryptString(ct string, key string, iv string) (string, error) {
    // Key 디코딩 및 검증
    key_bytes, err := base64.StdEncoding.DecodeString(key)
    if err != nil {
        return "", fmt.Errorf("invalid key: %v", err)
    }
    if len(key_bytes) != 16 && len(key_bytes) != 24 && len(key_bytes) != 32 {
        return "", fmt.Errorf("invalid key length: %d (must be 16, 24, or 32 bytes)", len(key_bytes))
    }

    // IV 디코딩 및 검증
    iv_bytes, err := base64.StdEncoding.DecodeString(iv)
    if err != nil {
        return "", fmt.Errorf("invalid iv: %v", err)
    }
    if len(iv_bytes) != aes.BlockSize {
        return "", fmt.Errorf("invalid iv length: %d (must be %d bytes)", len(iv_bytes), aes.BlockSize)
    }

    // 암호문 디코딩 및 검증
    ct_bytes, err := base64.StdEncoding.DecodeString(ct)
    if err != nil {
        return "", fmt.Errorf("invalid ciphertext: %v", err)
    }
    if len(ct_bytes)%aes.BlockSize != 0 {
        return "", fmt.Errorf("ciphertext length %d is not a multiple of block size %d", len(ct_bytes), aes.BlockSize)
    }

    // AES 복호화
    block, err := aes.NewCipher(key_bytes)
    if err != nil {
        return "", fmt.Errorf("failed to create cipher: %v", err)
    }
    mode := cipher.NewCBCDecrypter(block, iv_bytes)
    pt_bytes := make([]byte, len(ct_bytes))
    mode.CryptBlocks(pt_bytes, ct_bytes)

    // 패딩 제거
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
