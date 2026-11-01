package characters

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The characters module's share of the campaign package (MR-050): the table
// content (the entries and the options switched off), the NPCs and creatures
// with the master's notes about them, and the players' characters. Everything
// goes through the checks the calls that make these things by hand use
// (prepare and the overlay for the content, checkNewSheet, checkStory and
// names.Clean for the characters), against the table content the package
// itself brings, so a sheet that needs a class of the table is checked with
// that class.
//
// A character comes without what happened to it in play: no vitals, no
// level-up history, no creatures, no lock, no story release, no player.

// PackagePart returns the characters' part of the package.
func (s *Service) PackagePart() campaignpackage.Part { return &packagePart{s: s} }

type packagePart struct{ s *Service }

const (
	stashContent    = "characters.content"
	stashCharacters = "characters.characters"
	optionsEntry    = "content/options.json"
	nsCharacter     = "character"
)

// orderStep is the time between two things made from one package, so the
// lists keep the package's order.
const orderStep = time.Millisecond

// --- export -----------------------------------------------------------------

// Export implements campaignpackage.Part.
func (p *packagePart) Export(ctx context.Context, tx pgx.Tx, campaignID string, snap *campaignpackage.Snapshot) error {
	q := p.s.queries.WithTx(tx)
	rows, err := q.ListCampaignContent(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("list the table content: %w", err)
	}
	for _, r := range rows {
		e, err := decodeRow(r)
		if err != nil {
			return err
		}
		name := campaignpackage.EntryName("content", snap.Next("content"), ".json")
		if err := snap.AddMessage(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT, name, &pkgv1.PackageContent{Entry: entryToProto(e, 0)}); err != nil {
			return err
		}
	}
	off, err := q.ListContentOff(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("list the options switched off: %w", err)
	}
	if len(off) > 0 {
		if err := snap.AddMessage(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT_OPTIONS, optionsEntry, &pkgv1.PackageContentOptions{Disabled: off}); err != nil {
			return err
		}
	}
	return p.exportCharacters(ctx, q, campaignID, snap)
}

func (p *packagePart) exportCharacters(ctx context.Context, q *charactersdb.Queries, campaignID string, snap *campaignpackage.Snapshot) error {
	chars, err := q.ListCharactersForPackage(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("list the characters: %w", err)
	}
	noteRows, err := q.ListMasterNotesOfCampaign(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("list the master's notes: %w", err)
	}
	notes := map[string]string{}
	for _, n := range noteRows {
		notes[n.CharacterID] = n.Notes
	}
	for _, c := range chars {
		if err := exportCharacter(snap, c, notes[c.ID]); err != nil {
			return err
		}
	}
	return nil
}

// exportCharacter adds one character to the snapshot, as a player's character
// or an NPC, unless it is no part of the table to come. Its sheet and story are
// read first, so a stored document that cannot be read stops the export even
// for a character that would be left out.
func exportCharacter(snap *campaignpackage.Snapshot, c charactersdb.ListCharactersForPackageRow, notes string) error {
	sheet, err := loadSheet(c.ID, c.Sheet)
	if err != nil {
		return err
	}
	story, err := loadStory(c.ID, c.Story)
	if err != nil {
		return err
	}
	switch {
	case c.Kind == kindPlayer && c.Status != statusActive:
		return nil // a pending request or a dead character is not part of the table to come
	case c.Kind != kindPlayer && sheet.GetBasic().GetCombatOnly():
		return nil // the app's own copy of a creature put in combat: it is made again on first use
	case c.Kind == kindPlayer:
		name := campaignpackage.EntryName("characters", snap.Next("characters"), ".json")
		out := &pkgv1.PackageCharacter{Id: c.ID, Name: c.Name, Sheet: sheet, Story: story, MasterNotes: notes}
		return snap.AddMessage(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CHARACTER, name, out)
	}
	name := campaignpackage.EntryName("npcs", snap.Next("npcs"), ".json")
	out := &pkgv1.PackageNpc{Id: c.ID, Name: c.Name, Kind: kindFromDB[c.Kind], Sheet: sheet, Story: story, MasterNotes: notes}
	return snap.AddMessage(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_NPC, name, out)
}

