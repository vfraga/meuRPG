package characters

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The master's switches, "Opções para os jogadores" (MR-025, RN-23, ADR-0018): per
// campaign, each class, subclass, race, subrace, background, spell and feat, the SRD's and
// the table's, is on or off for the players. Everything is on by default; the off
// set is campaign_content_off, read with the revision in the caller's transaction
// and added to the content by rules.Overlay.Off. Every change bumps the campaign's
// content revision (so the live content, cached by revision, is right) and sends
// the live hint content_changed.

// MaxOptionSwitches is the most options one SetOptionSwitches call may change: the
// SRD has 358 switchable options, and the table adds up to 1000 entries, so one call can switch a whole campaign off (1400 leaves room for the SRD to grow).
const MaxOptionSwitches = 1400

// errNoSwitchChange is what the transaction of SetOptionSwitches returns when
// nothing needs to change: it rolls the revision bump back.
var errNoSwitchChange = errors.New("no switch changes")

// publishContentChanged sends the live hint of a write to the table's content, after
// its commit. Without a live stream connected (tests, a server without play) it does
// nothing.
func (s *Service) publishContentChanged(campaignID string) {
	if s.live != nil {
		s.live.PublishContentChanged(campaignID)
	}
}

// playerUses counts, per content key, the campaign's player characters that use it
// (a key counts once per character), SRD and table keys alike.
func playerUses(sheets []charactersdb.ListCampaignSheetsRow, content *rules.Content) (map[string]int, error) {
	uses := map[string]int{}
	for _, row := range sheets {
		if row.Kind != kindPlayer {
			continue
		}
		sheet, err := loadSheet(row.ID, row.Sheet)
		if err != nil {
			return nil, err
		}
		if full := sheet.GetFull(); full != nil {
			seen := map[string]bool{}
			b := buildOf(full)
			// The table's class features are not a pick of the sheet: it has the ones of its classes and levels.
			for _, kf := range append(rules.BuildKeys(b), content.ClassFeatureKeys(b)...) {
				if !seen[kf.Key] {
					seen[kf.Key] = true
					uses[kf.Key]++
				}
			}
		}
	}
	return uses, nil
}

// optionEntries are the options of a content with their state. The table's entries
// are told by their key; the archived mark and the names come from the catalog.
func optionEntries(c *rules.Content, uses map[string]int) []*rulesv1.OptionSwitchEntry {
	cat := c.Catalog()
	var out []*rulesv1.OptionSwitchEntry
	add := func(kind rulesv1.TableContentKind, key, namePT, parent string, level int, archived, off bool) {
		out = append(out, &rulesv1.OptionSwitchEntry{
			Key: key, Kind: kind, NamePt: namePT, Table: rules.IsTableKey(key), Off: off, Archived: archived,
			Hidden: archived || c.Hidden(key), ParentKey: parent, Level: i32(level), CharactersUsing: i32(uses[key]),
		})
	}
	for _, e := range cat.Classes {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS, e.Key, e.NamePT, "", 0, e.Archived, e.Off)
	}
	for _, e := range cat.Subclasses {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS, e.Key, e.NamePT, e.Class, 0, e.Archived, e.Off)
	}
	for _, e := range cat.Races {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE, e.Key, e.NamePT, "", 0, e.Archived, e.Off)
	}
	for _, e := range cat.Subraces {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBRACE, e.Key, e.NamePT, e.Race, 0, e.Archived, e.Off)
	}
	for _, e := range cat.Backgrounds {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND, e.Key, e.NamePT, "", 0, e.Archived, e.Off)
	}
	for _, e := range cat.Spells {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL, e.Key, e.NamePT, "", e.Level, e.Archived, e.Off)
	}
	for _, e := range c.Feats() {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, e.Key, e.NamePT, "", 0, e.Archived, e.Off)
	}
	for _, e := range c.Options() {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION, e.Key, e.NamePT, "", 0, e.Archived, e.Off)
	}
	for _, e := range c.ClassFeatures() {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS_FEATURE, e.Key, e.NamePT, e.Class, e.Level, e.Archived, e.Off)
	}
	for _, e := range c.Creatures() {
		add(rulesv1.TableContentKind_TABLE_CONTENT_KIND_CREATURE, e.Key, e.NamePT, "", 0, e.Archived, e.Off)
	}
	slices.SortStableFunc(out, func(a, b *rulesv1.OptionSwitchEntry) int {
		if d := cmp.Compare(a.GetKind(), b.GetKind()); d != 0 {
			return d
		}
		if d := strings.Compare(a.GetNamePt(), b.GetNamePt()); d != 0 {
			return d
		}
		return strings.Compare(a.GetKey(), b.GetKey())
	})
	return out
}

