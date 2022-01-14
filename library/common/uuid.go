package common

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func RandomInt64() int64 {
	return rand.Int63()
}