// --- stage ------------------------------------------------------------------

// contentKeyShape is the key of a table entry: the kind, a colon, a slug and
// "@mesa". The slug is what the server makes from the name. The kinds come from
// tableKinds, so a kind the table can create is one a package can carry.
var contentKeyShape = regexp.MustCompile(`^(` + tableKindPrefixes() + `):([a-z0-9]+(?:-[a-z0-9]+)*)@mesa$`)

// tableKindPrefixes is the key prefixes of tableKinds as a regexp alternation.
func tableKindPrefixes() string {
	prefixes := make([]string, 0, len(tableKinds))
	for _, tk := range tableKinds {
		prefixes = append(prefixes, regexp.QuoteMeta(tk.prefix))
	}
	return strings.Join(prefixes, "|")
}

type stagedContent struct {
	entries []entryRow
	off     []string
	content *rules.Content
	// kinds the entries are of, by key, for the problems.
	names map[string]string
}

// contentReason is the problem a violation of the table content becomes.
func contentReason(reason string) pkgv1.PackageProblemReason {
	switch reason {
	case rules.ReasonReference, rules.ReasonKey:
		return pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_UNKNOWN_CONTENT
	case rules.ReasonLimit, reasonSizeLimit:
		return pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_LIMIT
	case reasonDuplicateName, rules.ReasonDuplicateKey:
		return pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_DUPLICATE
	default:
		return pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_INVALID
	}
}

// Stage implements campaignpackage.Part.
func (p *packagePart) Stage(_ context.Context, in *campaignpackage.Import) error {
	sc := p.stageContent(in)
	if sc == nil {
		// The content has problems: the characters cannot be judged against it.
		return nil
	}
	in.Set(stashContent, sc)
	return p.stageCharacters(in, sc)
}

// stageContent reads, checks and compiles the table content. A nil answer
// means a problem was recorded.
func (p *packagePart) stageContent(in *campaignpackage.Import) *stagedContent {
	files := in.Pkg.Entries(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT)
	if len(files) > rules.MaxOverlayEntries {
		in.Problem(packageContentKind, "", pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_LIMIT, rules.MaxOverlayEntries)
		return nil
	}
	sc, entriesGood := p.stageEntries(in, files)
	// The options are judged even when an entry was not, so one preview lists both.
	options, optionsGood := stageOptionsEntry(in)
	if !entriesGood || !optionsGood {
		return nil
	}
	if !p.compileStagedContent(in, sc) || !stageSwitchedOff(in, sc, options) {
		return nil
	}
	in.Counts.ContentEntries += int32(len(sc.entries)) //nolint:gosec // G115: at most 1000
	return sc
}

// packageContentKind is the kind of every problem of the table content.
const packageContentKind = pkgv1.PackageProblemKind_PACKAGE_PROBLEM_KIND_CONTENT

// stageEntries reads and checks every table entry of the package, in its order,
// and says whether all of them were good. Each one is stamped orderStep after the
// one before, so the lists keep the package's order.
func (p *packagePart) stageEntries(in *campaignpackage.Import, files []*pkgv1.PackageEntry) (*stagedContent, bool) {
	sc := &stagedContent{names: map[string]string{}}
	now := p.s.now()
	good := true
	seen := map[string]bool{}
	for i, f := range files {
		var pc pkgv1.PackageContent
		if !in.Read(f.GetPath(), &pc) {
			good = false
			continue
		}
		e, ok := p.stageEntry(in, pc.GetEntry(), now.Add(time.Duration(i)*orderStep), seen)
		if ok {
			// The rule of the editors: no other entry of the kind has the name.
			if v := duplicateName(sc.entries, e.kind, e.row.ContentKey, e.row.NamePt); v != nil {
				in.Problem(packageContentKind, e.row.NamePt, contentReason(v.GetReason()), 0)
				ok = false
			}
		}
		if !ok {
			good = false
			continue
		}
		sc.entries = append(sc.entries, e)
		sc.names[e.row.ContentKey] = e.row.NamePt
	}
	return sc, good
}

