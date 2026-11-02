package usage

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

const resetExportCap = 32

func ResetBeforeKey() string { return "usage.reset.before" }

var mustKeepKinds = map[string]struct{}{
	"handoff": {}, "spec": {}, "decision": {},
}

type resetNote struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Version int    `json:"version"`
	Summary string `json:"summary,omitempty"`
}

func usageSnapshot(events []Event, at time.Time) map[string]any {
	byBot := map[string]int{}
	total := 0
	for _, ev := range events {
		if ev.Type != "tokens" {
			continue
		}
		tok := ev.In + ev.Out + ev.Cache
		byBot[ev.Bot] += tok
		total += tok
	}
	return map[string]any{"tokens_by_bot": byBot, "tokens_total": total, "at": at.UTC().Format(time.RFC3339)}
}

func (s *Service) ResetPack(ctx context.Context, authBot, project string) (map[string]any, error) {
	entries, err := s.V.List(ctx, authBot, project, vault.ListFilter{})
	if err != nil {
		return nil, err
	}
	var notes []resetNote
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
		if len(notes) >= resetExportCap {
			break
		}
	}
	since := s.now().Add(-7 * 24 * time.Hour)
	events, err := s.loadEvents(ctx, authBot, project, "", since)
	if err != nil {
		return nil, err
	}
	before := usageSnapshot(events, s.now())
	checklist := strings.TrimSpace(`# RESET checklist (BotOptimizer)

1. Export must-keep notes (handoff, spec, decision) — done by reset-pack.
2. Snapshot usage before reset — stored at usage.reset.before.
3. Trim context / start fresh session; re-read brief + must-keep keys only.
4. After reset, run memory reset-compare to see token delta.
5. No vault deletes in this flow — archive manually if needed.
`)
	pack := map[string]any{
		"ok": true, "notes": notes, "note_count": len(notes), "capped": len(notes) >= resetExportCap,
		"usage_before": before, "reset_md": checklist,
	}
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
	return pack, nil
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

func (s *Service) ResetCompare(ctx context.Context, authBot, project string) (map[string]any, error) {
	rec, err := s.V.Get(ctx, authBot, project, ResetBeforeKey(), 0)
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
	since := s.now().Add(-7 * 24 * time.Hour)
	events, err := s.loadEvents(ctx, authBot, project, "", since)
	if err != nil {
		return nil, err
	}
	after := usageSnapshot(events, s.now())
	delta := tokenTotalFromSnap(after) - tokenTotalFromSnap(before)
	return map[string]any{
		"ok": true, "before": before, "after": after, "delta_tokens": delta,
	}, nil
}
