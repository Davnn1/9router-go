// Package imagehistory trims images from turns that are no longer the active
// conversation. A screenshot is cheap to paste but expensive to carry: the
// base64 inflates every later request body, and the provider charges vision
// tokens again for an image the model already read ten turns ago.
//
// The trim keeps the last N user turns intact, including every image in them,
// and replaces an older image block with a short text marker. It never touches
// the text around it, so the conversation still makes sense.
package imagehistory

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// KeepTurnsFromEnv reads IMAGE_HISTORY_TURNS. 0 (the default) disables the
// trim; values outside 1..100 are ignored so a typo cannot silently discard
// every image.
func KeepTurnsFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("IMAGE_HISTORY_TURNS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0
	}
	return n
}

const omitted = "[image omitted: older turn]"

// StripOldImages keeps images in the most recent keepTurns user turns and
// replaces older image blocks. It understands the OpenAI chat shape
// (`messages[].content[].type=image_url`), the Claude Messages shape
// (`type=image`), and the Responses shape (`type=input_image`). A body with no
// image is returned untouched.
func StripOldImages(body []byte, keepTurns int) []byte {
	if keepTurns <= 0 || len(body) == 0 || !bytes.Contains(body, []byte(`"image`)) {
		return body
	}
	var root map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return body
	}

	changed := false
	if messages, ok := root["messages"].([]any); ok {
		changed = stripMessageList(messages, keepTurns) || changed
	}
	if input, ok := root["input"].([]any); ok {
		changed = stripMessageList(input, keepTurns) || changed
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return out
}

// stripMessageList trims every message before the first kept user turn. Images
// in the current turn (user + the assistant/tool messages after it) stay.
func stripMessageList(items []any, keepTurns int) bool {
	userIdx := make([]int, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if ok && strings.EqualFold(strings.TrimSpace(stringOf(m["role"])), "user") {
			userIdx = append(userIdx, i)
		}
	}
	if len(userIdx) == 0 {
		return false
	}
	keepFrom := len(userIdx) - keepTurns
	if keepFrom < 0 {
		keepFrom = 0
	}
	cutoff := userIdx[keepFrom]
	changed := false
	for i := 0; i < cutoff; i++ {
		m, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		if stripContent(m["content"]) {
			changed = true
		}
	}
	return changed
}

func stripContent(value any) bool {
	blocks, ok := value.([]any)
	if !ok {
		return false
	}
	changed := false
	for i, block := range blocks {
		m, ok := block.(map[string]any)
		if !ok {
			continue
		}
		switch stringOf(m["type"]) {
		case "image_url", "image", "input_image":
			blocks[i] = map[string]any{"type": "text", "text": omitted}
			changed = true
		}
	}
	return changed
}

func stringOf(value any) string {
	s, _ := value.(string)
	return s
}