// stageOptionsEntry reads the options switched off: at most one entry, at its
// fixed path. It says false when a problem was recorded.
func stageOptionsEntry(in *campaignpackage.Import) (*pkgv1.PackageContentOptions, bool) {
	opts := in.Pkg.Entries(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT_OPTIONS)
	options := &pkgv1.PackageContentOptions{}
	if len(opts) > 1 || (len(opts) == 1 && opts[0].GetPath() != optionsEntry) {
		in.Problem(packageContentKind, "", pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_UNEXPECTED_ENTRY, 0)
		return options, false
	}
	if len(opts) == 1 && !in.Read(optionsEntry, options) {
		return options, false
	}
	return options, true
}

// compileStagedContent compiles the whole content at once, since an entry can
// depend on another (a subclass on its class, a spell list a class reuses). It
// says false when the rules refused it, with a problem for each violation.
func (p *packagePart) compileStagedContent(in *campaignpackage.Import, sc *stagedContent) bool {
	overlay, idx := overlayFromEntries(sc.entries, 1)
	for _, e := range sc.entries {
		overlay.Strict = append(overlay.Strict, e.row.ContentKey)
	}
	content, err := p.s.srd.With(overlay)
	if err != nil {
		for _, v := range violationsOf(err, idx, "") {
			name := sc.names[v.GetKey()]
			in.Problem(packageContentKind, name, contentReason(v.GetReason()), 0)
		}
		return false
	}
	sc.content = content
	return true
}

// stageSwitchedOff checks each option the package switches off against the
// compiled content: one it cannot switch, or one named twice, is a problem.
func stageSwitchedOff(in *campaignpackage.Import, sc *stagedContent, options *pkgv1.PackageContentOptions) bool {
	good := true
	for _, key := range options.GetDisabled() {
		if !sc.content.Switchable(key) || slices.Contains(sc.off, key) {
			in.Problem(packageContentKind, key, pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_UNKNOWN_CONTENT, 0)
			good = false
			continue
		}
		sc.off = append(sc.off, key)
	}
	return good
}

// stageEntry checks one entry of the table content the way a write of it does
// (shape, feature count and keys, size), and returns it as it will be stored.
func (p *packagePart) stageEntry(in *campaignpackage.Import, te *rulesv1.TableEntry, at time.Time, seen map[string]bool) (entryRow, bool) {
	contentKind := pkgv1.PackageProblemKind_PACKAGE_PROBLEM_KIND_CONTENT
	name := te.GetNamePt()
	bad := func(reason pkgv1.PackageProblemReason) (entryRow, bool) {
		in.Problem(contentKind, name, reason, 0)
		return entryRow{}, false
	}
	body, kind := bodyOf(te)
	m := contentKeyShape.FindStringSubmatch(te.GetKey())
	if body == nil || kind != te.GetKind() || m == nil || m[1] != kindPrefix(kind) || len(m[2]) > rules.MaxSlugLength || body.GetNamePt() != te.GetNamePt() {
		return bad(pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_INVALID)
	}
	if seen[te.GetKey()] {
		return bad(pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_DUPLICATE)
	}
	seen[te.GetKey()] = true
	body = proto.Clone(body).(tableBody) //nolint:forcetypeassert // Clone returns the same type
	if v := firstViolation(checkShape(body), checkFeatureCount(kind, body)); v != nil {
		return bad(contentReason(v.GetReason()))
	}
	// Every feature key in the package was made by a server: they are the entry's own.
	if v := featureKeys(te.GetKey(), body, body); len(v) > 0 {
		return bad(contentReason(v[0].GetReason()))
	}
	data, err := storedData(body)
	if err != nil {
		return bad(pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_INVALID)
	}
	if len(data) > MaxTableEntryBytes {
		return bad(pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_LIMIT)
	}
	row := charactersdb.CampaignContent{
		CampaignID: in.CampaignID, ContentKey: te.GetKey(), Kind: kindPrefix(kind), NamePt: te.GetNamePt(), Data: data,
		Revision: 1, CreatedAt: at, UpdatedAt: at,
	}
	if te.GetArchived() {
		row.ArchivedAt = &at
	}
	return entryRow{row: row, kind: kind, body: body}, true
}

