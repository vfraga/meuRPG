package characters

import (
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The handler's own checks of sizes stand behind the request size limits of the fields: called
// directly, they refuse what the limits refuse first when a request comes through the interceptor.

func TestCheckSwitchRequestRefusesMoreThanTheLimit(t *testing.T) {
	t.Parallel()
	switches := func(n int) []*rulesv1.OptionSwitch {
		out := make([]*rulesv1.OptionSwitch, n)
		for i := range out {
			out[i] = &rulesv1.OptionSwitch{Key: "spell:s" + strconv.Itoa(i), Off: true}
		}
		return out
	}
	if err := checkSwitchRequest(switches(MaxOptionSwitches)); err != nil {
		t.Errorf("checkSwitchRequest(%d) error = %v, want nil", MaxOptionSwitches, err)
	}
	err := checkSwitchRequest(switches(MaxOptionSwitches + 1))
	if connect.CodeOf(err) != connect.CodeInvalidArgument || err == nil || !strings.Contains(err.Error(), "switches must have at most 1400 switches") {
		t.Errorf("checkSwitchRequest(1401) error = %v, want invalid_argument: switches must have at most 1400 switches", err)
	}
}

func TestReadPackJSONRefusesAFileOverTheLimit(t *testing.T) {
	t.Parallel()
	pack, _, top := readPackJSON([]byte(strings.Repeat(" ", MaxPackBytes+1)))
	if pack != nil || len(top) != 1 || top[0].GetField() != "pack" || top[0].GetReason() != reasonSizeLimit {
		t.Errorf("readPackJSON(3 MiB + 1) = %v, %v; want no pack and a size_limit violation at pack", pack, top)
	}
}
