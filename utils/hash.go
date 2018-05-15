package utils

import (
    "encoding/hex"
    "crypto/sha256"
)


func Hash(input string) string {
    sha_256 := sha256.New()
    sha_256.Write([]byte(input))

    return hex.EncodeToString(sha_256.Sum(nil))
}
