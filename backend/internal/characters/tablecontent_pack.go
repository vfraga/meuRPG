package characters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The content pack (MR-025): the table's own entries as one file, ExportTableContent
// and ImportTableContent. A pack is {"format": "meurpg.table-content", "version": 1,
// "name": "...", "entries": [...]} in proto JSON; each entry is a TableEntry with its
// key, kind, name_pt and body. A pack is untrusted input. Nothing in it is run, and
// every entry goes through the rules of CreateTableEntry and UpdateTableEntry; an
// APPLY writes the whole pack in one transaction or nothing.

// Limits and names of the pack format.
const (
	// PackFormat is the "format" of a pack, and PackVersion the only "version" this server
	// reads.
	PackFormat  = "meurpg.table-content"
	PackVersion = 1
	// MaxPackBytes is the most proto JSON a pack takes: 1000 entries of the books' size come to about 1.4 MiB, and the RPC message limit is 4 MiB.
	MaxPackBytes = 3 << 20
	// maxPackNameRunes bounds the pack's label.
	maxPackNameRunes = 80
	// maxImportPasses bounds how many times the engine checks a pack that has refused
	// entries: each pass finds the entries it refuses, and leaves them out of the next.
	maxImportPasses = 25
)

// errNothingToImport rolls back the transaction of an APPLY that changes nothing, so the
// revision it bumped first is not kept.
var errNothingToImport = errors.New("the import changes nothing")

// errImportReplayed ends the transaction of an APPLY that found the answer of its key.
var errImportReplayed = errors.New("the answer of the idempotency key was already made")

// ExportTableContent implements rulesv1connect.TableContentServiceHandler.
func (s *Service) ExportTableContent(
	ctx context.Context,
	req *connect.Request[rulesv1.ExportTableContentRequest],
) (*connect.Response[rulesv1.ExportTableContentResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	pack := &rulesv1.TableContentPack{Format: PackFormat, Version: PackVersion}
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		pack.Entries, pack.Name = nil, ""
		rows, err := s.queries.WithTx(tx).ListCampaignContent(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the table's content", err)
		}
		entries, err := readEntries(rows)
		if err != nil {
			return err
		}
		for _, e := range entries {
			// Only what is content: the key, the kind, the name and the body. The revision,
			// the dates, the counts and the archive mark are this campaign's own.
			pe := &rulesv1.TableEntry{Key: e.row.ContentKey, Kind: e.kind, NamePt: e.row.NamePt}
			setBody(pe, e.body)
			pack.Entries = append(pack.Entries, pe)
		}
		if s.campaignNames != nil {
			if pack.Name, err = s.campaignNames.CampaignName(ctx, tx, m.CampaignID); err != nil {
				return wrap("read the campaign's name", err)
			}
			pack.Name = packLabel(pack.Name)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "export the table's content", err)
	}
	return connect.NewResponse(&rulesv1.ExportTableContentResponse{Pack: pack}), nil
}

// packLabel is a campaign's name as a pack's label: on one line, at most 80 characters.
func packLabel(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > maxPackNameRunes {
		name = string([]rune(name)[:maxPackNameRunes])
	}
	return strings.TrimSpace(name)
}