// firstViolation is the first violation of the lists, or nil.
func firstViolation(lists ...[]*rulesv1.TableContentViolation) *rulesv1.TableContentViolation {
	for _, l := range lists {
		if len(l) > 0 {
			return l[0]
		}
	}
	return nil
}

type stagedCharacter struct {
	id, name  string
	kind      string
	sheet     []byte
	story     []byte
	notes     string
	createdAt time.Time
}

type stagedCharacters struct{ npcs, players []*stagedCharacter }

// stageCharacters checks the NPCs and the players' characters against the
// table content of the package.
func (p *packagePart) stageCharacters(in *campaignpackage.Import, sc *stagedContent) error {
	s := p.s
	content := sc.content
	if content == nil {
		content = s.srd // a package without table content: the SRD alone
	}
	npcFiles := in.Pkg.Entries(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_NPC)
	charFiles := in.Pkg.Entries(pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CHARACTER)
	out := &stagedCharacters{}
	in.Set(stashCharacters, out)
	if len(npcFiles)+len(charFiles) > s.maxCharacters {
		in.Problem(pkgv1.PackageProblemKind_PACKAGE_PROBLEM_KIND_NPC, "", pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_LIMIT, int64(s.maxCharacters))
		return nil
	}
	master := authz.Membership{CampaignID: in.CampaignID, UserID: in.UserID, Role: authz.RoleMaster}
	base := s.now()
	for i, f := range npcFiles {
		var n pkgv1.PackageNpc
		if !in.Read(f.GetPath(), &n) {
			continue
		}
		c := p.stageOne(in, content, master, stageInput{
			kind: pkgv1.PackageProblemKind_PACKAGE_PROBLEM_KIND_NPC, id: n.GetId(), name: n.GetName(), sheet: n.GetSheet(), story: n.GetStory(), notes: n.GetMasterNotes(), player: false, npcKind: n.GetKind(),
		}, base.Add(time.Duration(i)*orderStep))
		if c != nil {
			out.npcs = append(out.npcs, c)
			in.Counts.Npcs++
		}
	}
	for i, f := range charFiles {
		var ch pkgv1.PackageCharacter
		if !in.Read(f.GetPath(), &ch) {
			continue
		}
		c := p.stageOne(in, content, master, stageInput{
			kind: pkgv1.PackageProblemKind_PACKAGE_PROBLEM_KIND_CHARACTER, id: ch.GetId(), name: ch.GetName(), sheet: ch.GetSheet(), story: ch.GetStory(), notes: ch.GetMasterNotes(), player: true,
		}, base.Add(time.Duration(len(npcFiles)+i)*orderStep))
		if c != nil {
			out.players = append(out.players, c)
			in.Facts.Players[ch.GetId()] = true
			in.Counts.Characters++
		}
	}
	return nil
}

type stageInput struct {
	kind            pkgv1.PackageProblemKind
	id, name, notes string
	sheet           *charactersv1.CharacterSheet
	story           *charactersv1.CharacterStory
	player          bool
	npcKind         charactersv1.CharacterKind
}

// stageOne checks one character or NPC and returns it ready to insert.
//
// The checks run in a fixed order and the first that fails is the problem
// recorded: the name, the id and the sheet's presence, the master's notes, the
// NPC's kind, the sheet as a hand-made one, the creature it was made from, the
// story.
func (p *packagePart) stageOne(in *campaignpackage.Import, content *rules.Content, master authz.Membership, c stageInput, at time.Time) *stagedCharacter {
	name, err := names.Clean(c.name, MaxNameLength)
	if err != nil {
		in.Problem(c.kind, "", packageInvalid, 0)
		return nil
	}
	out, reason := p.stageChecked(in, content, master, c, name)
	if reason != pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_UNSPECIFIED {
		in.Problem(c.kind, name, reason, 0)
		return nil
	}
	out.createdAt = at
	return out
}

