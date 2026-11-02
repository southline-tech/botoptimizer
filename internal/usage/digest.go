package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

const digestMarkdownCap = 8192

type weekBudget struct {
	WeekTokens int     `json:"week_tokens"`
	NearPct    float64 `json:"near_pct"`
}

func BudgetKey() string { return "usage.budget.week" }

func digestKey(date string) (string, error) {
	if _, err := parseDateOnly(date); err != nil {
		return "", err
	}
	return "usage.digest." + date, nil
}

func (s *Service) loadWeekBudget(ctx context.Context, authBot, project string, weekLimit int) (weekBudget, error) {
	b := weekBudget{NearPct: 10}
	if weekLimit > 0 {
		b.WeekTokens = weekLimit
		return b, nil
	}
	rec, err := s.V.Get(ctx, authBot, project, BudgetKey(), 0)
	if err == vault.ErrNotFound {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	_ = json.Unmarshal([]byte(rec.Body), &b)
	if b.NearPct <= 0 {
		b.NearPct = 10
	}
	return b, nil
}

func weekStartUTC(now time.Time) time.Time {
	wd := int(now.UTC().Weekday())
	if wd == 0 {
		wd = 7
	}
	d := now.UTC().AddDate(0, 0, -(wd - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) Digest(ctx context.Context, authBot, project, date string, weekLimit int, nearPct float64, save bool) (map[string]any, error) {
	now := s.now()
	if date == "" {
		date = now.Format("2006-01-02")
	}
	dayStart, err := parseDateOnly(date)
	if err != nil {
		return nil, err
	}
	dayEnd := dayStart.Add(24 * time.Hour)
	events, err := s.loadEvents(ctx, authBot, project, "", dayStart)
	if err != nil {
		return nil, err
	}
	var dayEvents []Event
	for _, ev := range events {
		if ev.At.Before(dayStart) || !ev.At.Before(dayEnd) {
			continue
		}
		dayEvents = append(dayEvents, ev)
	}
	bots := map[string]int{}
	for _, ev := range dayEvents {
		if ev.Type == "tokens" {
			bots[ev.Bot] += ev.In + ev.Out + ev.Cache
		}
	}
	hintOut, err := s.Hints(ctx, authBot, project, "", dayStart)
	if err != nil {
		return nil, err
	}
	classOut, err := s.Classes(ctx, authBot, project, "", dayStart)
	if err != nil {
		return nil, err
	}
	suggest, err := s.WatchSuggest(ctx, authBot, project, "", dayStart, false)
	if err != nil {
		return nil, err
	}
	watchOut, err := s.WatchCheck(ctx, authBot, project, "", dayStart)
	if err != nil {
		return nil, err
	}

	budget, err := s.loadWeekBudget(ctx, authBot, project, weekLimit)
	if err != nil {
		return nil, err
	}
	if nearPct > 0 {
		budget.NearPct = nearPct
	}
	weekTok := 0
	weekEv, err := s.loadEvents(ctx, authBot, project, "", weekStartUTC(now))
	if err != nil {
		return nil, err
	}
	for _, ev := range weekEv {
		if ev.Type == "tokens" {
			weekTok += ev.In + ev.Out + ev.Cache
		}
	}
	limit := map[string]any{
		"week_tokens_used":   weekTok,
		"week_tokens_limit":  budget.WeekTokens,
		"near_pct_threshold": budget.NearPct,
	}
	near := false
	if budget.WeekTokens > 0 {
		remaining := budget.WeekTokens - weekTok
		remainingPct := float64(remaining) * 100 / float64(budget.WeekTokens)
		limit["remaining_pct"] = remainingPct
		if remainingPct <= budget.NearPct {
			near = true
			limit["near"] = true
			limit["warning"] = "LIMIT_NEAR"
		}
	}

	out := map[string]any{
		"date":             date,
		"bots":             bots,
		"hints":            hintOut["hints"],
		"classes":          classOut["by_class"],
		"poll_suggestions": suggest["suggestions"],
		"watch_alerts":     watchOut["alerts"],
		"limit":            limit,
		"near":             near,
	}
	if save {
		key, err := digestKey(date)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(out)
		prev, gerr := s.V.Get(ctx, authBot, project, key, 0)
		if gerr == vault.ErrNotFound {
			rec, err := s.V.Put(ctx, authBot, vault.PutInput{
				Project: project, Kind: "usage", Key: key, Title: "usage digest " + date, Body: string(b),
			})
			if err != nil {
				return nil, err
			}
			out["saved"] = map[string]any{"key": key, "version": rec.Version}
		} else if gerr != nil {
			return nil, gerr
		} else {
			ver := prev.Version
			rec, err := s.V.Put(ctx, authBot, vault.PutInput{
				Project: project, Key: key, Title: "usage digest " + date, Body: string(b), IfVersion: &ver,
			})
			if err != nil {
				return nil, err
			}
			out["saved"] = map[string]any{"key": key, "version": rec.Version}
		}
	}
	return out, nil
}

func DigestMarkdown(d map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Usage digest %v\n\n", d["date"])
	if bots, ok := d["bots"].(map[string]int); ok && len(bots) > 0 {
		b.WriteString("## Bots (tokens)\n")
		for bot, tok := range bots {
			fmt.Fprintf(&b, "- %s: %d\n", bot, tok)
		}
		b.WriteString("\n")
	}
	if lim, ok := d["limit"].(map[string]any); ok {
		b.WriteString("## Weekly limit\n")
		for _, k := range []string{"week_tokens_used", "week_tokens_limit", "remaining_pct", "warning"} {
			if v, ok := lim[k]; ok {
				fmt.Fprintf(&b, "- %s: %v\n", k, v)
			}
		}
	}
	s := b.String()
	if len(s) > digestMarkdownCap {
		s = s[:digestMarkdownCap]
	}
	return s
}
