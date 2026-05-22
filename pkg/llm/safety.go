// Safety layer wrapped around every Client.
//
// Defense-in-depth (ADR-0021):
//  1. Sanitizer  — detects prompt-injection patterns ("ignore previous",
//     fake system tags, attempts to leak system prompt). Tags the request
//     with an injection_score and, if above threshold, refuses outright.
//  2. PII redactor — strips obvious PII (email, phone, SSN-like, AWS keys)
//     before the prompt leaves the cluster. Tagged in safety_signals so the
//     caller knows redaction happened.
//  3. Output PII redactor — applied to model output before return.
//  4. Refusal — a hard refusal returns a synthetic ChatResponse rather than
//     calling the backend at all. We log the reason but never echo the
//     attacker's payload back into a log line in clear text.
package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// SafeClient wraps any Client with the safety layer. Configure via Sanitizer
// / Redactor; sensible defaults are provided for both.
type SafeClient struct {
	Inner       Client
	Sanitizer   *Sanitizer
	Redactor    *Redactor
	// RefuseAt is the injection score threshold (0..1) above which we refuse
	// to call the model at all. Default 0.7.
	RefuseAt float64
}

func NewSafeClient(inner Client) *SafeClient {
	return &SafeClient{
		Inner:     inner,
		Sanitizer: DefaultSanitizer(),
		Redactor:  DefaultRedactor(),
		RefuseAt:  0.7,
	}
}

func (s *SafeClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	signals := SafetySignals{}
	maxScore := 0.0
	// Inspect every user/tool message; never inspect system (it's our own).
	for i := range req.Messages {
		m := &req.Messages[i]
		if m.Role != RoleUser && m.Role != RoleTool {
			continue
		}
		score := s.Sanitizer.Score(m.Content)
		if score > maxScore {
			maxScore = score
		}
		if redacted, did := s.Redactor.Redact(m.Content); did {
			m.Content = redacted
			signals["pii_redacted"] = true
		}
	}
	signals["injection_score"] = maxScore
	if maxScore >= s.RefuseAt {
		signals["refused"] = true
		signals["refusal_reason"] = "prompt_injection_suspected"
		return &ChatResponse{
			ID:           "safety-refusal",
			Model:        req.Model,
			Message:      Message{Role: RoleAssistant, Content: "I can't help with that request as written."},
			FinishReason: "content_filter",
			Safety:       signals,
		}, nil
	}
	resp, err := s.Inner.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("nil response from inner client")
	}
	if redacted, did := s.Redactor.Redact(resp.Message.Content); did {
		resp.Message.Content = redacted
		signals["output_pii"] = true
	}
	if resp.Safety == nil {
		resp.Safety = SafetySignals{}
	}
	for k, v := range signals {
		resp.Safety[k] = v
	}
	return resp, nil
}

func (s *SafeClient) Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	// Redact inputs in place before embedding — embeddings of PII can be
	// inverted with adversarial decoders.
	for i, in := range req.Input {
		if r, did := s.Redactor.Redact(in); did {
			req.Input[i] = r
		}
	}
	return s.Inner.Embed(ctx, req)
}

func (s *SafeClient) Describe(ctx context.Context, req VisionDescribeRequest) (*VisionDescribeResponse, error) {
	// VLM input is an image URN; injection over the instruction is rare but
	// possible. Apply sanitizer + redactor to the instruction.
	score := s.Sanitizer.Score(req.Instruction)
	if r, _ := s.Redactor.Redact(req.Instruction); r != req.Instruction {
		req.Instruction = r
	}
	if score >= s.RefuseAt {
		return &VisionDescribeResponse{
			Description: "(refused)",
			Safety:      SafetySignals{"refused": true, "refusal_reason": "prompt_injection_suspected"},
		}, nil
	}
	resp, err := s.Inner.Describe(ctx, req)
	if err != nil {
		return nil, err
	}
	if r, did := s.Redactor.Redact(resp.Description); did {
		resp.Description = r
		if resp.Safety == nil {
			resp.Safety = SafetySignals{}
		}
		resp.Safety["output_pii"] = true
	}
	if resp.Safety == nil {
		resp.Safety = SafetySignals{}
	}
	resp.Safety["injection_score"] = score
	return resp, nil
}