// packageInvalid is the problem of a thing of the package that is not what the
// app would make.
const packageInvalid = pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_INVALID

// stageChecked is stageOne after the name: the character ready to insert, or the
// reason of the first check that failed.
func (p *packagePart) stageChecked(in *campaignpackage.Import, content *rules.Content, master authz.Membership, c stageInput, name string) (*stagedCharacter, pkgv1.PackageProblemReason) {
	id, ok := in.IDs.Define(nsCharacter, c.id)
	if !ok || c.sheet == nil {
		return nil, packageInvalid
	}
	notes, err := names.CleanText(c.notes, MaxMasterNotesLength)
	if err != nil {
		return nil, packageInvalid
	}
	sheet := proto.Clone(c.sheet).(*charactersv1.CharacterSheet) //nolint:forcetypeassert // Clone returns the same type
	kind, creature, ok := stageCharacterKind(in, c, sheet, name)
	if !ok {
		return nil, packageInvalid
	}
	checked, err := checkNewSheet(content, master, kind, sheet)
	if err != nil {
		return nil, sheetReason(err)
	}
	if !keepStagedCreatureLink(content, checked, creature) {
		return nil, pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_UNKNOWN_CONTENT
	}
	story, err := checkStory(c.story)
	if err != nil {
		return nil, packageInvalid
	}
	out := &stagedCharacter{id: id, name: name, kind: kind, notes: notes}
	if out.sheet, err = storeJSON.Marshal(checked); err != nil {
		return nil, packageInvalid
	}
	if out.story, err = storeJSON.Marshal(story); err != nil {
		return nil, packageInvalid
	}
	return out, pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_UNSPECIFIED
}

// stageCharacterKind is the stored kind of a character of the package and, for
// an NPC with a basic sheet, what the sheet says of the creature it was made
// from. It says false for an NPC of no NPC kind, or the app's own copy of a
// creature put in combat. An NPC's portrait is a gallery image of the package,
// so the sheet is given the image's new id.
func stageCharacterKind(in *campaignpackage.Import, c stageInput, sheet *charactersv1.CharacterSheet, name string) (string, *charactersv1.BasicSheet, bool) {
	if c.player {
		return kindPlayer, nil, true
	}
	kind, known := kindToDB[c.npcKind]
	if !known || kind == kindPlayer {
		return "", nil, false
	}
	var creature *charactersv1.BasicSheet
	if basic := sheet.GetBasic(); basic != nil {
		if basic.GetCombatOnly() {
			return "", nil, false
		}
		creature = &charactersv1.BasicSheet{MonsterKey: basic.GetMonsterKey(), AbilityScores: basic.GetAbilityScores()}
	}
	if old := portraitOf(sheet); old != "" {
		setPortrait(sheet, in.IDs.Ref("image", old, c.kind, name))
	}
	return kind, creature, true
}

// keepStagedCreatureLink puts back on the checked sheet the link to the creature and
// its scores, which are the server's: only the NPC made from a creature has
// them, and the package keeps them as they were. It says false for a creature
// the bestiary does not have or scores out of the rules' range.
func keepStagedCreatureLink(content *rules.Content, checked *charactersv1.CharacterSheet, creature *charactersv1.BasicSheet) bool {
	if creature.GetMonsterKey() == "" {
		return true
	}
	if _, known := content.CreatureByKey(creature.GetMonsterKey()); !known || !validAbilityScores(creature.GetAbilityScores()) {
		return false
	}
	checked.GetBasic().MonsterKey, checked.GetBasic().AbilityScores = creature.GetMonsterKey(), creature.GetAbilityScores()
	return true
}

