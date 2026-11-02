package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/southline-tech/botoptimizer/internal/vault"
)

type projectArg struct {
	Project string `json:"project"`
}

type logToolArgs struct {
	projectArg
	Bot   string  `json:"bot"`
	Type  string  `json:"type"`
	In    *int    `json:"in"`
	Out   *int    `json:"out"`
	Cache int     `json:"cache"`
	Model string  `json:"model"`
	Cost  float64 `json:"cost"`
	N     int     `json:"n"`
	Bytes int     `json:"bytes"`
	Tool  string  `json:"tool"`
	PR    string  `json:"pr"`
	Key   string  `json:"key"`
	Ref   string  `json:"ref"`
	Class string  `json:"class"`
	Mod   string  `json:"mod"`
	Hist  int     `json:"hist"`
}

type importToolArgs struct {
	projectArg
	Date  string `json:"date"`
	Body  string `json:"body"`
	Merge bool   `json:"merge"`
}

type csvToolArgs struct {
	projectArg
	Date string  `json:"date"`
	CSV  string  `json:"csv"`
	Body string  `json:"body"`
	TZ   string  `json:"tz"`
	Map  *CSVMap `json:"map,omitempty"`
}

type watchCheckArgs struct {
	projectArg
	Bot   string `json:"bot"`
	Since string `json:"since"`
}

type chartToolArgs struct {
	projectArg
	Range   string `json:"range"`
	Out     string `json:"out"`
	At      string `json:"at"`
	Explain bool   `json:"explain"`
}

type hintsToolArgs struct {
	projectArg
	Bot   string `json:"bot"`
	Since string `json:"since"`
}

type classesToolArgs struct {
	projectArg
	Bot   string `json:"bot"`
	Since string `json:"since"`
}

type watchSuggestArgs struct {
	projectArg
	Bot   string `json:"bot"`
	Since string `json:"since"`
	Apply bool   `json:"apply"`
	IKnow bool   `json:"i_know"`
}

type digestToolArgs struct {
	projectArg
	Date      string  `json:"date"`
	WeekLimit int     `json:"week_limit"`
	NearPct   float64 `json:"near_pct"`
	Save      bool    `json:"save"`
}

type resetPackArgs struct {
	projectArg
	Bot       string `json:"bot"`
	OutDir    string `json:"out_dir"`
	SaveVault bool   `json:"save_vault"`
}

type resetCompareArgs struct {
	projectArg
	BeforePath string `json:"before_path"`
	Bot        string `json:"bot"`
}

// ValidateUsageLogMap checks CLI-assembled usage_log args before MCP call.
func ValidateUsageLogMap(a map[string]any) error {
	typ, _ := a["type"].(string)
	if strings.TrimSpace(typ) != "tokens" {
		return nil
	}
	_, inOK := a["in"]
	_, outOK := a["out"]
	if !inOK || !outOK {
		return fmt.Errorf("%w: tokens require in and out", ErrInvalid)
	}
	return nil
}

