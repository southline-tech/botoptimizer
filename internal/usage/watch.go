package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

type WatchRule struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"` // tokens | proxy
	Type       string  `json:"type,omitempty"`
	Window     string  `json:"window"`
	MaxTokens  int     `json:"max_tokens,omitempty"`
	Multiplier float64 `json:"multiplier,omitempty"`
	MaxN       int     `json:"max_n,omitempty"`
	NearPct    float64 `json:"near_pct,omitempty"`
	Message    string  `json:"message,omitempty"`
}

type WatchAlert struct {
	ID      string `json:"id"`
	Bot     string `json:"bot,omitempty"`
	Message string `json:"message"`
}

func DefaultWatchRules() []WatchRule {
	return []WatchRule{
		{ID: "tok-day", Kind: "tokens", Window: "24h", MaxTokens: 500000, Message: "daily token budget exceeded"},
		{ID: "tok-spike", Kind: "tokens", Window: "1h", Multiplier: 3, Message: "hourly token spike"},
		{ID: "proxy-wake", Kind: "proxy", Type: "wake", Window: "1h", MaxN: 50, Message: "excessive wakes"},
		{ID: "proxy-recheck", Kind: "proxy", Type: "recheck", Window: "1h", MaxN: 30, Message: "recheck storm"},
	}
}

func (s *Service) InitWatchRules(ctx context.Context, authBot, project string) (map[string]any, error) {
	rules := DefaultWatchRules()
	b, _ := json.Marshal(rules)
	key := WatchRulesKey()
	prev, err := s.V.Get(ctx, authBot, project, key, 0)
	if err == nil {
		return map[string]any{"ok": true, "key": key, "version": prev.Version, "created": false}, nil
	}
	if err != vault.ErrNotFound {
		return nil, err
	}
	rec, err := s.V.Put(ctx, authBot, vault.PutInput{
		Project: project, Kind: "note", Key: key, Title: "watch rules", Body: string(b),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "key": key, "version": rec.Version, "created": true}, nil
}

func (s *Service) loadWatchRules(ctx context.Context, authBot, project string) ([]WatchRule, error) {
	rec, err := s.V.Get(ctx, authBot, project, WatchRulesKey(), 0)
	if err == vault.ErrNotFound {
		return DefaultWatchRules(), nil
	}
	if err != nil {
		return nil, err
	}
	var rules []WatchRule
	if json.Unmarshal([]byte(rec.Body), &rules) != nil || len(rules) == 0 {
		return DefaultWatchRules(), nil
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Kind == rules[j].Kind {
			return rules[i].ID < rules[j].ID
		}
		if rules[i].Kind == "tokens" {
			return true
		}
		if rules[j].Kind == "tokens" {
			return false
		}
		return rules[i].ID < rules[j].ID
	})
	return rules, nil
}

