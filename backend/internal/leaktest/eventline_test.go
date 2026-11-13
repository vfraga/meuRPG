package leaktest

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Two vitals events that are the same but for the instant they were written read the same once the times are
// blanked, whatever the digits of the fraction: a time on a whole millisecond prints three digits, one that is not
// prints six, and an event cut at a fixed length would then end at another place of the same text.
func TestAnEventLineReadsTheSameWhateverTheDigitsOfItsTime(t *testing.T) {
	event := func(at time.Time) *playv1.WatchGameSessionResponse {
		res := make([]*playv1.ResourceUsage, 0, 12)
		for range 12 {
			res = append(res, &playv1.ResourceUsage{Key: "ki", NamePt: "Chi", Total: 5, Used: 1, Recharge: rulesv1.Recharge_RECHARGE_SHORT_REST})
		}
		return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_VitalsChanged_{
			VitalsChanged: &playv1.WatchGameSessionResponse_VitalsChanged{Vitals: &playv1.CharacterVitals{
				CharacterId: "11111111-1111-1111-1111-111111111111", Name: "Toren", UpdatedAt: timestamppb.New(at), Resources: res,
			}},
		}}
	}
	whole := time.Date(2026, 11, 7, 14, 53, 1, 123_000_000, time.UTC)
	fine := time.Date(2026, 11, 7, 14, 53, 1, 123_456_000, time.UTC)
	a, b := normalizeRead(eventLine("vitals_changed", event(whole))), normalizeRead(eventLine("vitals_changed", event(fine)))
	if a != b {
		t.Errorf("the same event reads differently by the digits of its time:\n%s\n%s", a, b)
	}
	if n := len(a); n < 500 || !strings.HasSuffix(a, "}}}") {
		t.Errorf("the line is cut (%d bytes, ends %q), the test would prove nothing", n, a[max(n-8, 0):])
	}
}