// packViolation is a violation about the pack itself.
func packViolation(field, reason, format string, args ...any) *rulesv1.TableContentViolation {
	return &rulesv1.TableContentViolation{Field: field, Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// packEntry is an entry of a pack that passed the checks of the pack itself: its key,
// kind and body, ready for the rules.
type packEntry struct {
	index int
	key   string
	kind  rulesv1.TableContentKind
	body  tableBody
	// unknown are the fields of the entry's JSON that the message does not have.
	unknown []*rulesv1.TableContentViolation
}

// checkPack checks what is about the pack itself, before the database is read: its size,
// its format and version, its label, the number of entries and, for each, the form of its
// key, its body and its name, and that no key repeats. It returns the entries that are
// well formed and every violation found.
func checkPack(pack *rulesv1.TableContentPack, unknown map[int][]*rulesv1.TableContentViolation) ([]packEntry, []*rulesv1.TableContentViolation) {
	if packTooBig(pack) {
		return nil, []*rulesv1.TableContentViolation{packViolation("pack", reasonSizeLimit, "the pack is over %d bytes", MaxPackBytes)}
	}
	out := checkPackHeader(pack)
	if n := len(pack.GetEntries()); n > rules.MaxOverlayEntries {
		out = append(out, packViolation("pack.entries", rules.ReasonLimit, "%d entries; the limit is %d per table", n, rules.MaxOverlayEntries))
		return nil, out
	}
	var entries []packEntry
	seen := map[string]bool{}
	for i, e := range pack.GetEntries() {
		if v := checkPackEntry(fmt.Sprintf("pack.entries[%d]", i), e, seen); v != nil {
			out = append(out, v)
			continue
		}
		body, kind := bodyOfEntry(e)
		entries = append(entries, packEntry{index: i, key: e.GetKey(), kind: kind, body: body, unknown: unknown[i]})
	}
	return entries, out
}

// packTooBig says whether the pack is over the limit, as proto or as proto JSON (a control
// character takes six bytes of JSON).
func packTooBig(pack *rulesv1.TableContentPack) bool {
	if proto.Size(pack) > MaxPackBytes {
		return true
	}
	data, err := storeJSON.Marshal(pack)
	return err != nil || len(data) > MaxPackBytes
}

// checkPackHeader checks the pack's format, version and label.
func checkPackHeader(pack *rulesv1.TableContentPack) []*rulesv1.TableContentViolation {
	var out []*rulesv1.TableContentViolation
	if pack.GetFormat() != PackFormat {
		out = append(out, packViolation("pack.format", rules.ReasonValue, "the format is %q", PackFormat))
	}
	if pack.GetVersion() != PackVersion {
		out = append(out, packViolation("pack.version", rules.ReasonValue, "the only version this server reads is %d", PackVersion))
	}
	if name := pack.GetName(); utf8.RuneCountInString(name) > maxPackNameRunes || strings.ContainsFunc(name, func(r rune) bool { return r == '\n' || r == '\r' || r < ' ' }) {
		out = append(out, packViolation("pack.name", rules.ReasonName, "the name is one line of at most %d characters", maxPackNameRunes))
	}
	return out
}

// checkPackEntry is the first thing wrong with the form of one entry of a pack (its kind,
// its key, a key seen before, its body, its name), or nil; it marks a good key as seen.
func checkPackEntry(field string, e *rulesv1.TableEntry, seen map[string]bool) *rulesv1.TableContentViolation {
	prefix := kindPrefix(e.GetKind())
	if prefix == "" {
		return packViolation(field+".kind", rules.ReasonValue, "the kind of an entry is one of the kinds of table content")
	}
	reason, msg := rules.CheckEntryKey(prefix, e.GetKey())
	if e.GetKind() == rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION {
		reason, msg = rules.CheckOptionKey(e.GetKey())
	}
	if reason != "" {
		return packViolation(field+".key", reason, "%s", msg)
	}
	if seen[e.GetKey()] {
		return packViolation(field+".key", rules.ReasonDuplicateKey, "the pack has this key twice")
	}
	seen[e.GetKey()] = true
	body, kind := bodyOfEntry(e)
	if body == nil || kind != e.GetKind() {
		return packViolation(field+".body", rules.ReasonValue, "an entry has exactly the body of its kind")
	}
	if e.GetNamePt() != "" && e.GetNamePt() != body.GetNamePt() {
		return packViolation(field+".name_pt", rules.ReasonValue, "the name of an entry is the name in its body")
	}
	return nil
}

// bodyOfEntry is the body of an entry and the kind it gives, or nil.
func bodyOfEntry(e *rulesv1.TableEntry) (tableBody, rulesv1.TableContentKind) {
	switch b := e.GetBody().(type) {
	case *rulesv1.TableEntry_TableClass:
		return b.TableClass, rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS
	case *rulesv1.TableEntry_TableSubclass:
		return b.TableSubclass, rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS
	case *rulesv1.TableEntry_TableRace:
		return b.TableRace, rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE
	case *rulesv1.TableEntry_TableSubrace:
		return b.TableSubrace, rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBRACE
	case *rulesv1.TableEntry_TableBackground:
		return b.TableBackground, rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND
	case *rulesv1.TableEntry_TableSpell:
		return b.TableSpell, rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL
	case *rulesv1.TableEntry_TableFeat:
		return b.TableFeat, rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT
	case *rulesv1.TableEntry_TableOption:
		return b.TableOption, rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION
	case *rulesv1.TableEntry_TableClassFeature:
		return b.TableClassFeature, rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS_FEATURE
	case *rulesv1.TableEntry_TableCreature:
		return b.TableCreature, rulesv1.TableContentKind_TABLE_CONTENT_KIND_CREATURE
	}
	return nil, rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED
}

// importPlan is what a pack would do to a campaign.
type importPlan struct {
	// results are the outcomes in the pack's order; refused ones carry violations.
	results []*rulesv1.TableImportEntry
	// content is the campaign's content with the pack in it; nil when something was refused.
	content *rules.Content
	// writes are the rows an APPLY writes: the new and the updated entries.
	writes []planWrite
	// global are the violations no entry of the pack owns (a limit of the whole table, an
	// existing entry the pack would break).
	global []*rulesv1.TableContentViolation
}

// planWrite is a row an APPLY writes.
type planWrite struct {
	key     string
	kind    rulesv1.TableContentKind
	data    []byte
	namePT  string
	created bool
}

// importCandidate is an entry of the campaign as it would be after the import: a stored
// one the pack leaves alone, or the pack's (write is set when it is new or updated).
type importCandidate struct {
	packIndex int // -1 for a stored entry the pack leaves alone
	row       entryRow
	write     *planWrite
}

// planner works out what a pack does to a campaign (planImport).
type planner struct {
	campaignID string
	revision   int32
	now        time.Time
	srd        *rules.Content
	entries    []packEntry
	stored     map[string]entryRow
	packIdx    map[string]int
	cands      []importCandidate
	// lead are the violations an entry has before the engine runs.
	lead map[int][]*rulesv1.TableContentViolation
	// excluded are the entries left out of the engine's check: the ones whose data cannot be
	// checked, and the ones it has already refused.
	excluded map[int]bool
	plan     *importPlan
}

func refused(r *rulesv1.TableImportEntry) bool {
	return r.GetStatus() == rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED
}

// planImport works out, in tx, what the pack does: for each entry, new, updated, unchanged
// or refused (with every violation), by running the whole campaign's overlay with the pack
// in it. Entries the rules refuse are left out and the rest is checked again, so one pass
// of the user shows everything wrong. It writes nothing.
func (s *Service) planImport(ctx context.Context, q *charactersdb.Queries, campaignID string, entries []packEntry, revision int32) (*importPlan, error) {
	rows, err := q.ListCampaignContent(ctx, campaignID)
	if err != nil {
		return nil, wrap("list the table's content", err)
	}
	storedRows, err := readEntries(rows)
	if err != nil {
		return nil, err
	}
	p := &planner{
		campaignID: campaignID, revision: revision, now: s.now(), srd: s.srd, entries: entries,
		stored: map[string]entryRow{}, packIdx: map[string]int{}, lead: map[int][]*rulesv1.TableContentViolation{}, excluded: map[int]bool{},
		plan: &importPlan{results: make([]*rulesv1.TableImportEntry, len(entries))},
	}
	for _, e := range storedRows {
		p.stored[e.row.ContentKey] = e
	}
	for i, pe := range entries {
		p.packIdx[pe.key] = i
	}
	for _, e := range storedRows {
		if _, replaced := p.packIdx[e.row.ContentKey]; !replaced {
			p.cands = append(p.cands, importCandidate{packIndex: -1, row: e})
		}
	}
	for i, pe := range entries {
		p.classify(i, pe)
	}
	p.checkNames()
	p.runEngine()
	for _, c := range p.cands {
		if c.write != nil && !refused(p.plan.results[c.packIndex]) {
			p.plan.writes = append(p.plan.writes, *c.write)
		}
	}
	return p.plan, nil
}

// classify prepares one entry of the pack (the keys of its features, its size) and says whether
// it is new, updated or unchanged, or already refused.
func (p *planner) classify(i int, pe packEntry) {
	res := &rulesv1.TableImportEntry{Key: pe.key, Kind: pe.kind, NamePt: pe.body.GetNamePt()}
	p.plan.results[i] = res
	p.lead[i] = append(p.lead[i], pe.unknown...)
	old, exists := p.stored[pe.key]
	var oldBody tableBody
	if exists {
		oldBody = old.body
	}
	st, err := prepareWith(importFeatureKeys, pe.key, pe.kind, pe.body, oldBody)
	if err != nil {
		p.lead[i] = append(p.lead[i], violationsOfRefusal(err, pe.key)...)
		p.excluded[i] = true
		res.Status = rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED
		if exists {
			p.cands = append(p.cands, importCandidate{packIndex: i, row: old}) // the stored one stays for the others to refer to
		}
		return
	}
	row := charactersdb.CampaignContent{
		CampaignID: p.campaignID, ContentKey: pe.key, Kind: kindPrefix(pe.kind), NamePt: pe.body.GetNamePt(), Data: st.data,
		Revision: p.revision, CreatedAt: p.now, UpdatedAt: p.now,
	}
	switch {
	case !exists:
		res.Status = rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_NEW
	case old.row.NamePt == row.NamePt && proto.Equal(st.body, old.body):
		res.Status = rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_UNCHANGED
		p.cands = append(p.cands, importCandidate{packIndex: i, row: old})
		return
	default:
		res.Status = rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_UPDATED
		res.ChangedFields = changedFields(old, row.NamePt, st.body)
		row = old.row
		row.NamePt, row.Data, row.Revision, row.UpdatedAt = pe.body.GetNamePt(), st.data, p.revision, p.now
	}
	w := &planWrite{key: pe.key, kind: pe.kind, data: st.data, namePT: row.NamePt, created: !exists}
	p.cands = append(p.cands, importCandidate{packIndex: i, row: entryRow{row: row, kind: pe.kind, body: st.body}, write: w})
}

// checkNames refuses a name another entry of the kind has, as the editors do.
func (p *planner) checkNames() {
	all := make([]entryRow, 0, len(p.cands))
	for _, c := range p.cands {
		all = append(all, c.row)
	}
	for i, pe := range p.entries {
		if p.plan.results[i].GetStatus() == rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_UNCHANGED {
			continue
		}
		if v := duplicateName(all, pe.kind, pe.key, pe.body.GetNamePt()); v != nil {
			p.lead[i] = append(p.lead[i], v)
		}
	}
	for i := range p.entries {
		if len(p.lead[i]) > 0 {
			p.plan.results[i].Status = rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED
			p.plan.results[i].Violations = p.lead[i]
		}
	}
}

// live is the campaign the engine checks: what is stored with the pack's entries put in, and
// without the ones refused so far (an updated one stays as it was stored).
func (p *planner) live() []entryRow {
	var out []entryRow
	for _, c := range p.cands {
		if c.packIndex < 0 || !p.excluded[c.packIndex] {
			out = append(out, c.row)
			continue
		}
		if old, wasStored := p.stored[c.row.row.ContentKey]; wasStored {
			out = append(out, old)
		}
	}
	return out
}

// runEngine runs the campaign's whole overlay with the pack in it. Each pass that fails says
// which entries of the pack the rules refuse; they are left out of the next pass.
func (p *planner) runEngine() {
	strict := make([]string, 0, len(p.entries))
	for _, pe := range p.entries {
		strict = append(strict, pe.key)
	}
	for range maxImportPasses {
		overlay, idx := overlayFromEntries(p.live(), int(p.revision))
		overlay.Strict = strict
		content, err := p.srd.With(overlay)
		if err == nil {
			if !anyRefused(p.plan.results) {
				p.plan.content = content
			}
			return
		}
		if !p.blame(violationsOf(err, idx, "")) || len(p.plan.global) > 0 {
			return // nothing of the pack to leave out: the rest is not the pack's to fix
		}
	}
}

// blame attributes the engine's violations to the entries of the pack that own them, and
// reports whether it found one it had not refused yet.
func (p *planner) blame(violations []*rulesv1.TableContentViolation) bool {
	progressed := false
	for _, v := range violations {
		i, ok := p.packIdx[v.GetKey()]
		if v.GetKey() == "" || !ok {
			p.plan.global = append(p.plan.global, v)
			continue
		}
		res := p.plan.results[i]
		if !p.excluded[i] {
			p.excluded[i], progressed = true, true
		}
		res.Status, res.ChangedFields = rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED, nil
		res.Violations = append(res.Violations, v)
	}
	return progressed
}

func anyRefused(results []*rulesv1.TableImportEntry) bool {
	return slices.ContainsFunc(results, refused)
}

// violationsOfRefusal is the violations of an errRefusedContent, each with the entry's key.
func violationsOfRefusal(err error, key string) []*rulesv1.TableContentViolation {
	var out []*rulesv1.TableContentViolation
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range ce.Details() {
			msg, derr := d.Value()
			if derr != nil {
				continue
			}
			if r, ok := msg.(*rulesv1.TableContentRefusal); ok {
				out = append(out, r.GetViolations()...)
			}
		}
	}
	if len(out) == 0 {
		out = []*rulesv1.TableContentViolation{{Reason: rules.ReasonValue, Message: "the entry was refused"}}
	}
	for _, v := range out {
		v.Key = key
	}
	return out
}

// changedFields are the paths of what differs between a stored entry and the pack's:
// "name_pt" and the body's top-level fields.
func changedFields(old entryRow, name string, next tableBody) []string {
	var out []string
	if old.row.NamePt != name {
		out = append(out, "name_pt")
	}
	prefix := bodyField(old.kind)
	oldMsg, newMsg := old.body.ProtoReflect(), next.ProtoReflect()
	fields := oldMsg.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.Name() == "name_pt" {
			continue
		}
		if !sameField(oldMsg, newMsg, fd) {
			out = append(out, prefix+"."+string(fd.Name()))
		}
	}
	return out
}