func parseWindow(w string) (time.Duration, error) {
	w = strings.TrimSpace(w)
	if w == "" {
		return 0, fmt.Errorf("%w: window required", ErrInvalid)
	}
	if strings.HasSuffix(w, "h") {
		n, err := time.ParseDuration(w)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	if strings.HasSuffix(w, "d") {
		n, err := time.ParseDuration(strings.TrimSuffix(w, "d") + "h")
		if err != nil {
			return 0, err
		}
		return n * 24, nil
	}
	return time.ParseDuration(w)
}

func (s *Service) WatchCheck(ctx context.Context, authBot, project, botFilter string, since time.Time) (map[string]any, error) {
	rules, err := s.loadWatchRules(ctx, authBot, project)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if since.IsZero() {
		since = now.Add(-24 * time.Hour)
	}
	events, err := s.loadEvents(ctx, authBot, project, botFilter, since)
	if err != nil {
		return nil, err
	}
	var alerts []WatchAlert
	for _, rule := range rules {
		win, err := parseWindow(rule.Window)
		if err != nil {
			continue
		}
		cut := now.Add(-win)
		if rule.Kind == "tokens" {
			byBot := map[string]int{}
			var series []int
			for _, ev := range events {
				if ev.Type != "tokens" || ev.At.Before(cut) {
					continue
				}
				tok := ev.In + ev.Out + ev.Cache
				byBot[ev.Bot] += tok
				series = append(series, tok)
			}
			for bot, sum := range byBot {
				if botFilter != "" && bot != botFilter {
					continue
				}
				if rule.MaxTokens > 0 && sum > rule.MaxTokens {
					alerts = append(alerts, WatchAlert{ID: rule.ID, Bot: bot, Message: rule.Message})
				}
			}
			if rule.Multiplier > 0 && len(series) > 2 {
				avg := 0
				for _, v := range series {
					avg += v
				}
				avg /= len(series)
				last := series[len(series)-1]
				if avg > 0 && float64(last) > float64(avg)*rule.Multiplier {
					alerts = append(alerts, WatchAlert{ID: rule.ID, Message: rule.Message})
				}
			}
			continue
		}
		if rule.Kind == "proxy" {
			count := 0
			for _, ev := range events {
				if ev.At.Before(cut) || ev.Type != rule.Type {
					continue
				}
				if botFilter != "" && ev.Bot != botFilter {
					continue
				}
				n := ev.N
				if n == 0 {
					n = 1
				}
				count += n
			}
			if rule.MaxN > 0 && count > rule.MaxN {
				alerts = append(alerts, WatchAlert{ID: rule.ID, Message: rule.Message})
			}
		}
	}
	return map[string]any{"alerts": alerts, "count": len(alerts)}, nil
}

type watchSuggestion struct {
	ID      string `json:"id"`
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

func pollURLRefs(events []Event) []watchSuggestion {
	seen := map[string]struct{}{}
	var out []watchSuggestion
	for _, e := range events {
		if e.Ref == "" || !urlRefRE.MatchString(e.Ref) {
			continue
		}
		if e.Type != "recheck" && e.Type != "read" && e.Type != "retry" {
			continue
		}
		if _, ok := seen[e.Ref]; ok {
			continue
		}
		seen[e.Ref] = struct{}{}
		out = append(out, watchSuggestion{
			ID: "poll_to_watch", Ref: e.Ref, Message: "consider watch instead of polling",
		})
	}
	return out
}

// WatchSuggest returns polling→watch suggestions; apply writes watch.rules only when apply is true.
func (s *Service) WatchSuggest(ctx context.Context, authBot, project, botFilter string, since time.Time, apply bool) (map[string]any, error) {
	if since.IsZero() {
		since = s.now().Add(-24 * time.Hour)
	}
	events, err := s.loadEvents(ctx, authBot, project, botFilter, since)
	if err != nil {
		return nil, err
	}
	suggestions := pollURLRefs(events)
	out := map[string]any{
		"dry_run": !apply, "suggestions": suggestions, "count": len(suggestions),
	}
	if !apply || len(suggestions) == 0 {
		return out, nil
	}
	rules, err := s.loadWatchRules(ctx, authBot, project)
	if err != nil {
		return nil, err
	}
	for _, sug := range suggestions {
		id := "poll-" + sanitizeKV(strings.TrimPrefix(strings.TrimPrefix(sug.Ref, "https://"), "http://"))
		if len(id) > 40 {
			id = id[:40]
		}
		rules = append(rules, WatchRule{
			ID: id, Kind: "proxy", Type: "recheck", Window: "1h", MaxN: 5,
			Message: "url poll budget for " + sug.Ref,
		})
	}
	b, _ := json.Marshal(rules)
	rec, err := s.V.Get(ctx, authBot, project, WatchRulesKey(), 0)
	if err != nil && err != vault.ErrNotFound {
		return nil, err
	}
	if err == vault.ErrNotFound {
		_, err = s.V.Put(ctx, authBot, vault.PutInput{
			Project: project, Kind: "note", Key: WatchRulesKey(), Title: "watch rules", Body: string(b),
		})
	} else {
		ver := rec.Version
		_, err = s.V.Put(ctx, authBot, vault.PutInput{
			Project: project, Key: WatchRulesKey(), Body: string(b), IfVersion: &ver,
		})
	}
	if err != nil {
		return nil, err
	}
	out["applied"] = true
	return out, nil
}
