package progression

import (
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// The handler's own check of the treasures stands behind the request size limit of the field.
func TestCheckTreasureIDsRefusesMoreThanTheLimit(t *testing.T) {
	t.Parallel()
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = uuid.NewSHA1(uuid.NameSpaceOID, []byte(strconv.Itoa(i))).String()
		}
		return out
	}
	if got, err := checkTreasureIDs(ids(maxAwardTreasures)); err != nil || len(got) != maxAwardTreasures {
		t.Errorf("checkTreasureIDs(%d) = %d ids, %v; want all, nil", maxAwardTreasures, len(got), err)
	}
	_, err := checkTreasureIDs(ids(maxAwardTreasures + 1))
	if connect.CodeOf(err) != connect.CodeInvalidArgument || err == nil || !strings.Contains(err.Error(), "treasure_point_ids must have at most 100 treasures") {
		t.Errorf("checkTreasureIDs(101) error = %v, want invalid_argument: treasure_point_ids must have at most 100 treasures", err)
	}
}
