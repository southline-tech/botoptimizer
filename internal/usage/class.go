package usage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

var validWorkClasses = map[string]struct{}{
	"chat": {}, "browser": {}, "mcp": {}, "script": {},
}

func normalizeLogClass(class, typ string) (string, error) {
	class = strings.TrimSpace(class)
	if class == "" {
		if typ == "tokens" || typ == "msg" {
			return "chat", nil
		}
		return "", nil
	}
	if _, ok := validWorkClasses[class]; !ok {
		return "", fmt.Errorf("%w: class must be chat|browser|mcp|script", ErrClassInvalid)
	}
	return class, nil
}

func eventClass(ev Event) string {
	if ev.Class != "" {
		return ev.Class
	}
	if ev.Type == "tokens" || ev.Type == "msg" {
		return "chat"
	}
	return "other"
}

// Classes aggregates token and event counts by work class.
func (s *Service) Classes(ctx context.Context, authBot, project, botFilter string, since time.Time) (map[string]any, error) {
	if since.IsZero() {
		since = s.now().Add(-24 * time.Hour)
	}
	events, err := s.loadEvents(ctx, authBot, project, botFilter, since)
	if err != nil {
		return nil, err
	}
	by := map[string]map[string]int{}
	for _, ev := range events {
		cls := eventClass(ev)
		if by[cls] == nil {
			by[cls] = map[string]int{"tokens": 0, "events": 0}
		}
		by[cls]["events"]++
		if ev.Type == "tokens" {
			by[cls]["tokens"] += ev.In + ev.Out + ev.Cache
		}
	}
	totalTok := 0
	for _, m := range by {
		totalTok += m["tokens"]
	}
	var hint *Hint
	if totalTok > 0 {
		browser := by["browser"]["tokens"]
		if browser*100/totalTok >= 60 {
			hint = &Hint{ID: "browser_skew", Message: fmt.Sprintf("browser=%d%% of tokens", browser*100/totalTok)}
		}
	}
	out := map[string]any{"by_class": by, "window_since": since.UTC().Format(time.RFC3339)}
	if hint != nil {
		out["hint"] = hint
	}
	return out, nil
}
