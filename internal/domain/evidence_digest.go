package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const EvidenceFormatVersion = "cave-clearance-evidence/v2"

type evidenceDigestMaterial struct {
	FormatVersion    string         `json:"format_version"`
	TrialID          string         `json:"trial_id"`
	Trial            ClearanceTrial `json:"trial"`
	AuditEvents      []AuditEvent   `json:"audit_events"`
	AuditChainDigest string         `json:"audit_chain_digest"`
}

func normalizedEvidenceTrial(t ClearanceTrial) ClearanceTrial {
	if t.Permit != nil {
		p := *t.Permit
		p.EvidenceDigest = ""
		t.Permit = &p
	}
	t.RejectionEvidenceDigest = ""
	return t
}

func EvidenceContentDigest(t ClearanceTrial, events []AuditEvent) string {
	chain := ""
	if len(events) > 0 {
		chain = events[len(events)-1].EventDigest
	}
	m := evidenceDigestMaterial{FormatVersion: EvidenceFormatVersion, TrialID: t.TrialID, Trial: normalizedEvidenceTrial(t), AuditEvents: events, AuditChainDigest: chain}
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
