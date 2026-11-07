package characters

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The players' "Magias" page (MR-045, RN-23): the table's spells, the SRD's and the
// master's together, found by name and filtered by class, circle and school. The
// filters are package rules'; this file checks who may ask, which spells a player
// may see and, for "Só as que posso aprender", which lists a character reads.

const (
	defaultSpellsPage = 100
	maxSpellsPage     = 400 // the page size the app asks for; a longer list comes in pages
	maxSpellQuery     = 100
	maxSpellFilters   = 20
)

var errBasicSheetCasts = errors.New("a basic sheet has no casting classes")

// ListSpells implements rulesv1connect.ContentServiceHandler.
func (s *Service) ListSpells(
	ctx context.Context,
	req *connect.Request[rulesv1.ListSpellsRequest],
) (*connect.Response[rulesv1.ListSpellsResponse], error) {
	// Active members only: a pending member has the catalog (ListContent) to build
	// a sheet and needs no reference page (RN-15).
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	msg := req.Msg
	page, err := readSpellsPage(msg)
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	if msg.GetCharacterId() != "" {
		if page.filter.Learnable, err = s.learnableLists(ctx, content, m, msg.GetCharacterId()); err != nil {
			return nil, err
		}
	}
	master := isMaster(m)
	if !master {
		unknown, err := s.spellClassUnknown(ctx, content, m, page.filter.Class)
		if err != nil {
			return nil, err
		}
		if unknown {
			return connect.NewResponse(&rulesv1.ListSpellsResponse{ContentVersion: content.Version()}), nil
		}
		// A player never reads a spell the master retired or switched off ("Opções
		// para os jogadores"): one test, Content.Hidden.
		page.filter.Hidden = content.Hidden
	}
	res, err := spellsPageOf(content, page, master, spellsFilterID(msg))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(res), nil
}

const (
	// lowestSpellCircle and highestSpellCircle bound a circle in the filters: 0 for the
	// cantrips, up to the 9th.
	lowestSpellCircle  = 0
	highestSpellCircle = 9
)

// spellsPage is a ListSpells request whose shape is checked: the filters, the page
// size and the offset of its first row.
type spellsPage struct {
	filter rules.SpellFilter
	size   int
	offset int
}

// readSpellsPage checks what needs no database, in order: the query, the number of
// circles and of schools, each circle, the page size and the token.
func readSpellsPage(msg *rulesv1.ListSpellsRequest) (spellsPage, error) {
	if utf8.RuneCountInString(msg.GetQuery()) > maxSpellQuery {
		return spellsPage{}, invalidArgument(fieldErr("query", "must have at most %d characters", maxSpellQuery))
	}
	if len(msg.GetLevels()) > maxSpellFilters {
		return spellsPage{}, invalidArgument(fieldErr("levels", "must have at most %d entries", maxSpellFilters))
	}
	if len(msg.GetSchoolKeys()) > maxSpellFilters {
		return spellsPage{}, invalidArgument(fieldErr("school_keys", "must have at most %d entries", maxSpellFilters))
	}
	filter := rules.SpellFilter{Query: msg.GetQuery(), Class: msg.GetClassKey(), Schools: msg.GetSchoolKeys()}
	for _, l := range msg.GetLevels() {
		if l < lowestSpellCircle || l > highestSpellCircle {
			return spellsPage{}, invalidArgument(fieldErr("levels", "must be 0 to 9"))
		}
		filter.Levels = append(filter.Levels, int(l))
	}
	size, err := spellsPageSize(msg.GetPageSize())
	if err != nil {
		return spellsPage{}, err
	}
	offset := 0
	if token := msg.GetPageToken(); token != "" {
		var ok bool
		if offset, ok = parseSpellsToken(token, spellsFilterID(msg)); !ok {
			return spellsPage{}, invalidArgument(fieldErr("page_token", "is not a token of this list"))
		}
	}
	return spellsPage{filter: filter, size: size, offset: offset}, nil
}

