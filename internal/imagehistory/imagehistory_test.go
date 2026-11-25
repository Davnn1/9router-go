package imagehistory

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripOldImagesKeepsRecentTurns(t *testing.T) {
	body := []byte(`{
		"model":"vision",
		"messages":[
			{"role":"user","content":[
				{"type":"text","text":"first"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}
			]},
			{"role":"assistant","content":"seen"},
			{"role":"user","content":[
				{"type":"image_url","image_url":{"url":"data:image/png;base64,BBBB"}}
			]},
			{"role":"assistant","content":"seen"},
			{"role":"user","content":[
				{"type":"image_url","image_url":{"url":"data:image/png;base64,CCCC"}}
			]}
		]
	}`)

	got := StripOldImages(body, 2)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	messages := root["messages"].([]any)
	first := messages[0].(map[string]any)["content"].([]any)
	if block := first[1].(map[string]any); block["type"] != "text" || block["text"] != omitted {
		t.Errorf("old image was not replaced: %+v", block)
	}
	second := messages[2].(map[string]any)["content"].([]any)
	if second[0].(map[string]any)["type"] != "image_url" {
		t.Errorf("image inside the kept window was dropped: %+v", second)
	}
	if !strings.Contains(string(got), "BBBB") || !strings.Contains(string(got), "CCCC") {
		t.Errorf("kept images missing from result")
	}
	if strings.Contains(string(got), "AAAA") {
		t.Errorf("old base64 survived the trim")
	}
}

func TestStripOldImagesHandlesClaudeAndResponses(t *testing.T) {
	claude := []byte(`{"messages":[
		{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"OLD"}}]},
		{"role":"assistant","content":"ok"},
		{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"NEW"}}]}
	]}`)
	got := StripOldImages(claude, 1)
	if strings.Contains(string(got), `"OLD"`) || !strings.Contains(string(got), `"NEW"`) {
		t.Errorf("Claude trim = %s", got)
	}

	responses := []byte(`{"input":[
		{"role":"user","content":[{"type":"input_image","image_url":"data:OLD"}]},
		{"role":"user","content":[{"type":"input_image","image_url":"data:NEW"}]}
	]}`)
	got = StripOldImages(responses, 1)
	if strings.Contains(string(got), "OLD") || !strings.Contains(string(got), "NEW") {
		t.Errorf("Responses trim = %s", got)
	}
}

func TestStripOldImagesLeavesImageFreeBodyAlone(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	got := StripOldImages(body, 1)
	if string(got) != string(body) {
		t.Errorf("body changed without images: %s", got)
	}
}
