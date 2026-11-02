package usage

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

func TestRereadWake_ScoreAndQuiet(t *testing.T) {
	s, p := usageVault(t)
	for i := 0; i < 5; i++ {
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "wake", N: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "msg", N: 1, Hist: 30}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := s.Hints(uctx, "pm", p, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range out["hints"].([]Hint) {
		if h.ID == "reread_wake" {
			found = true
			if h.Score <= 0 || h.Score > 1 {
				t.Fatalf("score %v", h.Score)
			}
		}
	}
	if !found {
		t.Fatal("missing reread_wake")
	}
	s2, p2 := usageVault(t)
	out2, _ := s2.Hints(uctx, "pm", p2, "", time.Time{})
	for _, h := range out2["hints"].([]Hint) {
		if h.ID == "reread_wake" {
			t.Fatal("quiet log should not emit reread_wake")
		}
	}
}

func TestPollToWatch_AndWatchSuggestDryRun(t *testing.T) {
	s, p := usageVault(t)
	ref := "https://status.example.com/poll"
	for i := 0; i < 4; i++ {
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "recheck", N: 1, Ref: ref}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "read", Key: "local", Ref: "file:///tmp/x"}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Hints(uctx, "pm", p, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !hintIDs(out)["poll_to_watch"] {
		t.Fatal(out)
	}
	sug, err := s.WatchSuggest(uctx, "pm", p, "", time.Time{}, false)
	if err != nil || sug["dry_run"] != true || sug["count"].(int) < 1 {
		t.Fatalf("%+v %v", sug, err)
	}
	_, _ = s.InitWatchRules(uctx, "pm", p)
	before, _ := s.V.Get(uctx, "pm", p, WatchRulesKey(), 0)
	sug2, err := s.WatchSuggest(uctx, "pm", p, "", time.Time{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sug2["applied"]; ok {
		t.Fatal("dry-run must not apply")
	}
	after, _ := s.V.Get(uctx, "pm", p, WatchRulesKey(), 0)
	if after.Body != before.Body {
		t.Fatal("dry-run changed watch.rules")
	}
}

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

func TestResetPack_AndResetCompare(t *testing.T) {
	s, p := usageVault(t)
	if _, err := s.V.Put(uctx, "pm", vault.PutInput{Project: p, Kind: "handoff", Key: "handoff.main", Title: "H", Body: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "tokens", In: intPtr(100), Out: intPtr(0)}); err != nil {
		t.Fatal(err)
	}
	pack, err := s.ResetPack(uctx, "pm", p)
	if err != nil || pack["note_count"].(int) < 1 || pack["reset_md"].(string) == "" {
		t.Fatalf("%+v %v", pack, err)
	}
	_, err = s.ResetCompare(uctx, "pm", p)
	if err != nil {
		t.Fatal(err)
	}
	s3, p3 := usageVault(t)
	_, err = s3.ResetCompare(uctx, "pm", p3)
	if !errors.Is(err, ErrResetIncomplete) {
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
