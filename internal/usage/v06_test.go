package usage

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

func TestModalityImage_AndScreenshotBloat(t *testing.T) {
	s, p := usageVault(t)
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "image", Bytes: 250000, Mod: "image"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "read", Ref: "https://x/y.pdf", Mod: "pdf"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "image", Bytes: 600000}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Hints(uctx, "pm", p, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	ids := hintIDs(out)
	if !ids["modality_image"] || !ids["screenshot_bloat"] {
		t.Fatalf("%v", ids)
	}
}

func TestUsageClasses_AndClassInvalid(t *testing.T) {
	s, p := usageVault(t)
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "tokens", In: intPtr(100), Out: intPtr(50), Class: "browser"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "tokens", In: intPtr(10), Out: intPtr(5)}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Classes(uctx, "pm", p, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	by := out["by_class"].(map[string]map[string]int)
	if by["browser"]["tokens"] != 150 || by["chat"]["tokens"] != 15 {
		t.Fatalf("%+v", by)
	}
	_, err = s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "tokens", In: intPtr(1), Out: intPtr(1), Class: "bogus"})
	if !errors.Is(err, ErrClassInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestUsageDigest_AndWeekLimitNear(t *testing.T) {
	s, p := usageVault(t)
	s.Now = func() time.Time { return time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC) }
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "tokens", In: intPtr(90000), Out: intPtr(5000)}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Digest(uctx, "pm", p, "2026-11-02", 100000, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if !out["near"].(bool) {
		t.Fatalf("expected near %+v", out["limit"])
	}
	out2, err := s.Digest(uctx, "pm", p, "2026-11-02", 500000, 10, false)
	if err != nil || out2["near"].(bool) {
		t.Fatalf("false near %+v", out2["limit"])
	}
	md := DigestMarkdown(out)
	if len(md) > digestMarkdownCap || !strings.Contains(md, "2026-11-02") {
		t.Fatal(md)
	}
	budget, _ := json.Marshal(weekBudget{WeekTokens: 200000, NearPct: 10})
	if _, err := s.V.Put(uctx, "pm", vault.PutInput{Project: p, Kind: "usage", Key: BudgetKey(), Title: "budget", Body: string(budget)}); err != nil {
		t.Fatal(err)
	}
	out3, err := s.Digest(uctx, "pm", p, "2026-11-02", 0, 0, true)
	if err != nil || out3["saved"] == nil {
		t.Fatalf("%+v %v", out3, err)
	}
}

func hintIDs(out map[string]any) map[string]bool {
	m := map[string]bool{}
	for _, h := range out["hints"].([]Hint) {
		m[h.ID] = true
	}
	return m
}