// ListOptionSwitches implements rulesv1connect.TableContentServiceHandler.
func (s *Service) ListOptionSwitches(
	ctx context.Context,
	req *connect.Request[rulesv1.ListOptionSwitchesRequest],
) (*connect.Response[rulesv1.ListOptionSwitchesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	var res *rulesv1.ListOptionSwitchesResponse
	// One transaction: the revision, the content (with its off set) and the sheets
	// are one snapshot.
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		content, err := s.contentFor(ctx, tx, m.CampaignID)
		if err != nil {
			return err
		}
		sheets, err := q.ListCampaignSheets(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the campaign's sheets", err)
		}
		uses, err := playerUses(sheets, content)
		if err != nil {
			return err
		}
		res = &rulesv1.ListOptionSwitchesResponse{Options: optionEntries(content, uses), TableRevision: i32(content.TableRevision())}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "list the option switches", err)
	}
	return connect.NewResponse(res), nil
}

// SetOptionSwitches implements rulesv1connect.TableContentServiceHandler.
func (s *Service) SetOptionSwitches(
	ctx context.Context,
	req *connect.Request[rulesv1.SetOptionSwitchesRequest],
) (*connect.Response[rulesv1.SetOptionSwitchesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	switches := req.Msg.GetSwitches()
	if err := checkSwitchRequest(switches); err != nil {
		return nil, err
	}
	call := &switchCall{s: s, campaignID: m.CampaignID, switches: switches}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error { return call.run(ctx, tx) })
	if errors.Is(err, errNoSwitchChange) {
		rev, rerr := contentRevision(ctx, s.queries, m.CampaignID)
		if rerr != nil {
			return nil, s.dbError(ctx, "read the content revision", rerr)
		}
		return connect.NewResponse(&rulesv1.SetOptionSwitchesResponse{TableRevision: rev}), nil
	}
	if err != nil {
		return nil, s.dbError(ctx, "set the option switches", err)
	}
	if call.changed {
		logging.Event(ctx, s.logger, "content.option_switched", slog.Int("changed", int(call.res.GetChanged())), slog.Int("table_revision", int(call.res.GetTableRevision())))
		s.publishContentChanged(m.CampaignID)
	}
	return connect.NewResponse(call.res), nil
}

// checkSwitchRequest refuses, before the database is read, a request with no switch, too
// many, or a key that is there twice.
func checkSwitchRequest(switches []*rulesv1.OptionSwitch) error {
	switch {
	case len(switches) == 0:
		return invalidArgument(fieldErr("switches", "must have at least one switch"))
	case len(switches) > MaxOptionSwitches:
		return invalidArgument(fieldErr("switches", "must have at most %d switches", MaxOptionSwitches))
	}
	asked := make(map[string]bool, len(switches))
	for i, sw := range switches {
		if asked[sw.GetKey()] {
			return invalidArgument(fieldErr("switches", "switches[%d]: the key %q is there twice", i, sw.GetKey()))
		}
		asked[sw.GetKey()] = true
	}
	return nil
}

// switchCall is one SetOptionSwitches: the request and what its transaction found. The
// transaction's closure runs again when CockroachDB retries it, so run starts it all again.
type switchCall struct {
	s          *Service
	campaignID string
	switches   []*rulesv1.OptionSwitch

	res     *rulesv1.SetOptionSwitchesResponse
	changed bool
}