// sameField compares one field of two messages of a kind.
func sameField(a, b protoreflect.Message, fd protoreflect.FieldDescriptor) bool {
	if a.Has(fd) != b.Has(fd) {
		return false
	}
	if !a.Has(fd) {
		return true
	}
	switch {
	case fd.IsList():
		x, y := a.Get(fd).List(), b.Get(fd).List()
		if x.Len() != y.Len() {
			return false
		}
		for i := range x.Len() {
			if !sameValue(fd, x.Get(i), y.Get(i)) {
				return false
			}
		}
		return true
	case fd.IsMap():
		return false // the bodies have no maps
	}
	return sameValue(fd, a.Get(fd), b.Get(fd))
}

func sameValue(fd protoreflect.FieldDescriptor, x, y protoreflect.Value) bool {
	if fd.Message() != nil {
		return proto.Equal(x.Message().Interface(), y.Message().Interface())
	}
	return x.Equal(y)
}

// importCall is one ImportTableContent: the request and what the transaction found. The
// transaction's closure runs again when CockroachDB retries it, so everything it sets starts
// again from run.
type importCall struct {
	s       *Service
	m       authz.Membership
	msg     *rulesv1.ImportTableContentRequest
	pack    *rulesv1.TableContentPack
	entries []packEntry
	apply   bool
	// scopedKey and requestHash are the idempotency key of an APPLY and the hash of its request.
	scopedKey, requestHash *string

	res      *rulesv1.ImportTableContentResponse
	affected []affectedSheet
	changed  bool
	replayed bool
}