// Sanitizer scores text 0..1 for "looks like a prompt injection". The
// signals here are deliberately conservative — false positives are worse
// than false negatives for legitimate workloads, so we anchor on patterns
// that have no good-faith reading in the AegisVision domain.
type Sanitizer struct {
	patterns []*regexp.Regexp
}

func DefaultSanitizer() *Sanitizer {
	pats := []string{
		`(?i)\bignore\s+(?:all|any|the|every|prior|previous|above)(?:\s+(?:prior|previous|above))*\s+instructions?\b`,
		`(?i)\bdisregard\s+(?:all|any|the|every|prior|previous|above)(?:\s+(?:prior|previous|above))*\s+instructions?\b`,
		`(?i)\bforget\s+(?:all|any|the|every|prior|previous|above)\s+(?:prior|previous|above)?\s*instructions?\b`,
		`(?i)\bsystem\s*:\s*you are\b`,
		`(?i)<\s*system\s*>`,
		`(?i)\bact\s+as\s+(?:a|an)\s+(?:different|new|root|admin)\b`,
		`(?i)\bjail[- ]?break\b`,
		`(?i)\breveal\s+(?:your|the)\s+(?:system|hidden|secret)\s+prompt\b`,
		`(?i)\b(?:print|repeat|output|leak|expose)\s+(?:the|your)\s+(?:system|hidden|secret)\s+prompt\b`,
		`(?i)\bDAN\b`,
		`(?i)\b(?:override|bypass|disable)\s+(?:safety|policy|filter|guardrails?)\b`,
	}
	out := make([]*regexp.Regexp, 0, len(pats))
	for _, p := range pats {
		out = append(out, regexp.MustCompile(p))
	}
	return &Sanitizer{patterns: out}
}

// Score returns 0..1. Heuristic: each matching pattern adds 0.25, capped.
func (s *Sanitizer) Score(text string) float64 {
	if s == nil || len(text) == 0 {
		return 0
	}
	hits := 0
	for _, p := range s.patterns {
		if p.MatchString(text) {
			hits++
		}
	}
	if hits == 0 {
		return 0
	}
	score := float64(hits) * 0.25
	if score > 1 {
		score = 1
	}
	return score
}

// Redactor removes structurally-recognizable PII. It is intentionally narrow
// — we redact patterns that are unambiguous, not anything that *could* be a
// name. False redactions silently corrupt prompts, so we err on letting
// natural-language tenant content pass.
type Redactor struct {
	patterns []*regexp.Regexp
	replace  string
}

func DefaultRedactor() *Redactor {
	pats := []string{
		// Email
		`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`,
		// E.164-ish phone
		`\+?\d{1,3}[\s\-]?\(?\d{2,4}\)?[\s\-]?\d{3,4}[\s\-]?\d{3,4}`,
		// SSN-like
		`\b\d{3}-\d{2}-\d{4}\b`,
		// AWS access key id
		`\bAKIA[0-9A-Z]{16}\b`,
		// AWS secret access key
		`\b[A-Za-z0-9/+=]{40}\b`,
		// Bearer token-ish (JWT-like 3-part dotted base64)
		`\beyJ[A-Za-z0-9_\-]+\.eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\b`,
	}
	out := make([]*regexp.Regexp, 0, len(pats))
	for _, p := range pats {
		out = append(out, regexp.MustCompile(p))
	}
	return &Redactor{patterns: out, replace: "[REDACTED]"}
}

// Redact returns the redacted string and whether anything changed.
func (r *Redactor) Redact(text string) (string, bool) {
	if r == nil || len(text) == 0 {
		return text, false
	}
	out := text
	for _, p := range r.patterns {
		out = p.ReplaceAllString(out, r.replace)
	}
	return out, out != text
}

// Truncate clips a string to maxRunes runes (not bytes), preserving valid
// UTF-8. Used by callers that need bounded log fields.
func Truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// FormatTools renders tools as a system-prompt-friendly block (for backends
// that don't natively support function-calling).
func FormatTools(tools []ToolSchema) string {
	if len(tools) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Available tools (call by emitting a JSON object {\"name\":..., \"arguments\":...}):\n")
	for _, t := range tools {
		fmt.Fprintf(&sb, "- %s (risk=%d): %s\n", t.Name, t.RiskTier, t.Description)
	}
	return sb.String()
}