// run is the transaction of the call: read the content, check the keys, bump the revision,
// write the new off set, and answer what changed for the players.
func (c *switchCall) run(ctx context.Context, tx pgx.Tx) error {
	c.changed, c.res = false, nil
	q := c.s.queries.WithTx(tx)
	// The content is read first (the revision row joins the transaction's reads),
	// then the revision is bumped: a write that got in between makes this one
	// retry, and two switches of a campaign run one after the other.
	before, err := c.s.contentFor(ctx, tx, c.campaignID)
	if err != nil {
		return err
	}
	if err := checkSwitchable(before, c.switches); err != nil {
		return err
	}
	rev, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: c.campaignID, Now: c.s.now()})
	if err != nil {
		return wrap("bump the content revision", err)
	}
	current, err := q.ListContentOff(ctx, c.campaignID)
	if err != nil {
		return wrap("read the options switched off", err)
	}
	turnOff, turnOn := switchChanges(c.switches, current)
	if len(turnOff)+len(turnOn) == 0 {
		return errNoSwitchChange // nothing to write: roll the bump back
	}
	if err := c.write(ctx, q, turnOff, turnOn); err != nil {
		return err
	}
	after, err := c.contentAfter(ctx, q, rev, offAfter(current, turnOff, turnOn))
	if err != nil {
		return err
	}
	sheets, err := q.ListCampaignSheets(ctx, c.campaignID)
	if err != nil {
		return wrap("list the campaign's sheets", err)
	}
	uses, err := playerUses(sheets, after)
	if err != nil {
		return err
	}
	c.res = &rulesv1.SetOptionSwitchesResponse{
		TableRevision: rev, Changed: i32(len(turnOff) + len(turnOn)), Options: changedOptions(before, after, uses),
	}
	c.changed = true
	return nil
}

// checkSwitchable refuses a key that is not a switchable option of the content.
func checkSwitchable(content *rules.Content, switches []*rulesv1.OptionSwitch) error {
	for i, sw := range switches {
		if !content.Switchable(sw.GetKey()) {
			return invalidArgument(fieldErr("switches", "switches[%d].key: %q is not a class, subclass, race, subrace, background, spell or feat of this campaign", i, sw.GetKey()))
		}
	}
	return nil
}

// switchChanges are the keys the switches turn off and on, against the stored off set: a
// switch to the state its key already has changes nothing.
func switchChanges(switches []*rulesv1.OptionSwitch, current []string) (turnOff, turnOn []string) {
	for _, sw := range switches {
		isOff := slices.Contains(current, sw.GetKey())
		switch {
		case sw.GetOff() && !isOff:
			turnOff = append(turnOff, sw.GetKey())
		case !sw.GetOff() && isOff:
			turnOn = append(turnOn, sw.GetKey())
		}
	}
	return turnOff, turnOn
}

// write stores the keys switched off and deletes the ones switched on.
func (c *switchCall) write(ctx context.Context, q *charactersdb.Queries, turnOff, turnOn []string) error {
	if len(turnOff) > 0 {
		if err := q.InsertContentOff(ctx, charactersdb.InsertContentOffParams{CampaignID: c.campaignID, ContentKeys: turnOff, Now: c.s.now()}); err != nil {
			return wrap("switch options off", err)
		}
	}
	if len(turnOn) > 0 {
		if err := q.DeleteContentOff(ctx, charactersdb.DeleteContentOffParams{CampaignID: c.campaignID, ContentKeys: turnOn}); err != nil {
			return wrap("switch options on", err)
		}
	}
	return nil
}

// offAfter is the off set once the switches are written: the stored one without what is
// turned on, then what is turned off.
func offAfter(current, turnOff, turnOn []string) []string {
	var out []string
	for _, k := range current {
		if !slices.Contains(turnOn, k) {
			out = append(out, k)
		}
	}
	return append(out, turnOff...)
}

// contentAfter is the content as the players have it now. It is built here from the rows
// the transaction sees and the new off set, and never goes through the live source: a
// write never fills the cache (the transaction may still roll back, and the revision would
// then be served a content nobody committed).
func (c *switchCall) contentAfter(ctx context.Context, q *charactersdb.Queries, rev int32, off []string) (*rules.Content, error) {
	rows, err := q.ListCampaignContent(ctx, c.campaignID)
	if err != nil {
		return nil, wrap("list the table's content", err)
	}
	overlay, err := overlayOf(rows, int(rev))
	if err != nil {
		return nil, err
	}
	overlay.Off = append(overlay.Off, off...)
	after, err := c.s.srd.With(overlay)
	if err != nil {
		return nil, wrap("add the table's content to the rules", err)
	}
	return after, nil
}

// changedOptions are the options of after whose state changed for the players since
// before: a new one, or one switched, hidden or shown.
func changedOptions(before, after *rules.Content, uses map[string]int) []*rulesv1.OptionSwitchEntry {
	was := map[string]*rulesv1.OptionSwitchEntry{}
	for _, e := range optionEntries(before, nil) {
		was[e.GetKey()] = e
	}
	var out []*rulesv1.OptionSwitchEntry
	for _, e := range optionEntries(after, uses) {
		if w := was[e.GetKey()]; w == nil || w.GetOff() != e.GetOff() || w.GetHidden() != e.GetHidden() {
			out = append(out, e)
		}
	}
	return out
}