// validAbilityScores says whether the six scores of a creature are in the
// rules' range.
func validAbilityScores(a *rulesv1.AbilityScores) bool {
	if a == nil {
		return true
	}
	for _, v := range []int32{a.GetStrength(), a.GetDexterity(), a.GetConstitution(), a.GetIntelligence(), a.GetWisdom(), a.GetCharisma()} {
		if v < 1 || v > rules.MaxScore {
			return false
		}
	}
	return true
}

// sheetReason is the problem a refused sheet becomes: a thing the rules do not
// have (a class of the table the package does not bring, an unknown key) is
// told from any other mistake.
func sheetReason(err error) pkgv1.PackageProblemReason {
	msg := err.Error()
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		msg = ce.Message()
	}
	for _, unknown := range []string{"unknown", "not a ", "is not an ", "does not exist", "not in the"} {
		if strings.Contains(msg, unknown) {
			return pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_NOT_IN_PACKAGE
		}
	}
	return pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_INVALID
}

// --- apply ------------------------------------------------------------------

// Apply implements campaignpackage.Part.
func (p *packagePart) Apply(ctx context.Context, tx pgx.Tx, in *campaignpackage.Import) error {
	q := p.s.queries.WithTx(tx)
	if sc, _ := in.Get(stashContent).(*stagedContent); sc != nil && (len(sc.entries) > 0 || len(sc.off) > 0) {
		if err := p.applyContent(ctx, q, in.CampaignID, sc); err != nil {
			return err
		}
	}
	chars, _ := in.Get(stashCharacters).(*stagedCharacters)
	if chars == nil {
		return nil
	}
	if err := applyNpcs(ctx, q, in, chars.npcs); err != nil {
		return err
	}
	return p.applyPlayers(ctx, tx, in, chars.players)
}

// applyContent writes the table content of the package under one new content
// revision: the entries, then the options switched off.
func (p *packagePart) applyContent(ctx context.Context, q *charactersdb.Queries, campaignID string, sc *stagedContent) error {
	now := p.s.now()
	if _, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: campaignID, Now: now}); err != nil {
		return fmt.Errorf("start the content revision: %w", err)
	}
	for _, e := range sc.entries {
		r := e.row
		if err := q.InsertImportedCampaignContent(ctx, charactersdb.InsertImportedCampaignContentParams{
			CampaignID: campaignID, ContentKey: r.ContentKey, Kind: r.Kind, NamePt: r.NamePt, Data: r.Data, Revision: r.Revision,
			ArchivedAt: r.ArchivedAt, Now: r.CreatedAt,
		}); err != nil {
			return fmt.Errorf("insert a table entry: %w", err)
		}
	}
	if len(sc.off) == 0 {
		return nil
	}
	if err := q.InsertContentOff(ctx, charactersdb.InsertContentOffParams{CampaignID: campaignID, ContentKeys: sc.off, Now: now}); err != nil {
		return fmt.Errorf("switch options off: %w", err)
	}
	return nil
}

// applyNpcs creates the NPCs of a package, owned by the importing master, each
// with the master's notes about it.
func applyNpcs(ctx context.Context, q *charactersdb.Queries, in *campaignpackage.Import, npcs []*stagedCharacter) error {
	for _, c := range npcs {
		if err := q.InsertImportedNpc(ctx, charactersdb.InsertImportedNpcParams{
			ID: c.id, CampaignID: in.CampaignID, Kind: c.kind, MasterUserID: &in.UserID, Name: c.name, Sheet: c.sheet, Story: c.story, Now: c.createdAt,
		}); err != nil {
			return fmt.Errorf("insert an imported NPC: %w", err)
		}
		if c.notes == "" {
			continue
		}
		if _, err := q.UpsertMasterNotes(ctx, charactersdb.UpsertMasterNotesParams{CampaignID: in.CampaignID, CharacterID: c.id, Notes: c.notes, UpdatedAt: c.createdAt}); err != nil {
			return fmt.Errorf("save the master's notes: %w", err)
		}
	}
	return nil
}
