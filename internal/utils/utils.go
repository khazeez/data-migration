package utils

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

func GenerateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func NowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func ResolveDefaultValue(raw interface{}) interface{} {
	if raw == nil {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return raw
	}
	switch strings.ToLower(s) {
	case "uuid":
		return GenerateUUID()
	case "now":
		return NowString()
	case "null":
		return nil
	default:
		return raw
	}
}
