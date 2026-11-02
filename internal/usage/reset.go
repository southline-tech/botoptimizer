package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

const resetExportCap = 50

func ResetBeforeKey() string { return "usage.reset.before" }

var mustKeepKinds = map[string]struct{}{
	"handoff": {}, "spec": {}, "decision": {},
}

type ResetPackInput struct {
	Bot       string
	OutDir    string
	SaveVault bool
}

type ResetCompareInput struct {
	BeforePath string
	Bot        string
}

type resetNote struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Version int    `json:"version"`
	Summary string `json:"summary,omitempty"`
}

func usageSnapshot(events []Event, at time.Time, botFilter string) map[string]any {
	byBot := map[string]int{}
	total := 0
	for _, ev := range events {
		if ev.Type != "tokens" {
			continue
		}
		if botFilter != "" && ev.Bot != botFilter {
			continue
		}
		tok := ev.In + ev.Out + ev.Cache
		byBot[ev.Bot] += tok
		total += tok
	}
	out := map[string]any{"tokens_by_bot": byBot, "tokens_total": total, "at": at.UTC().Format(time.RFC3339)}
	if botFilter != "" {
		out["bot"] = botFilter
	}
	return out
}

func resetChecklistMD() string {
	return strings.TrimSpace(`# RESET checklist (BotOptimizer)

1. Export must-keep notes (handoff, spec, decision) — under must-keep/.
2. Usage snapshot before reset — before.json.
3. Trim context / start fresh session; re-read brief + must-keep keys only.
4. After reset, run memory reset-compare with --before before.json --bot NEW.
5. No vault deletes in this flow — archive manually if needed.
`)
}

func safeMustKeepName(key string) string {
	key = strings.ReplaceAll(key, "/", "_")
	if key == "" {
		key = "note"
	}
	return key + ".md"
}

func (s *Service) ResetPack(ctx context.Context, authBot, project string, in ResetPackInput) (map[string]any, error) {
	if in.Bot != "" {
		if err := vault.ValidName("bot", in.Bot); err != nil {
			return nil, err
		}
	}
	entries, err := s.V.List(ctx, authBot, project, vault.ListFilter{})
	if err != nil {
		return nil, err
	}
	var notes []resetNote
	var noteKeys []string
	for _, e := range entries {
		if _, ok := mustKeepKinds[e.Kind]; !ok {
			continue
		}
		if e.Status == vault.StatusArchived {
			continue
		}
		notes = append(notes, resetNote{
			Key: e.Key, Kind: e.Kind, Title: e.Title, Version: e.Version, Summary: e.Summary,
		})
		noteKeys = append(noteKeys, e.Key)
		if len(notes) >= resetExportCap {
			break
		}
	}
	since := s.now().Add(-7 * 24 * time.Hour)
	events, err := s.loadEvents(ctx, authBot, project, in.Bot, since)
	if err != nil {
		return nil, err
	}
	before := usageSnapshot(events, s.now(), in.Bot)
	checklist := resetChecklistMD()
	pack := map[string]any{
		"ok": true, "notes": notes, "note_count": len(notes), "capped": len(notes) >= resetExportCap,
		"usage_before": before, "reset_md": checklist,
	}
	if in.OutDir != "" {
		if err := writeResetPackDir(ctx, s.V, authBot, project, in.OutDir, noteKeys, before, checklist); err != nil {
			return nil, err
		}
		pack["out_dir"] = in.OutDir
	}
	if in.SaveVault {
		b, _ := json.Marshal(before)
		prev, gerr := s.V.Get(ctx, authBot, project, ResetBeforeKey(), 0)
		if gerr == vault.ErrNotFound {
			_, err = s.V.Put(ctx, authBot, vault.PutInput{
				Project: project, Kind: "usage", Key: ResetBeforeKey(), Title: "reset usage before", Body: string(b),
			})
		} else if gerr != nil {
			return nil, gerr
		} else {
			ver := prev.Version
			_, err = s.V.Put(ctx, authBot, vault.PutInput{
				Project: project, Key: ResetBeforeKey(), Body: string(b), IfVersion: &ver,
			})
		}
		if err != nil {
			return nil, err
		}
		pack["before_key"] = ResetBeforeKey()
	}
	return pack, nil
}

func writeResetPackDir(ctx context.Context, v *vault.Vault, authBot, project, outDir string, keys []string, before map[string]any, checklist string) error {
	mk := filepath.Join(outDir, "must-keep")
	if err := os.MkdirAll(mk, 0755); err != nil {
		return err
	}
	for _, key := range keys {
		rec, err := v.Get(ctx, authBot, project, key, 0)
		if err != nil {
			continue
		}
		name := safeMustKeepName(key)
		body := fmt.Sprintf("# %s (%s)\n\n%s", rec.Title, rec.Kind, rec.Body)
		if err := os.WriteFile(filepath.Join(mk, name), []byte(body), 0644); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(before, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "before.json"), b, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "RESET.md"), []byte(checklist), 0644)
}

func tokenTotalFromSnap(m map[string]any) int {
	switch v := m["tokens_total"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func loadBeforeSnapshot(ctx context.Context, v *vault.Vault, authBot, project, path string) (map[string]any, error) {
	if strings.TrimSpace(path) != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%w: read before file", ErrInvalid)
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil, ErrResetIncomplete
		}
		return m, nil
	}
	rec, err := v.Get(ctx, authBot, project, ResetBeforeKey(), 0)
	if err == vault.ErrNotFound {
		return nil, ErrResetIncomplete
	}
	if err != nil {
		return nil, err
	}
	var before map[string]any
	if json.Unmarshal([]byte(rec.Body), &before) != nil {
		return nil, ErrResetIncomplete
	}
	return before, nil
}

func (s *Service) ResetCompare(ctx context.Context, authBot, project string, in ResetCompareInput) (map[string]any, error) {
	if in.Bot != "" {
		if err := vault.ValidName("bot", in.Bot); err != nil {
			return nil, err
		}
	}
	before, err := loadBeforeSnapshot(ctx, s.V, authBot, project, in.BeforePath)
	if err != nil {
		return nil, err
	}
	since := s.now().Add(-7 * 24 * time.Hour)
	botFilter := in.Bot
	events, err := s.loadEvents(ctx, authBot, project, botFilter, since)
	if err != nil {
		return nil, err
	}
	after := usageSnapshot(events, s.now(), botFilter)
	delta := tokenTotalFromSnap(after) - tokenTotalFromSnap(before)
	return map[string]any{
		"ok": true, "before": before, "after": after, "delta_tokens": delta,
	}, nil
}
