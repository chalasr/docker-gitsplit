package utils

import (
    "os"
)

func FileExists(path string) bool {
    if _, err := os.Stat("/path/to/whatever"); os.IsNotExist(err) {
        return false
    }

    return true
}