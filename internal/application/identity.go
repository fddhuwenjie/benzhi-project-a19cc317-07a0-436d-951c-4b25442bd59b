package application

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"cave-microclimate-clearance/internal/domain"
)

var fallbackCounter atomic.Uint64

func newID(prefix string) string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err == nil {
		return prefix + "-" + hex.EncodeToString(b)
	}
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), fallbackCounter.Add(1))
}

func fingerprint(operation string, value any) string {
	return domain.AuditPayloadDigest(operation, value)
}

func stableID(prefix, seed string) string {
	h := sha256.Sum256([]byte(prefix + "\x00" + seed))
	return prefix + "-" + hex.EncodeToString(h[:12])
}

func validateMeta(m CommandMeta, creation bool) error {
	if strings.TrimSpace(m.RequestID) == "" || len(m.RequestID) > 128 {
		return domain.Validation("request_id", "request_id 不能为空且不得超过 128 字符")
	}
	if strings.TrimSpace(m.ActorID) == "" {
		return domain.Validation("actor_id", "actor_id 不能为空")
	}
	if creation && m.ExpectedRevision != 0 {
		return domain.Validation("expected_revision", "创建请求 expected_revision 必须为 0")
	}
	if !creation && m.ExpectedRevision < 1 {
		return domain.Validation("expected_revision", "写请求必须提供正数 expected_revision")
	}
	return nil
}