// ImportTableContent implements rulesv1connect.TableContentServiceHandler.
func (s *Service) ImportTableContent(
	ctx context.Context,
	req *connect.Request[rulesv1.ImportTableContentRequest],
) (*connect.Response[rulesv1.ImportTableContentResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	pack, unknown, err := readImportRequest(req.Msg)
	if err != nil {
		return nil, err
	}
	idemKey, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	// What is about the pack itself is answered before the database is read, in both modes.
	entries, violations := checkPack(pack, unknown)
	if len(violations) > 0 {
		return nil, errRefusedContent(violations)
	}
	call := &importCall{s: s, m: m, msg: req.Msg, pack: pack, entries: entries, apply: req.Msg.GetMode() == rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY}
	if call.apply {
		call.scopedKey, call.requestHash = idem.Scope(m.CampaignID, idemKey), idem.Hash(req.Msg)
	}
	if err := call.transact(ctx); err != nil {
		return nil, s.dbError(ctx, "import the table's content", err)
	}
	if !call.replayed {
		// The players' display names come after the transaction (the identity module reads
		// through its own pool). A replay answers what the first call kept, without them.
		if call.res.AffectedCharacters, err = s.affectedToProto(ctx, call.affected); err != nil {
			return nil, s.dbError(ctx, "read display names", err)
		}
	}
	if call.changed {
		res := call.res
		logging.Event(ctx, s.logger, "content.imported", slog.Int("table_revision", int(res.GetTableRevision())),
			slog.Int("new", int(res.GetTotals().GetNew())), slog.Int("updated", int(res.GetTotals().GetUpdated())), slog.Int("unchanged", int(res.GetTotals().GetUnchanged())))
		s.publishContentChanged(m.CampaignID)
	}
	return connect.NewResponse(call.res), nil
}

// readImportRequest checks the mode and takes the pack, from the message or from its file
// (read strictly, so an unknown field is a violation).
func readImportRequest(msg *rulesv1.ImportTableContentRequest) (*rulesv1.TableContentPack, map[int][]*rulesv1.TableContentViolation, error) {
	mode := msg.GetMode()
	if mode != rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW && mode != rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY {
		return nil, nil, invalidArgument(fieldErr("mode", "must be preview or apply"))
	}
	pack, unknown := msg.GetPack(), map[int][]*rulesv1.TableContentViolation(nil)
	if raw := msg.GetPackJson(); len(raw) > 0 {
		if pack != nil {
			return nil, nil, invalidArgument(fieldErr("pack_json", "and pack are alternatives: send one"))
		}
		var top []*rulesv1.TableContentViolation
		if pack, unknown, top = readPackJSON(raw); len(top) > 0 {
			return nil, nil, errRefusedContent(top)
		}
	}
	if pack == nil {
		return nil, nil, invalidArgument(fieldErr("pack", "is required"))
	}
	return pack, unknown, nil
}

// transact runs the call's transaction: an APPLY's writes, a PREVIEW's read. An APPLY that
// found the answer of its key, or that changes nothing, ends its transaction with an error
// that rolls the bump back; neither is an error of the call.
func (c *importCall) transact(ctx context.Context) error {
	run := func(tx pgx.Tx) error { return c.run(ctx, tx) }
	var err error
	if c.apply {
		err = db.InTx(ctx, c.s.pool, run)
	} else {
		err = db.ReadTx(ctx, c.s.pool, run)
	}
	switch {
	case c.replayed && errors.Is(err, errImportReplayed):
		return nil // the first call wrote and announced it
	case errors.Is(err, errNothingToImport):
		return nil
	}
	return err
}

// run is the transaction of the call: a PREVIEW reads, an APPLY bumps the content revision
// first (so it takes its turn among the campaign's writes), plans, and writes.
func (c *importCall) run(ctx context.Context, tx pgx.Tx) error {
	q := c.s.queries.WithTx(tx)
	c.res, c.affected, c.changed, c.replayed = nil, nil, false, false
	var rev int32
	var err error
	if c.apply {
		if rev, err = q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: c.m.CampaignID, Now: c.s.now()}); err != nil {
			return wrap("bump the content revision", err)
		}
		if replayed, err := c.replay(ctx, q); replayed || err != nil {
			return err
		}
	} else {
		current, err := contentRevision(ctx, q, c.m.CampaignID)
		if err != nil {
			return err
		}
		rev = current + 1 // the revision a write would make
	}
	plan, err := c.s.planImport(ctx, q, c.m.CampaignID, c.entries, rev)
	if err != nil {
		return err
	}
	if len(plan.global) > 0 {
		return errRefusedContent(plan.global)
	}
	c.res = &rulesv1.ImportTableContentResponse{
		Mode: c.msg.GetMode(), PackName: c.pack.GetName(), Entries: plan.results, Totals: importTotals(plan.results),
	}
	if !c.apply {
		c.res.TableRevision, err = contentRevision(ctx, q, c.m.CampaignID)
		return err
	}
	if c.res.GetTotals().GetRefused() > 0 {
		var all []*rulesv1.TableContentViolation
		for _, r := range plan.results {
			all = append(all, r.GetViolations()...)
		}
		return errRefusedContent(all)
	}
	if len(plan.writes) == 0 {
		c.res.TableRevision = rev - 1
		return errNothingToImport
	}
	return c.write(ctx, q, plan, rev)
}

