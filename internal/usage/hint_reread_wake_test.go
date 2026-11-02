package usage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

func TestHintRereadWake_Threshold(t *testing.T) {
	s, p := usageVault(t)
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return base }
	for i := 0; i < 5; i++ {
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "wake", N: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "msg", N: 1}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := s.Hints(uctx, "pm", p, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range out["hints"].([]Hint) {
		if h.ID == "reread_wake" {
			t.Fatalf("below threshold should omit hint score=%v", h.Score)
		}
	}
}

func TestHintRereadWake_History(t *testing.T) {
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
	var h *Hint
	for i := range out["hints"].([]Hint) {
		if out["hints"].([]Hint)[i].ID == "reread_wake" {
			h = &out["hints"].([]Hint)[i]
			break
		}
	}
	if h == nil || h.Score < rereadWakeMinScore || h.Evidence == "" || h.Suggest == "" {
		t.Fatalf("%+v", h)
	}
}

func TestHintRereadWake_Proxy(t *testing.T) {
	s, p := usageVault(t)
	for i := 0; i < 12; i++ {
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "wake", N: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 18; i++ {
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "msg", N: 1}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := s.Hints(uctx, "pm", p, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !hintIDs(out)["reread_wake"] {
		t.Fatal("expected reread_wake for proxy-only high wakes/turns")
	}
}

func TestHintRereadWake_Quiet(t *testing.T) {
	s, p := usageVault(t)
	out, err := s.Hints(uctx, "pm", p, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range out["hints"].([]Hint) {
		if h.ID == "reread_wake" {
			t.Fatal("quiet log")
		}
	}
}

func logPollSpread(s *Service, p, ref string, n int) {
	base := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		s.Now = func() time.Time { return base.Add(time.Duration(i%3) * time.Hour) }
		if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "b", Type: "recheck", N: 1, Ref: ref}); err != nil {
			panic(err)
		}
	}
}

func TestPollToWatch_AndWatchSuggestDryRun(t *testing.T) {
	s, p := usageVault(t)
	ref := "https://status.example.com/poll"
	logPollSpread(s, p, ref, 8)
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
	sug, err := s.WatchSuggest(uctx, "pm", p, "", time.Time{}, false, false)
	if err != nil || sug["dry_run"] != true {
		t.Fatalf("%+v %v", sug, err)
	}
	_, _ = s.InitWatchRules(uctx, "pm", p)
	before, _ := s.V.Get(uctx, "pm", p, WatchRulesKey(), 0)
	sugApply, err := s.WatchSuggest(uctx, "pm", p, "", time.Time{}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sugApply["applied"]; ok {
		t.Fatal("--apply without --i-know must not apply")
	}
	after, _ := s.V.Get(uctx, "pm", p, WatchRulesKey(), 0)
	if after.Body != before.Body {
		t.Fatal("apply without i-know changed watch.rules")
	}
}

func TestWatchSuggest_ApplyRequiresIKnow(t *testing.T) {
	s, p := usageVault(t)
	logPollSpread(s, p, "https://a.example/x", 8)
	_, _ = s.InitWatchRules(uctx, "pm", p)
	before, _ := s.V.Get(uctx, "pm", p, WatchRulesKey(), 0)
	sug, err := s.WatchSuggest(uctx, "pm", p, "", time.Time{}, true, true)
	if err != nil || sug["applied"] != true {
		t.Fatalf("%+v %v", sug, err)
	}
	after, _ := s.V.Get(uctx, "pm", p, WatchRulesKey(), 0)
	if after.Body == before.Body {
		t.Fatal("expected rules update with apply+i_know")
	}
}

func TestResetPack_OutDirAndCompare(t *testing.T) {
	s, p := usageVault(t)
	dir := t.TempDir()
	if _, err := s.V.Put(uctx, "pm", vault.PutInput{Project: p, Kind: "handoff", Key: "handoff.main", Title: "H", Body: "keep-me"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "oldbot", Type: "tokens", In: intPtr(100), Out: intPtr(0)}); err != nil {
		t.Fatal(err)
	}
	pack, err := s.ResetPack(uctx, "pm", p, ResetPackInput{Bot: "oldbot", OutDir: dir})
	if err != nil || pack["note_count"].(int) < 1 {
		t.Fatalf("%+v %v", pack, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "must-keep", "handoff.main.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "before.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "RESET.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Log(uctx, "pm", p, LogInput{Bot: "newbot", Type: "tokens", In: intPtr(50), Out: intPtr(0)}); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResetCompare(uctx, "pm", p, ResetCompareInput{
		BeforePath: filepath.Join(dir, "before.json"), Bot: "newbot",
	})
	if err != nil {
		t.Fatal(err)
	}
	s3, p3 := usageVault(t)
	_, err = s3.ResetCompare(uctx, "pm", p3, ResetCompareInput{})
	if !errors.Is(err, ErrResetIncomplete) {
		t.Fatalf("got %v", err)
	}
}
