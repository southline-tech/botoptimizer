package usage

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Hint struct {
	ID      string  `json:"id"`
	Bot     string  `json:"bot,omitempty"`
	Message string  `json:"message"`
	Score   float64 `json:"score,omitempty"`
}

func (s *Service) Hints(ctx context.Context, authBot, project, botFilter string, since time.Time) (map[string]any, error) {
	if since.IsZero() {
		since = s.now().Add(-24 * time.Hour)
	}
	events, err := s.loadEvents(ctx, authBot, project, botFilter, since)
	if err != nil {
		return nil, err
	}
	var hints []Hint
	if h := detectRecheckLoop(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectScreenshotBloat(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectStatusChatter(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectDupRead(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectRetryLoop(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectTokenSpike(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectRereadWake(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectPollToWatch(events); h != nil {
		hints = append(hints, *h)
	}
	if h := detectModalityImage(events); h != nil {
		hints = append(hints, *h)
	}
	return map[string]any{"hints": hints, "count": len(hints)}, nil
}

func detectRecheckLoop(ev []Event) *Hint {
	n := 0
	for _, e := range ev {
		if e.Type == "recheck" {
			n += max(1, e.N)
		}
	}
	if n >= 10 {
		return &Hint{ID: "recheck_loop", Message: fmt.Sprintf("%d rechecks in window", n)}
	}
	return nil
}

func detectScreenshotBloat(ev []Event) *Hint {
	for _, e := range ev {
		if e.Type == "image" && e.Bytes >= 500000 {
			return &Hint{ID: "screenshot_bloat", Bot: e.Bot, Message: "large image proxy bytes"}
		}
	}
	return nil
}

func detectStatusChatter(ev []Event) *Hint {
	n := 0
	for _, e := range ev {
		if e.Type == "msg" {
			n += max(1, e.N)
		}
	}
	if n >= 20 {
		return &Hint{ID: "status_chatter", Message: fmt.Sprintf("%d status messages", n)}
	}
	return nil
}

func detectDupRead(ev []Event) *Hint {
	seen := map[string]int{}
	for _, e := range ev {
		if e.Type != "read" || e.Key == "" {
			continue
		}
		k := e.Bot + "|" + e.Key
		seen[k]++
		if seen[k] >= 5 {
			return &Hint{ID: "dup_read", Bot: e.Bot, Message: "repeated read on " + e.Key}
		}
	}
	return nil
}

func detectRetryLoop(ev []Event) *Hint {
	n := 0
	for _, e := range ev {
		if e.Type == "retry" {
			n += max(1, e.N)
		}
	}
	if n >= 5 {
		return &Hint{ID: "retry_loop", Message: fmt.Sprintf("%d retries", n)}
	}
	return nil
}

func detectTokenSpike(ev []Event) *Hint {
	var vals []int
	for _, e := range ev {
		if e.Type == "tokens" {
			vals = append(vals, e.In+e.Out+e.Cache)
		}
	}
	if len(vals) < 3 {
		return nil
	}
	sum := 0
	for _, v := range vals[:len(vals)-1] {
		sum += v
	}
	avg := sum / (len(vals) - 1)
	last := vals[len(vals)-1]
	if avg > 0 && last > avg*3 {
		return &Hint{ID: "token_spike", Message: fmt.Sprintf("last=%d avg=%d", last, avg)}
	}
	return nil
}

func detectRereadWake(ev []Event) *Hint {
	wakes, turns, hist := 0, 0, 0
	for _, e := range ev {
		if e.Type == "wake" {
			wakes += max(1, e.N)
		}
		if e.Type == "msg" {
			turns += max(1, e.N)
		}
		if e.Hist > hist {
			hist = e.Hist
		}
	}
	product := wakes * max(1, turns)
	if product < 6 {
		return nil
	}
	denom := 60.0
	if hist > 0 {
		denom = float64(hist)
		if denom < float64(product) {
			denom = float64(product)
		}
	}
	score := float64(product) / denom
	if score > 1 {
		score = 1
	}
	return &Hint{
		ID: "reread_wake", Message: fmt.Sprintf("wake×turn=%d score=%.2f", product, score), Score: score,
	}
}

var urlRefRE = regexp.MustCompile(`(?i)^https?://`)

func detectPollToWatch(ev []Event) *Hint {
	n := 0
	for _, e := range ev {
		if e.Ref == "" || !urlRefRE.MatchString(e.Ref) {
			continue
		}
		if e.Type == "recheck" || e.Type == "read" || e.Type == "retry" {
			n += max(1, e.N)
		}
	}
	if n < 3 {
		return nil
	}
	score := float64(n) / 10.0
	if score > 1 {
		score = 1
	}
	return &Hint{ID: "poll_to_watch", Message: fmt.Sprintf("%d url polls; prefer watch", n), Score: score}
}

func detectModalityImage(ev []Event) *Hint {
	var bytes, images, pdfs int
	for _, e := range ev {
		if e.Mod == "pdf" || strings.HasSuffix(strings.ToLower(e.Ref), ".pdf") {
			pdfs++
		}
		if e.Type == "image" || e.Mod == "image" {
			images += max(1, e.N)
			bytes += e.Bytes
		}
	}
	if bytes < 200000 && images < 3 && pdfs < 2 {
		return nil
	}
	return &Hint{
		ID: "modality_image", Message: "image/pdf heavy; prefer text crop or OCR",
	}
}