// replay finds the answer of an earlier APPLY with the same idempotency key. It ends the
// transaction with errImportReplayed (the bump is rolled back) when it has one.
func (c *importCall) replay(ctx context.Context, q *charactersdb.Queries) (bool, error) {
	if c.scopedKey == nil {
		return false, nil
	}
	prior, err := q.GetCampaignContentImport(ctx, charactersdb.GetCampaignContentImportParams{CampaignID: c.m.CampaignID, CreateKey: *c.scopedKey})
	switch {
	case err == nil:
		if err := idem.SameRequest(&prior.CreateHash, c.requestHash); err != nil {
			return true, err
		}
		first := &rulesv1.ImportTableContentResponse{}
		if err := proto.Unmarshal(prior.Response, first); err != nil {
			return true, wrap("read the answer of the key", err)
		}
		c.res, c.replayed = first, true
		return true, errImportReplayed
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	}
	return true, wrap("find the import of the key", err)
}

// write stores the new and the updated entries, finds the sheets the update leaves with
// issues, and keeps the answer under the idempotency key.
func (c *importCall) write(ctx context.Context, q *charactersdb.Queries, plan *importPlan, rev int32) error {
	now := c.s.now()
	for _, w := range plan.writes {
		var err error
		if w.created {
			_, err = q.InsertCampaignContent(ctx, charactersdb.InsertCampaignContentParams{
				CampaignID: c.m.CampaignID, ContentKey: w.key, Kind: kindPrefix(w.kind), NamePt: w.namePT, Data: w.data, Revision: rev, Now: now,
			})
		} else {
			_, err = q.UpdateCampaignContent(ctx, charactersdb.UpdateCampaignContentParams{
				CampaignID: c.m.CampaignID, ContentKey: w.key, NamePt: w.namePT, Data: w.data, Revision: rev, Now: now,
			})
		}
		if err != nil {
			return wrap("write a table entry", err)
		}
	}
	var err error
	if c.affected, err = c.s.affectedByImport(ctx, q, c.m.CampaignID, plan); err != nil {
		return err
	}
	c.res.TableRevision, c.changed = rev, true
	for _, a := range c.affected {
		c.res.AffectedCharacters = append(c.res.AffectedCharacters, &rulesv1.AffectedCharacter{CharacterId: a.id, Name: a.name, Issues: i32(a.issues)})
	}
	if c.scopedKey == nil {
		return nil
	}
	// The kept answer has no player names: they are read after the transaction.
	data, err := proto.Marshal(c.res)
	if err != nil {
		return wrap("encode the answer of the import", err)
	}
	if err := q.InsertCampaignContentImport(ctx, charactersdb.InsertCampaignContentImportParams{
		CampaignID: c.m.CampaignID, CreateKey: *c.scopedKey, CreateHash: *c.requestHash, Response: data, Now: now,
	}); err != nil {
		return wrap("keep the answer of the import", err)
	}
	return nil
}