func decodeTool(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// CallTool runs one usage MCP tool.
func CallTool(ctx context.Context, v *vault.Vault, authBot, name string, raw json.RawMessage, resolveProject func(string) (string, error)) (any, error) {
	svc := &Service{V: v}
	switch name {
	case "usage_log":
		var a logToolArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		return svc.Log(ctx, authBot, project, LogInput{
			Bot: a.Bot, Type: a.Type, In: a.In, Out: a.Out, Cache: a.Cache,
			Model: a.Model, Cost: a.Cost, N: a.N, Bytes: a.Bytes, Tool: a.Tool, PR: a.PR, Key: a.Key, Ref: a.Ref,
			Class: a.Class, Mod: a.Mod, Hist: a.Hist,
		})
	case "usage_import":
		var a importToolArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		if a.Date == "" {
			return nil, fmt.Errorf("%w: date required", ErrInvalid)
		}
		var doc ShareDoc
		src := strings.TrimSpace(a.Body)
		if src == "" {
			return nil, fmt.Errorf("%w: body required", ErrInvalid)
		}
		if err := json.Unmarshal([]byte(src), &doc); err != nil {
			return nil, fmt.Errorf("%w: invalid share json", ErrInvalid)
		}
		return svc.ImportShare(ctx, authBot, project, a.Date, doc, a.Merge)
	case "usage_import_csv":
		var a csvToolArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		if a.Date == "" {
			return nil, fmt.Errorf("%w: date required", ErrInvalid)
		}
		r := strings.NewReader(a.CSV)
		if a.CSV == "" && a.Body != "" {
			r = strings.NewReader(a.Body)
		}
		override := CSVMap{}
		if a.Map != nil {
			override = *a.Map
		}
		return svc.ImportCSV(ctx, authBot, project, a.Date, a.TZ, r, override)
	case "watch_check":
		var a watchCheckArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		since := time.Time{}
		if strings.TrimSpace(a.Since) != "" {
			since, err = vault.ParseSince(a.Since)
			if err != nil {
				return nil, err
			}
		}
		return svc.WatchCheck(ctx, authBot, project, a.Bot, since)
	case "watch_init":
		var a projectArg
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		return svc.InitWatchRules(ctx, authBot, project)
	case "usage_chart":
		var a chartToolArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		if a.Explain && a.At != "" {
			return svc.ExplainAt(ctx, authBot, project, a.At)
		}
		html, meta, err := svc.ChartHTML(ctx, authBot, project, a.Range)
		if err != nil {
			return nil, err
		}
		if a.Out != "" {
			if err := os.WriteFile(a.Out, []byte(html), 0644); err != nil {
				return nil, err
			}
			meta["ok"] = true
			meta["out"] = a.Out
			return meta, nil
		}
		meta["html"] = html
		return meta, nil
	case "usage_hints":
		var a hintsToolArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		since := time.Time{}
		if strings.TrimSpace(a.Since) != "" {
			since, err = vault.ParseSince(a.Since)
			if err != nil {
				return nil, err
			}
		}
		return svc.Hints(ctx, authBot, project, a.Bot, since)
	case "usage_classes":
		var a classesToolArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		since := time.Time{}
		if strings.TrimSpace(a.Since) != "" {
			since, err = vault.ParseSince(a.Since)
			if err != nil {
				return nil, err
			}
		}
		return svc.Classes(ctx, authBot, project, a.Bot, since)
	case "watch_suggest":
		var a watchSuggestArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		since := time.Time{}
		if strings.TrimSpace(a.Since) != "" {
			since, err = vault.ParseSince(a.Since)
			if err != nil {
				return nil, err
			}
		}
		return svc.WatchSuggest(ctx, authBot, project, a.Bot, since, a.Apply, a.IKnow)
	case "usage_digest":
		var a digestToolArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		return svc.Digest(ctx, authBot, project, a.Date, a.WeekLimit, a.NearPct, a.Save)
	case "memory_reset_pack":
		var a resetPackArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		return svc.ResetPack(ctx, authBot, project, ResetPackInput{
			Bot: a.Bot, OutDir: a.OutDir, SaveVault: a.SaveVault,
		})
	case "memory_reset_compare":
		var a resetCompareArgs
		if err := decodeTool(raw, &a); err != nil {
			return nil, fmt.Errorf("%w: %v", vault.ErrInvalid, err)
		}
		project, err := resolveProject(a.Project)
		if err != nil {
			return nil, err
		}
		return svc.ResetCompare(ctx, authBot, project, ResetCompareInput{
			BeforePath: a.BeforePath, Bot: a.Bot,
		})
	default:
		return nil, fmt.Errorf("unknown usage tool %q", name)
	}
}

// ImportShareFromReader reads JSON share doc from r.
func ImportShareFromReader(r io.Reader) (ShareDoc, error) {
	var doc ShareDoc
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return doc, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return doc, nil
}
