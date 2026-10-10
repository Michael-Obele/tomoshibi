package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestActionScriptJSON(t *testing.T) {
	b, err := json.Marshal(Action{Type: "evaluate", Script: "1 + 1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"script":"1 + 1"`) {
		t.Errorf("script not serialized: %s", b)
	}
	var back Action
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Script != "1 + 1" || back.Type != "evaluate" {
		t.Errorf("round trip = %+v", back)
	}
}

func TestScrapeResultExtractedFieldsOmitWhenEmpty(t *testing.T) {
	b, err := json.Marshal(ScrapeResult{URL: "x", Markdown: "m"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "evaluations") || strings.Contains(string(b), "extracted_data") {
		t.Errorf("empty result must omit new fields: %s", b)
	}

	full := ScrapeResult{
		Evaluations:   []EvaluationResult{{Type: "evaluate", Result: 1}},
		ExtractedData: &ExtractedData{Sources: []DataSource{{Source: "json_ld", Data: "x"}}},
	}
	b, err = json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal full: %v", err)
	}
	if !strings.Contains(string(b), `"evaluations"`) || !strings.Contains(string(b), `"extracted_data"`) {
		t.Errorf("populated result missing new fields: %s", b)
	}
}

func TestAutoExtractJSON(t *testing.T) {
	b, err := json.Marshal(ScrapeOptions{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "auto_extract") {
		t.Errorf("nil AutoExtract must omit: %s", b)
	}

	f := false
	b, err = json.Marshal(ScrapeOptions{AutoExtract: &f})
	if err != nil {
		t.Fatalf("marshal false: %v", err)
	}
	if !strings.Contains(string(b), `"auto_extract":false`) {
		t.Errorf("explicit false must serialize: %s", b)
	}

	var opts ScrapeOptions
	if err := json.Unmarshal([]byte(`{"auto_extract":true}`), &opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if opts.AutoExtract == nil || !*opts.AutoExtract {
		t.Errorf("AutoExtract = %v, want true", opts.AutoExtract)
	}
}

func TestScreenshotOptionsScaleJSON(t *testing.T) {
	b, err := json.Marshal(ScreenshotOptions{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "scale") {
		t.Errorf("empty scale must omit: %s", b)
	}
	b, err = json.Marshal(ScreenshotOptions{Scale: "css"})
	if err != nil {
		t.Fatalf("marshal css: %v", err)
	}
	if !strings.Contains(string(b), `"scale":"css"`) {
		t.Errorf("scale not serialized: %s", b)
	}
}