// totalsOf counts the outcomes of a plan.
func importTotals(results []*rulesv1.TableImportEntry) *rulesv1.TableImportTotals {
	t := &rulesv1.TableImportTotals{}
	for _, r := range results {
		switch r.GetStatus() {
		case rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_NEW:
			t.New++
		case rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_UPDATED:
			t.Updated++
		case rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_UNCHANGED:
			t.Unchanged++
		case rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED:
			t.Refused++
		}
	}
	return t
}

// affectedByImport finds, among the campaign's sheets, the ones that use an entry the import
// updated and have issues with the new rules; a character counts once, with all its issues.
func (s *Service) affectedByImport(ctx context.Context, q *charactersdb.Queries, campaignID string, plan *importPlan) ([]affectedSheet, error) {
	var updated []string
	for _, w := range plan.writes {
		if !w.created {
			updated = append(updated, w.key)
		}
	}
	if len(updated) == 0 || plan.content == nil {
		return nil, nil
	}
	sheets, err := q.ListCampaignSheets(ctx, campaignID)
	if err != nil {
		return nil, wrap("list the campaign's sheets", err)
	}
	byID := map[string]*affectedSheet{}
	var order []string
	for _, key := range updated {
		found, err := affectedSheets(sheets, plan.content, key)
		if err != nil {
			return nil, err
		}
		for _, a := range found {
			if cur, ok := byID[a.id]; ok {
				cur.issues += a.issues
				continue
			}
			byID[a.id] = &a
			order = append(order, a.id)
		}
	}
	out := make([]affectedSheet, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}
