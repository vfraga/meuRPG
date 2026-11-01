package reqlimit

import (
	"net/http"
	"strings"
	"sync"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
)

const (
	// DefaultReadLimit is the most bytes the body of a unary RPC may have, for a
	// procedure that raisedReadLimits does not name.
	DefaultReadLimit = 64 << 10
	// Backstop is the most any one RPC message may have, whatever the
	// procedure: the server's connect.WithReadMaxBytes. No read limit is above it.
	Backstop = 4 << 20
)

// The raised read limits, named once.
const (
	limit128KiB = 128 << 10
	limit256KiB = 256 << 10
	limit512KiB = 512 << 10
)

// raisedReadLimits are the procedures whose unary body may be larger than
// DefaultReadLimit, by procedure ("/meurpg.x.v1.Service/Method"), in bytes.
// Each entry has a comment that says why. TestRegistry requires an entry for
// every procedure that reaches a field raised over its defaults, and none
// above Backstop. Tests change it only through the lock (withRaised).
var (
	raisedMu         sync.RWMutex
	raisedReadLimits = map[string]int64{
		// pack_json is up to 3 MiB (characters.MaxPackBytes), and proto JSON writes a
		// bytes field in base64: the message may reach the backstop.
		rulesv1connect.TableContentServiceImportTableContentProcedure: Backstop,
		// body is up to 400 KiB, and the JSON of a text of line breaks is twice its size.
		campaignsv1connect.CampaignDocumentServiceUpdateCampaignDocumentProcedure: limit512KiB,
		// An entry's data is up to 64 KiB of JSON (characters.MaxTableEntryBytes) and the
		// text fields (desc_pt, higher_level_pt, text_pt) go up to 16 KiB each.
		rulesv1connect.TableContentServiceCreateTableEntryProcedure: limit128KiB,
		rulesv1connect.TableContentServiceUpdateTableEntryProcedure: limit128KiB,
		// switches is up to 1400 entries of a content key and a flag.
		rulesv1connect.TableContentServiceSetOptionSwitchesProcedure: limit256KiB,
		// A full sheet carries known_spell_keys of up to 200 items, known_issues and
		// content_baselines of up to 1000, and custom_features_text of up to 20 KiB. The
		// read limit must exceed the raised fields' limits.
		charactersv1connect.CharacterServiceCreateCharacterProcedure:  limit128KiB,
		charactersv1connect.CharacterServiceUpdateCharacterProcedure:  limit128KiB,
		charactersv1connect.CharacterServicePreviewCharacterProcedure: limit128KiB,
		charactersv1connect.CharacterServicePreviewChoicesProcedure:   limit128KiB,
		// CreateCharacter also carries the story: backstory is up to 40 KiB. For
		// UpdateCharacterStory it is the field that raises the limit.
		charactersv1connect.CharacterServiceUpdateCharacterStoryProcedure: limit128KiB,
		// notes is up to 80 KiB.
		charactersv1connect.CharacterServiceUpdateMasterNotesProcedure: limit128KiB,
		// known_spell_keys is up to 200 items. The read limit must exceed the raised
		// field's limit; a realistic choice is far below it.
		charactersv1connect.CharacterServiceLevelUpCharacterProcedure: limit128KiB,
		charactersv1connect.CharacterServicePreviewLevelUpProcedure:   limit128KiB,
		// hooks is up to 16 KiB; the read limit must exceed it.
		mapsv1connect.MapServiceCreateMapPointProcedure: limit128KiB,
		mapsv1connect.MapServiceUpdateMapPointProcedure: limit128KiB,
		// squares is up to 400 items; the read limit must exceed it.
		mapsv1connect.MapServicePaintMapCellsProcedure: limit128KiB,
	}
)

// ReadLimit is the most bytes the body of a unary request to the procedure may have.
func ReadLimit(procedure string) int64 {
	raisedMu.RLock()
	defer raisedMu.RUnlock()
	if n, ok := raisedReadLimits[procedure]; ok {
		return n
	}
	return DefaultReadLimit
}

// Middleware reads the body of a unary RPC through http.MaxBytesReader at
// ReadLimit(procedure). Connect reports a body over the limit as
// `resource_exhausted`. Put it outside slowclient.Middleware, so that the
// deadline's body stays the outermost one.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/meurpg.") && slowclient.IsUnaryRPC(r) {
			r.Body = http.MaxBytesReader(w, r.Body, ReadLimit(r.URL.Path))
		}
		next.ServeHTTP(w, r)
	})
}
