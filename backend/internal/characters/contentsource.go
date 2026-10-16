package characters

import (
	"context"
	"slices"
	"sync"
	"weak"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// ContentSource gives the rules content and the table rules a campaign plays
// with (MR-025, RN-23, RN-24; ADR-0018). Every read of the content in this
// package goes through it, so the table's own content (slice 10.1c) can differ
// per campaign without touching the call sites again.
type ContentSource interface {
	// For returns the rules content and the table rules of a campaign. tx is
	// the caller's transaction (nil outside one): an implementation that reads
	// the campaign's content revision does it in tx, never through the pool
	// while a transaction is open (PR #121's rule).
	For(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, TableRules, error)
	// ContentFor is For for a caller that only needs the content: it skips the read
	// of the table rules, which is a database read per call.
	ContentFor(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, error)
}

// TableRules are the rules a table chooses (RN-23, RN-24): the shared value of
// package tablerules. Its zero value is the SRD defaults, so a campaign that
// chose nothing is TableRules{}.
type TableRules = tablerules.Rules

// HitPointsRule is how a level-up's hit points are decided. The zero value is the
// SRD's: the player chooses.
type HitPointsRule = tablerules.HitPointsRule

// The hit points rules.
const (
	// HitPointsPlayerChooses lets the player pick rolling or the average.
	HitPointsPlayerChooses = tablerules.HitPointsPlayerChooses
	// HitPointsRoll makes everybody roll; the average is refused.
	HitPointsRoll = tablerules.HitPointsRoll
	// HitPointsAverage makes everybody take the average; a roll is refused.
	HitPointsAverage = tablerules.HitPointsAverage
)

// AbilityMethods is a set of ways of making ability scores.
type AbilityMethods = tablerules.AbilityMethods

// DefaultTableRules returns the SRD defaults: the zero value.
func DefaultTableRules() TableRules { return TableRules{} }

// SRDSource is the ContentSource that gives every campaign the one SRD
// content and the default table rules. It never touches the database.
type SRDSource struct{ content *rules.Content }

// NewSRDSource returns the SRD source over content (rules.LoadSRD).
func NewSRDSource(content *rules.Content) SRDSource { return SRDSource{content: content} }

// ContentFor implements ContentSource.
func (s SRDSource) ContentFor(context.Context, pgx.Tx, string) (*rules.Content, error) {
	return s.content, nil
}

// For implements ContentSource.
func (s SRDSource) For(context.Context, pgx.Tx, string) (*rules.Content, TableRules, error) {
	return s.content, DefaultTableRules(), nil
}

// TableRulesReader reads the rules a table saved (package campaigns implements
// it: campaigns.Service.StoredTableRules).
type TableRulesReader interface {
	// StoredTableRules returns the table rules of a campaign, the defaults when
	// it never saved any. It reads inside tx when the caller has one (nil: the
	// pool).
	StoredTableRules(ctx context.Context, tx pgx.Tx, campaignID string) (TableRules, error)
}

// contentFor is the content of a campaign, read in tx (nil outside one), without
// the table rules (a caller that needs them asks s.content.For).
func (s *Service) contentFor(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, error) {
	return s.content.ContentFor(ctx, tx, campaignID)
}

// TableRulesFor is the table rules of a campaign, read in tx (nil outside one).
// Package play reads the critical and the death saves through it in slice 10.4b
// (cmd/api connects them), and nothing else needs the content with it.
func (s *Service) TableRulesFor(ctx context.Context, tx pgx.Tx, campaignID string) (TableRules, error) {
	_, r, err := s.content.For(ctx, tx, campaignID)
	return r, err
}

// maxCatalogs is how many contents' catalogs are kept: the plan's budget of
// cached contents (D1). The oldest is dropped first, so the catalogs of the
// contents a table edited away do not pile up.
const maxCatalogs = 8

// catalogCache is ListContent's answer per content, built on first use and
// bounded to maxCatalogs. It holds each content weakly: a catalog never keeps
// alive a content the live source has let go, so the memory a table's text takes
// is the live source's budget alone (maxLiveBytes), not that plus eight more.
type catalogCache struct {
	mu    sync.Mutex
	order []weak.Pointer[rules.Content] // oldest first
	// catalog is the master's catalog (every entry); players is the same without
	// the archived entries, made when a player first asks.
	catalog map[weak.Pointer[rules.Content]]*rulesv1.Content
	players map[weak.Pointer[rules.Content]]*rulesv1.Content
}

// drop forgets a content's catalogs. The caller holds mu.
func (cc *catalogCache) drop(w weak.Pointer[rules.Content]) {
	delete(cc.catalog, w)
	delete(cc.players, w)
}

// catalogFor is ListContent's answer for a content: the same content gives the
// same catalog, built once (while it is among the last maxCatalogs and alive).
func (s *Service) catalogFor(c *rules.Content, master bool) *rulesv1.Content {
	cc := &s.catalogs
	cc.mu.Lock()
	defer cc.mu.Unlock()
	w := weak.Make(c)
	v, ok := cc.catalog[w]
	if !ok {
		v = catalogToProto(c.Catalog())
		if cc.catalog == nil {
			cc.catalog, cc.players = map[weak.Pointer[rules.Content]]*rulesv1.Content{}, map[weak.Pointer[rules.Content]]*rulesv1.Content{}
		}
		cc.order = slices.DeleteFunc(cc.order, func(o weak.Pointer[rules.Content]) bool {
			if o.Value() != nil {
				return false
			}
			cc.drop(o) // its content is gone
			return true
		})
		if len(cc.order) >= maxCatalogs {
			cc.drop(cc.order[0])
			cc.order = cc.order[1:]
		}
		cc.order = append(cc.order, w)
		cc.catalog[w] = v
	}
	if master {
		return v
	}
	p, ok := cc.players[w]
	if !ok {
		p = withoutArchived(v, nil)
		cc.players[w] = p
	}
	return p
}

// catalogKeeping is a player's catalog with the retired or switched-off entries
// that keep (the keys the player's own sheet uses) left in, still marked
// `archived` or `off`: the editor shows the sheet's current value, and never
// offers it as a new choice. Not cached: it depends on the sheet.
func (s *Service) catalogKeeping(c *rules.Content, keep map[string]bool) *rulesv1.Content {
	master := s.catalogFor(c, true)
	if !c.AnyHidden() || len(keep) == 0 {
		return s.catalogFor(c, false)
	}
	return withoutArchived(master, keep)
}

// withoutArchived is a copy of a catalog without the entries the table retired or
// the master switched off, for the players (RN-23). It clones the whole message and filters the six lists,
// so a field a later slice adds to Content is in the players' catalog too and can
// never silently vanish. (A class's list of subclasses lives in the SRD content,
// so it needs no change here.)
func withoutArchived(c *rulesv1.Content, keep map[string]bool) *rulesv1.Content {
	out := proto.CloneOf(c)
	// The archived keys: nothing a player receives may name one (RN-23), not even
	// through a reference.
	f := catalogFilter{keep: keep, archived: map[string]bool{}}
	f.dropRetired(out)
	f.dropOrphans(out)
	f.dropReferences(out)
	return out
}

// catalogFilter takes out of a players' catalog what they never receive. archived fills up
// as the lists are filtered: the keys taken out.
type catalogFilter struct {
	keep     map[string]bool
	archived map[string]bool
}

// gone says whether an entry goes, and marks it: a retired one goes unless a player's own
// sheet uses it (then it stays, marked).
func (f catalogFilter) gone(key string, retired bool) bool {
	g := retired && !f.keep[key]
	f.archived[key] = f.archived[key] || g
	return g
}

// dropRetired takes the archived and switched off entries out of the six lists.
func (f catalogFilter) dropRetired(out *rulesv1.Content) {
	out.Races = slices.DeleteFunc(out.Races, func(r *rulesv1.Race) bool { return f.gone(r.GetKey(), r.GetArchived() || r.GetOff()) })
	out.Subraces = slices.DeleteFunc(out.Subraces, func(r *rulesv1.Subrace) bool { return f.gone(r.GetKey(), r.GetArchived() || r.GetOff()) })
	out.Classes = slices.DeleteFunc(out.Classes, func(r *rulesv1.CharacterClass) bool { return f.gone(r.GetKey(), r.GetArchived() || r.GetOff()) })
	out.Subclasses = slices.DeleteFunc(out.Subclasses, func(r *rulesv1.Subclass) bool { return f.gone(r.GetKey(), r.GetArchived() || r.GetOff()) })
	out.Backgrounds = slices.DeleteFunc(out.Backgrounds, func(r *rulesv1.Background) bool { return f.gone(r.GetKey(), r.GetArchived() || r.GetOff()) })
	out.Spells = slices.DeleteFunc(out.Spells, func(r *rulesv1.Spell) bool { return f.gone(r.GetKey(), r.GetArchived() || r.GetOff()) })
}

// dropOrphans takes out an entry whose required parent was taken out.
func (f catalogFilter) dropOrphans(out *rulesv1.Content) {
	out.Subclasses = slices.DeleteFunc(out.Subclasses, func(r *rulesv1.Subclass) bool { return f.archived[r.GetClassKey()] })
	out.Subraces = slices.DeleteFunc(out.Subraces, func(r *rulesv1.Subrace) bool { return f.archived[r.GetRaceKey()] })
}

// dropReferences takes the keys taken out out of what is left: the classes of a spell, the
// list a class or a subclass casts from, and the spells a subclass always prepares.
func (f catalogFilter) dropReferences(out *rulesv1.Content) {
	for _, sp := range out.Spells {
		sp.ClassKeys = slices.DeleteFunc(sp.ClassKeys, func(k string) bool { return f.archived[k] })
	}
	for _, cl := range out.Classes {
		if cs := cl.GetSpellcasting(); cs != nil && f.archived[cs.GetListClassKey()] {
			cs.ListClassKey = ""
		}
	}
	for _, sub := range out.Subclasses {
		// Nor does a subclass name a hidden spell it always prepares.
		sub.AlwaysPrepared = slices.DeleteFunc(sub.AlwaysPrepared, func(ap *rulesv1.SubclassAlwaysPrepared) bool { return f.archived[ap.GetSpellKey()] })
		if cs := sub.GetSpellcasting(); cs != nil && f.archived[cs.GetListClassKey()] {
			cs.ListClassKey = ""
		}
	}
}
