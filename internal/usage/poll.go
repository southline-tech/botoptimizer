package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const pollMinEvents = 8
const pollMinDistinctHours = 3

func pollEventOK(e Event) bool {
	if e.Ref == "" || !urlRefRE.MatchString(e.Ref) {
		return false
	}
	return e.Type == "recheck" || e.Type == "read" || e.Type == "retry"
}

func urlFingerprint(ref string) string {
	sum := sha256.Sum256([]byte(ref))
	return hex.EncodeToString(sum[:8])
}

func pollStatsByRef(events []Event) map[string]struct {
	count int
	hours map[int64]struct{}
} {
	out := map[string]struct {
		count int
		hours map[int64]struct{}
	}{}
	for _, e := range events {
		if !pollEventOK(e) {
			continue
		}
		st := out[e.Ref]
		st.count += max(1, e.N)
		if st.hours == nil {
			st.hours = map[int64]struct{}{}
		}
		h := e.At.UTC().Truncate(time.Hour).Unix()
		st.hours[h] = struct{}{}
		out[e.Ref] = st
	}
	return out
}

func refQualifiesForPoll(st struct {
	count int
	hours map[int64]struct{}
}) bool {
	return st.count >= pollMinEvents && len(st.hours) >= pollMinDistinctHours
}