// spellsPageSize is the page size asked for, the default for none.
func spellsPageSize(size int32) (int, error) {
	switch {
	case size == 0:
		return defaultSpellsPage, nil
	case size < 0 || size > maxSpellsPage:
		return 0, invalidArgument(fieldErr("page_size", "must be 1 to %d", maxSpellsPage))
	}
	return int(size), nil
}

// spellClassUnknown says whether a player's class filter names a class they may not
// see: it answers as an unknown one (an empty list), so a guessed key is never
// confirmed; unless their own sheet uses the class.
func (s *Service) spellClassUnknown(ctx context.Context, content *rules.Content, m authz.Membership, class string) (bool, error) {
	if class == "" || !content.Hidden(class) {
		return false, nil
	}
	uses, err := s.callerUses(ctx, m, content, class)
	if err != nil {
		return false, s.dbError(ctx, "read the caller's sheets", err)
	}
	return !uses, nil
}

// spellsPageOf is the page of the list the filters find, and the token of the next one
// (filters is their fingerprint). A token past the end of the list is refused.
func spellsPageOf(content *rules.Content, page spellsPage, master bool, filters string) (*rulesv1.ListSpellsResponse, error) {
	list := content.ListSpells(page.filter)
	res := &rulesv1.ListSpellsResponse{Total: i32(len(list)), ContentVersion: content.Version()}
	if page.offset > len(list) {
		return nil, invalidArgument(fieldErr("page_token", "is not a token of this list"))
	}
	end := min(page.offset+page.size, len(list))
	for _, e := range list[page.offset:end] {
		sp := spellToProto(e)
		if !master {
			// The classes whose list has the spell: a hidden one is not named to a player.
			sp.ClassKeys = slices.DeleteFunc(slices.Clone(sp.GetClassKeys()), content.Hidden)
		}
		res.Spells = append(res.Spells, sp)
	}
	if end < len(list) {
		res.NextPageToken = spellsToken(end, filters)
	}
	return res, nil
}

// learnableLists are the spell lists a character reads, each with the highest
// circle that class casts: one for every casting class of the sheet (a third
// caster's subclass reads the list of the class it casts from). The caller must
// be able to see the character; a basic sheet (an NPC) casts nothing.
func (s *Service) learnableLists(ctx context.Context, content *rules.Content, m authz.Membership, characterID string) ([]rules.SpellAccess, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return nil, errCharacterNotFound()
	}
	row, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return nil, s.dbError(ctx, "get a character", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return nil, errCharacterNotFound()
	}
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, s.dbError(ctx, "read a character", err)
	}
	if sheet.GetFull() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errBasicSheetCasts)
	}
	d := rules.Derive(buildOf(sheet.GetFull()), content)
	out := []rules.SpellAccess{} // a character that casts learns nothing, and that is not "any"
	for _, sc := range d.Spellcasting {
		list := sc.SpellList
		if list == "" {
			list = sc.Class
		}
		out = append(out, rules.SpellAccess{List: list, MaxLevel: sc.MaxSpellLevel})
	}
	return out, nil
}

// spellsFilterID is a short fingerprint of the filters, so a page token only works
// for the list it came from.
func spellsFilterID(r *rulesv1.ListSpellsRequest) string {
	levels := make([]string, 0, len(r.GetLevels()))
	for _, l := range r.GetLevels() {
		levels = append(levels, strconv.Itoa(int(l)))
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		r.GetQuery(), r.GetClassKey(), strings.Join(levels, ","), strings.Join(r.GetSchoolKeys(), ","), r.GetCharacterId(),
	}, "\x00")))
	return hex.EncodeToString(sum[:6])
}

// The page token is the offset of the next row and the fingerprint of the filters,
// as opaque text (the same shape as ListCreatures').
func spellsToken(offset int, filters string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("s" + strconv.Itoa(offset) + ":" + filters))
}

func parseSpellsToken(token, filters string) (int, bool) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, false
	}
	rest, found := strings.CutPrefix(string(b), "s")
	n, got, hasFilters := strings.Cut(rest, ":")
	offset, err := strconv.Atoi(n)
	return offset, found && hasFilters && got == filters && err == nil && offset >= 0
}
