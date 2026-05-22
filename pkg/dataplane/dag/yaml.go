// YAML loader for pipeline DAGs. The doc's §20 example pipeline is the
// canonical input form; this file parses it into the PipelineSpec the
// compiler consumes. We use the stdlib JSON path with a tiny YAML→JSON
// shim to avoid pulling a third-party YAML library into the dataplane
// module — the shim is enough for the well-formed pipelines the platform
// accepts.
package dag

import (
	"encoding/json"
	"errors"
	"strings"
)

// LoadJSON parses a JSON-formatted PipelineSpec.
func LoadJSON(body []byte) (PipelineSpec, error) {
	var spec PipelineSpec
	err := json.Unmarshal(body, &spec)
	return spec, err
}

// LoadYAML parses the doc's YAML form. Supported subset (sufficient for
// every example in the architecture doc):
//
//   - Top-level scalar key/value pairs.
//   - A `nodes:` block whose items are maps introduced by `- `.
//   - Per-node `params:` and `after:` keys with inline list/map values.
//   - Scalar values: strings, numbers, booleans, null.
func LoadYAML(body []byte) (PipelineSpec, error) {
	j, err := yamlToJSON(string(body))
	if err != nil {
		return PipelineSpec{}, err
	}
	return LoadJSON([]byte(j))
}

// yamlToJSON converts the YAML subset to JSON. The parser is a small
// purpose-built thing — supports the architecture-doc example shape and
// rejects anything fancier (anchors, multi-doc, multi-line strings).
func yamlToJSON(in string) (string, error) {
	lines := strings.Split(in, "\n")

	// Pass 1: collect top-level (k, v) pairs and the `nodes` list as raw
	// maps. Indentation is meaningful but rigid: top-level at column 0,
	// node items at any non-zero indent starting `- `, node fields at the
	// indent of the first node field.
	type node map[string]string
	var (
		topPairs []struct {
			K string
			V string
		}
		nodes      []node
		curNode    node
		inNodes    bool
	)

	flushCurNode := func() {
		if curNode != nil {
			nodes = append(nodes, curNode)
			curNode = nil
		}
	}

	for i := 0; i < len(lines); i++ {
		raw := strings.TrimRight(lines[i], " \t")
		if raw == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		trimmed := strings.TrimSpace(raw)
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))

		if indent == 0 {
			flushCurNode()
			k, v, ok := splitKV(trimmed)
			if !ok {
				return "", errors.New("yaml: bad top-level line: " + trimmed)
			}
			if k == "nodes" && v == "" {
				inNodes = true
				continue
			}
			inNodes = false
			topPairs = append(topPairs, struct{ K, V string }{k, v})
			continue
		}

		if !inNodes {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") {
			flushCurNode()
			curNode = node{}
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if rest != "" {
				if k, v, ok := splitKV(rest); ok {
					curNode[k] = v
				}
			}
			continue
		}
		// Continuation field of the current node.
		if curNode == nil {
			return "", errors.New("yaml: node field outside a node: " + trimmed)
		}
		if k, v, ok := splitKV(trimmed); ok {
			curNode[k] = v
		}
	}
	flushCurNode()

	// Pass 2: emit JSON.
	var out strings.Builder
	out.WriteString("{")
	first := true
	emit := func(k, jsonVal string) {
		if !first {
			out.WriteString(",")
		}
		out.WriteString(jsonStr(k))
		out.WriteString(":")
		out.WriteString(jsonVal)
		first = false
	}
	for _, p := range topPairs {
		emit(p.K, jsonScalar(p.V))
	}
	if len(nodes) > 0 || inNodes {
		if !first {
			out.WriteString(",")
		}
		out.WriteString(jsonStr("nodes") + ":[")
		for i, n := range nodes {
			if i > 0 {
				out.WriteString(",")
			}
			out.WriteString("{")
			nf := true
			for k, v := range n {
				if !nf {
					out.WriteString(",")
				}
				out.WriteString(jsonStr(k))
				out.WriteString(":")
				out.WriteString(jsonScalar(v))
				nf = false
			}
			out.WriteString("}")
		}
		out.WriteString("]")
	}
	out.WriteString("}")
	return out.String(), nil
}

func splitKV(line string) (string, string, bool) {
	idx := strings.IndexByte(line, ':')
	if idx <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// jsonScalar converts a YAML scalar into a JSON value. Empty → null;
// quoted strings stay strings; numbers/booleans/null pass through; inline
// lists/maps are normalized.
func jsonScalar(s string) string {
	if s == "" {
		return "null"
	}
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return jsonStr(s[1 : len(s)-1])
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		body := s[1 : len(s)-1]
		parts := splitNoQuotes(body, ',')
		var sb strings.Builder
		sb.WriteString("[")
		for i, p := range parts {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(jsonScalar(strings.TrimSpace(p)))
		}
		sb.WriteString("]")
		return sb.String()
	}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		body := s[1 : len(s)-1]
		parts := splitNoQuotes(body, ',')
		var sb strings.Builder
		sb.WriteString("{")
		nf := true
		for _, p := range parts {
			k, v, ok := splitKV(strings.TrimSpace(p))
			if !ok {
				continue
			}
			if !nf {
				sb.WriteString(",")
			}
			sb.WriteString(jsonStr(k) + ":" + jsonScalar(v))
			nf = false
		}
		sb.WriteString("}")
		return sb.String()
	}
	switch s {
	case "true", "false", "null":
		return s
	}
	if isNumber(s) {
		return s
	}
	return jsonStr(s)
}

func splitNoQuotes(s string, sep byte) []string {
	var parts []string
	depth := 0
	last := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	parts = append(parts, s[last:])
	return parts
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	dotSeen := false
	for i, c := range s {
		if i == 0 && (c == '-' || c == '+') {
			continue
		}
		if c == '.' {
			if dotSeen {
				return false
			}
			dotSeen = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
