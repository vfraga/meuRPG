package characters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// TableContentService (MR-025, RN-23, ADR-0018): the table's own content, read in
// full and written by the master. Every write runs in one transaction that first
// bumps the campaign's content revision (so two writes of a campaign run one
// after the other, and a sheet written next to one is ordered against it), then
// checks the WHOLE campaign's overlay with rules.Content.With, and only then
// writes: a refused overlay changes nothing. Edits are live (question 80): the
// next read of any request sees the new revision.

var _ rulesv1connect.TableContentServiceHandler = (*Service)(nil)

// reasonDuplicateName is a violation the server raises: a name another entry of
// the kind already has.
const reasonDuplicateName = "duplicate_name"

// errNoTableEntry is the answer for a key the campaign has no entry for.
func errNoTableEntry() error {
	return connect.NewError(connect.CodeNotFound, errors.New("table entry not found"))
}

// errRefusedContent is `invalid_argument` with the TableContentRefusal detail.
func errRefusedContent(violations []*rulesv1.TableContentViolation) error {
	err := connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the table's content was refused: %d violation(s), see the details", len(violations)))
	if detail, derr := connect.NewErrorDetail(&rulesv1.TableContentRefusal{Violations: violations}); derr == nil {
		err.AddDetail(detail)
	}
	return err
}

// errBlockedContent is `failed_precondition` (or `aborted` for a stale entry) with
// the TableContentBlocked detail.
func errBlockedContent(code connect.Code, reason rulesv1.TableContentBlockedReason, key, msg string) error {
	err := connect.NewError(code, errors.New(msg))
	if detail, derr := connect.NewErrorDetail(&rulesv1.TableContentBlocked{Reason: reason, Key: key}); derr == nil {
		err.AddDetail(detail)
	}
	return err
}

// tableKindOrder sorts the entries of ListTableEntries: by kind, then name.
func tableKindOrder(k rulesv1.TableContentKind) int { return int(k) }

// readEntries decodes the stored entries.
func readEntries(rows []charactersdb.CampaignContent) ([]entryRow, error) {
	out := make([]entryRow, 0, len(rows))
	for _, r := range rows {
		e, err := decodeRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// contentRevision is the campaign's content revision in tx, 0 when it has none.
func contentRevision(ctx context.Context, q *charactersdb.Queries, campaignID string) (int32, error) {
	rev, err := q.GetContentRevision(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, wrap("read the content revision", err)
	}
	return rev, nil
}

// characterUses counts, per table key, the characters of the campaign that use
// it (a key counts once per character), from the campaign's sheets.
func characterUses(sheets []charactersdb.ListCampaignSheetsRow) (map[string]int, error) {
	uses := map[string]int{}
	for _, row := range sheets {
		sheet, err := loadSheet(row.ID, row.Sheet)
		if err != nil {
			return nil, err
		}
		if full := sheet.GetFull(); full != nil {
			for _, k := range rules.TableKeys(buildOf(full)) {
				uses[k]++
			}
		}
	}
	return uses, nil
}

// ListTableEntries implements rulesv1connect.TableContentServiceHandler.
func (s *Service) ListTableEntries(
	ctx context.Context,
	req *connect.Request[rulesv1.ListTableEntriesRequest],
) (*connect.Response[rulesv1.ListTableEntriesResponse], error) {
	// A pending member only needs the catalog (ListContent): not_found here.
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	master := isMaster(m)
	res := &rulesv1.ListTableEntriesResponse{}
	// One transaction: the revision, the rows and the counts are one snapshot.
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		rev, err := contentRevision(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		rows, err := q.ListCampaignContent(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the table's content", err)
		}
		entries, err := readEntries(rows)
		if err != nil {
			return err
		}
		var uses map[string]int
		if master {
			sheets, err := q.ListCampaignSheets(ctx, m.CampaignID)
			if err != nil {
				return wrap("list the campaign's sheets", err)
			}
			if uses, err = characterUses(sheets); err != nil {
				return err
			}
		}
		content, err := s.contentFor(ctx, tx, m.CampaignID)
		if err != nil {
			return err
		}
		res = &rulesv1.ListTableEntriesResponse{TableRevision: rev, ContentVersion: content.Version()}
		// What a player never receives (RN-23): retired or switched off, SRD or table.
		var hidden func(string) bool
		if content.AnyHidden() {
			hidden = content.Hidden
		}
		var mine map[string]bool // the keys the player's own sheets use (read in this transaction)
		if !master {
			if mine, err = callerKeys(ctx, q, m); err != nil {
				return wrap("read the caller's sheets", err)
			}
		}
		for _, e := range entries {
			if !master {
				switch {
				case e.row.ArchivedAt != nil || content.Off(e.row.ContentKey):
					continue // a player never receives a retired or switched off entry, or a draft
				case hidden != nil && (parentHidden(e, hidden) || hidden(e.row.ContentKey)) && !mine[e.row.ContentKey]:
					continue // nor an entry whose class or race is retired or off, unless their sheet uses it
				}
				e = forPlayer(e, hidden)
			}
			pe := entryToProto(e, uses[e.row.ContentKey])
			pe.Off = content.Off(e.row.ContentKey)
			res.Entries = append(res.Entries, pe)
		}
		slices.SortStableFunc(res.Entries, func(a, b *rulesv1.TableEntry) int {
			if d := tableKindOrder(a.GetKind()) - tableKindOrder(b.GetKind()); d != 0 {
				return d
			}
			return strings.Compare(a.GetNamePt(), b.GetNamePt())
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "list the table's content", err)
	}
	return connect.NewResponse(res), nil
}

// duplicateName is a violation when another entry of the kind has the name, in
// any case.
func duplicateName(entries []entryRow, kind rulesv1.TableContentKind, key, name string) *rulesv1.TableContentViolation {
	for _, e := range entries {
		if e.kind == kind && e.row.ContentKey != key && strings.EqualFold(e.row.NamePt, name) {
			return &rulesv1.TableContentViolation{
				Field: bodyField(kind) + ".name_pt", Reason: reasonDuplicateName, Message: "another entry of this kind has this name",
			}
		}
	}
	return nil
}

// stored is what a write ends with: the new rows of the campaign's content.
type stored struct {
	key  string
	body tableBody
	kind rulesv1.TableContentKind
	data []byte
}

// checkOverlay builds the campaign's whole overlay with the entries (the write's
// already put in) and runs it through the engine at revision. It returns the
// content, or the refusal.
func (s *Service) checkOverlay(entries []entryRow, revision int, key string, lead ...*rulesv1.TableContentViolation) (*rules.Content, error) {
	overlay, idx := overlayFromEntries(entries, revision)
	overlay.Strict = []string{key} // a write: a field the effect's type does not read is refused
	content, err := s.srd.With(overlay)
	if err != nil {
		return nil, errRefusedContent(append(slices.Clone(lead), violationsOf(err, idx, key)...))
	}
	if len(lead) > 0 {
		return nil, errRefusedContent(lead)
	}
	return content, nil
}

// prepare finishes a body for a write: the keys of its features, the size of its
// data. It works on a clone of body, so a retried transaction starts from what the
// request had.
func prepare(key string, kind rulesv1.TableContentKind, body, old tableBody, lead ...*rulesv1.TableContentViolation) (stored, error) {
	b := proto.Clone(body).(tableBody)
	if v := checkShape(b); len(v) > 0 {
		return stored{}, errRefusedContent(append(slices.Clone(lead), v...))
	}
	if v := checkFeatureCount(kind, b); len(v) > 0 {
		return stored{}, errRefusedContent(append(slices.Clone(lead), v...))
	}
	if v := featureKeys(key, b, old); len(v) > 0 {
		return stored{}, errRefusedContent(append(slices.Clone(lead), v...))
	}
	data, err := storedData(b)
	if err != nil {
		return stored{}, wrap("encode a table entry", err)
	}
	if len(data) > MaxTableEntryBytes {
		return stored{}, errRefusedContent([]*rulesv1.TableContentViolation{{
			Field: bodyField(kind), Reason: reasonSizeLimit, Message: fmt.Sprintf("the entry is over %d bytes", MaxTableEntryBytes),
		}})
	}
	return stored{key: key, body: b, kind: kind, data: data}, nil
}

// withRow replaces or adds the entry of key in entries, as it will be stored.
func withRow(entries []entryRow, st stored, row charactersdb.CampaignContent) []entryRow {
	out := slices.Clone(entries)
	e := entryRow{row: row, kind: st.kind, body: st.body}
	for i := range out {
		if out[i].row.ContentKey == st.key {
			out[i] = e
			return out
		}
	}
	return append(out, e)
}

// errReplayed ends the transaction of a CreateTableEntry that found the entry of its key: the
// transaction is rolled back, so the revision it bumped first is not kept, and the handler
// answers with the entry the first call made.
var errReplayed = errors.New("the entry of the idempotency key was already made")

// CreateTableEntry implements rulesv1connect.TableContentServiceHandler.
func (s *Service) CreateTableEntry(
	ctx context.Context,
	req *connect.Request[rulesv1.CreateTableEntryRequest],
) (*connect.Response[rulesv1.CreateTableEntryResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	body, kind := bodyOf(req.Msg)
	if body == nil {
		return nil, invalidArgument(fieldErr("body", "is required: exactly one of the table_* messages"))
	}
	// The cheap size refusal comes before the transaction, so a body over the
	// limits never holds the campaign's content revision.
	if v := checkFeatureCount(kind, body); len(v) > 0 {
		return nil, errRefusedContent(v)
	}
	idemKey, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first entry only when it is the same request.
	scopedKey, requestHash := idem.Scope(m.CampaignID, idemKey), idem.Hash(req.Msg)
	var res *rulesv1.CreateTableEntryResponse
	replayed := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		replayed = false
		rev, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: m.CampaignID, Now: s.now()})
		if err != nil {
			return wrap("bump the content revision", err)
		}
		// The bump holds the campaign's content until this transaction ends, so a call with the
		// same key waits here for the first one, and then finds its entry.
		if scopedKey != nil {
			prior, err := q.GetCampaignContentByCreateKey(ctx, scopedKey)
			switch {
			case err == nil:
				if err := idem.SameRequest(prior.CreateHash, requestHash); err != nil {
					return err
				}
				entry, err := decodeRow(prior)
				if err != nil {
					return err
				}
				// The table's revision as it is now. The read sees this transaction's own bump, which
				// the rollback below undoes, so the revision is one less: a retry changes nothing.
				current, err := q.GetContentRevision(ctx, m.CampaignID)
				if err != nil {
					return wrap("read the content revision", err)
				}
				res = &rulesv1.CreateTableEntryResponse{Entry: entryToProto(entry, 0), TableRevision: current - 1}
				replayed = true
				return errReplayed
			case !errors.Is(err, pgx.ErrNoRows):
				return wrap("find the entry of the key", err)
			}
		}
		rows, err := q.ListCampaignContent(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the table's content", err)
		}
		entries, err := readEntries(rows)
		if err != nil {
			return err
		}
		key := entryKey(kind, body.GetNamePt(), rows)
		// A name another entry has is one more violation of this entry, listed with the ones the rules find (not instead of them).
		var lead []*rulesv1.TableContentViolation
		if v := duplicateName(entries, kind, key, body.GetNamePt()); v != nil {
			lead = append(lead, v)
		}
		st, err := prepare(key, kind, body, nil, lead...)
		if err != nil {
			return err
		}
		now := s.now()
		row := charactersdb.CampaignContent{
			CampaignID: m.CampaignID, ContentKey: key, Kind: kindPrefix(kind), NamePt: body.GetNamePt(), Data: st.data,
			Revision: rev, CreatedAt: now, UpdatedAt: now,
		}
		if _, err := s.checkOverlay(withRow(entries, st, row), int(rev), key, lead...); err != nil {
			return err
		}
		if scopedKey != nil {
			row, err = q.InsertCampaignContentWithKey(ctx, charactersdb.InsertCampaignContentWithKeyParams{
				CampaignID: m.CampaignID, ContentKey: key, Kind: kindPrefix(kind), NamePt: body.GetNamePt(), Data: st.data,
				Revision: rev, CreateKey: scopedKey, CreateHash: requestHash, Now: now,
			})
		} else {
			row, err = q.InsertCampaignContent(ctx, charactersdb.InsertCampaignContentParams{
				CampaignID: m.CampaignID, ContentKey: key, Kind: kindPrefix(kind), NamePt: body.GetNamePt(), Data: st.data,
				Revision: rev, Now: now,
			})
		}
		if err != nil {
			return wrap("insert a table entry", err)
		}
		res = &rulesv1.CreateTableEntryResponse{
			// No sheet can use an entry that did not exist a moment ago: the count is 0, set
			// for the master as every write response does.
			Entry: entryToProto(entryRow{row: row, kind: kind, body: st.body}, 0), TableRevision: rev,
		}
		return nil
	})
	if replayed && errors.Is(err, errReplayed) {
		return connect.NewResponse(res), nil // the first call wrote and announced the entry
	}
	if err != nil {
		return nil, s.dbError(ctx, "create a table entry", err)
	}
	logging.Event(ctx, s.logger, "content.entry_written", slog.String("entry_key", res.GetEntry().GetKey()), slog.String("kind", kindPrefix(kind)), slog.Int("table_revision", int(res.GetTableRevision())), slog.Bool("created", true))
	s.publishContentChanged(m.CampaignID)
	return connect.NewResponse(res), nil
}

// UpdateTableEntry implements rulesv1connect.TableContentServiceHandler.
func (s *Service) UpdateTableEntry(
	ctx context.Context,
	req *connect.Request[rulesv1.UpdateTableEntryRequest],
) (*connect.Response[rulesv1.UpdateTableEntryResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key := req.Msg.GetKey()
	if !rules.IsTableKey(key) {
		return nil, errNoTableEntry()
	}
	body, kind := bodyOf(req.Msg)
	if body == nil {
		return nil, invalidArgument(fieldErr("body", "is required: exactly one of the table_* messages"))
	}
	if kindPrefix(kind) != keyKind(key) {
		return nil, errRefusedContent([]*rulesv1.TableContentViolation{{
			Field: "body", Reason: reasonImmutable, Message: "the kind of an entry never changes",
		}})
	}
	// The cheap size refusal comes before the transaction, so a body over the
	// limits never holds the campaign's content revision.
	if v := checkFeatureCount(kind, body); len(v) > 0 {
		return nil, errRefusedContent(v)
	}
	var res *rulesv1.UpdateTableEntryResponse
	var affected []affectedSheet
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		rev, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: m.CampaignID, Now: s.now()})
		if err != nil {
			return wrap("bump the content revision", err)
		}
		cur, err := q.GetCampaignContent(ctx, charactersdb.GetCampaignContentParams{CampaignID: m.CampaignID, ContentKey: key})
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoTableEntry()
		}
		if err != nil {
			return wrap("read a table entry", err)
		}
		// An archived entry can still be edited (it stays archived).
		if cur.Revision != req.Msg.GetExpectedRevision() {
			return errBlockedContent(connect.CodeAborted, rulesv1.TableContentBlockedReason_TABLE_CONTENT_BLOCKED_REASON_STALE, key, "the entry changed since you opened it; reload it and try again")
		}
		old, err := decodeRow(cur)
		if err != nil {
			return err
		}
		if v := immutableChange(kind, old.body, body); v != nil {
			return errRefusedContent([]*rulesv1.TableContentViolation{v})
		}
		rows, err := q.ListCampaignContent(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the table's content", err)
		}
		entries, err := readEntries(rows)
		if err != nil {
			return err
		}
		var lead []*rulesv1.TableContentViolation
		if v := duplicateName(entries, kind, key, body.GetNamePt()); v != nil {
			lead = append(lead, v)
		}
		st, err := prepare(key, kind, body, old.body, lead...)
		if err != nil {
			return err
		}
		now := s.now()
		next := cur
		next.NamePt, next.Data, next.Revision, next.UpdatedAt = body.GetNamePt(), st.data, rev, now
		content, err := s.checkOverlay(withRow(entries, st, next), int(rev), key, lead...)
		if err != nil {
			return err
		}
		row, err := q.UpdateCampaignContent(ctx, charactersdb.UpdateCampaignContentParams{
			CampaignID: m.CampaignID, ContentKey: key, NamePt: body.GetNamePt(), Data: st.data, Revision: rev, Now: now,
		})
		if err != nil {
			return wrap("update a table entry", err)
		}
		sheets, err := q.ListCampaignSheets(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the campaign's sheets", err)
		}
		if affected, err = affectedSheets(sheets, content, key); err != nil {
			return err
		}
		uses, err := characterUses(sheets)
		if err != nil {
			return err
		}
		res = &rulesv1.UpdateTableEntryResponse{
			Entry: entryToProto(entryRow{row: row, kind: kind, body: st.body}, uses[key]), TableRevision: rev,
		}
		offKeys, err := q.ListContentOff(ctx, m.CampaignID)
		if err != nil {
			return wrap("read the options switched off", err)
		}
		res.Entry.Off = slices.Contains(offKeys, key)
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a table entry", err)
	}
	logging.Event(ctx, s.logger, "content.entry_written", slog.String("entry_key", key), slog.Int("table_revision", int(res.GetTableRevision())), slog.Bool("created", false))
	s.publishContentChanged(m.CampaignID)
	if res.AffectedCharacters, err = s.affectedToProto(ctx, affected); err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}
	return connect.NewResponse(res), nil
}

// immutableChange is a violation when an edit changes what never changes: the
// parent of a subclass (its class) or of a subrace (its race), or a spell going
// between cantrip and leveled. The last one is why: a sheet keeps its cantrips and
// its leveled spells in different lists, and Validate refuses a spell in the wrong
// one, so such an edit would make an unrelated save of a sheet that uses the spell
// fail (question 80: a change of an entry never stops another edit).
func immutableChange(kind rulesv1.TableContentKind, old, next tableBody) *rulesv1.TableContentViolation {
	var from, to, field string
	switch kind {
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL:
		if (old.(*rulesv1.TableSpell).GetLevel() == 0) != (next.(*rulesv1.TableSpell).GetLevel() == 0) {
			return &rulesv1.TableContentViolation{
				Field: "table_spell.level", Reason: reasonImmutable, Message: "a spell never goes between cantrip and leveled: make a new one",
			}
		}
		return nil
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS:
		from, to, field = old.(*rulesv1.TableSubclass).GetClassKey(), next.(*rulesv1.TableSubclass).GetClassKey(), "table_subclass.class_key"
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBRACE:
		from, to, field = old.(*rulesv1.TableSubrace).GetRaceKey(), next.(*rulesv1.TableSubrace).GetRaceKey(), "table_subrace.race_key"
	default:
		return nil
	}
	if from == to {
		return nil
	}
	return &rulesv1.TableContentViolation{Field: field, Reason: reasonImmutable, Message: "the parent of an entry never changes"}
}

// affectedSheet is a character that uses a changed entry and has issues now.
type affectedSheet struct {
	id, name, playerID string
	issues             int
}

// affectedSheets finds, among the campaign's sheets, the ones that use key and
// have issues with the new content (ones without are not told: nothing to fix).
func affectedSheets(sheets []charactersdb.ListCampaignSheetsRow, content *rules.Content, key string) ([]affectedSheet, error) {
	var out []affectedSheet
	for _, row := range sheets {
		sheet, err := loadSheet(row.ID, row.Sheet)
		if err != nil {
			return nil, err
		}
		full := sheet.GetFull()
		if full == nil {
			continue
		}
		if !slices.Contains(rules.TableKeys(buildOf(full)), key) {
			continue
		}
		// Only the new issues that depend on this entry: the ones this change can
		// be blamed for.
		if n := len(issuesTiedTo(content, full, key)); n > 0 {
			out = append(out, affectedSheet{id: row.ID, name: row.Name, playerID: deref(row.PlayerUserID), issues: n})
		}
	}
	return out, nil
}

// affectedToProto adds the players' display names, which the identity module
// reads (never inside the transaction: it reads through its own pool).
func (s *Service) affectedToProto(ctx context.Context, in []affectedSheet) ([]*rulesv1.AffectedCharacter, error) {
	var ids []string
	for _, a := range in {
		if a.playerID != "" {
			ids = append(ids, a.playerID)
		}
	}
	names, err := s.displayNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	var out []*rulesv1.AffectedCharacter
	for _, a := range in {
		out = append(out, &rulesv1.AffectedCharacter{
			CharacterId: a.id, Name: a.name, PlayerDisplayName: names[a.playerID], Issues: i32(a.issues),
		})
	}
	return out, nil
}

// ArchiveTableEntry implements rulesv1connect.TableContentServiceHandler.
func (s *Service) ArchiveTableEntry(
	ctx context.Context,
	req *connect.Request[rulesv1.ArchiveTableEntryRequest],
) (*connect.Response[rulesv1.ArchiveTableEntryResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	entry, rev, err := s.setArchived(ctx, m, req.Msg.GetKey(), true)
	if err != nil {
		return nil, s.dbError(ctx, "archive a table entry", err)
	}
	return connect.NewResponse(&rulesv1.ArchiveTableEntryResponse{Entry: entry, TableRevision: rev}), nil
}

// UnarchiveTableEntry implements rulesv1connect.TableContentServiceHandler.
func (s *Service) UnarchiveTableEntry(
	ctx context.Context,
	req *connect.Request[rulesv1.UnarchiveTableEntryRequest],
) (*connect.Response[rulesv1.UnarchiveTableEntryResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	entry, rev, err := s.setArchived(ctx, m, req.Msg.GetKey(), false)
	if err != nil {
		return nil, s.dbError(ctx, "unarchive a table entry", err)
	}
	return connect.NewResponse(&rulesv1.UnarchiveTableEntryResponse{Entry: entry, TableRevision: rev}), nil
}

// setArchived retires or brings back an entry, in a transaction that bumps the
// revision. Bringing one back checks the whole campaign's overlay again.
func (s *Service) setArchived(ctx context.Context, m authz.Membership, key string, archive bool) (*rulesv1.TableEntry, int32, error) {
	if !rules.IsTableKey(key) {
		return nil, 0, errNoTableEntry()
	}
	var out *rulesv1.TableEntry
	var revision int32
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		rev, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: m.CampaignID, Now: s.now()})
		if err != nil {
			return wrap("bump the content revision", err)
		}
		cur, err := q.GetCampaignContent(ctx, charactersdb.GetCampaignContentParams{CampaignID: m.CampaignID, ContentKey: key})
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoTableEntry()
		}
		if err != nil {
			return wrap("read a table entry", err)
		}
		switch {
		case archive && cur.ArchivedAt != nil:
			return errBlockedContent(connect.CodeFailedPrecondition, rulesv1.TableContentBlockedReason_TABLE_CONTENT_BLOCKED_REASON_ARCHIVED, key, "the entry is already archived")
		case !archive && cur.ArchivedAt == nil:
			return errBlockedContent(connect.CodeFailedPrecondition, rulesv1.TableContentBlockedReason_TABLE_CONTENT_BLOCKED_REASON_NOT_ARCHIVED, key, "the entry is not archived")
		}
		now := s.now()
		row, err := q.SetCampaignContentArchived(ctx, charactersdb.SetCampaignContentArchivedParams{
			CampaignID: m.CampaignID, ContentKey: key, Archived: archive, Now: now,
		})
		if err != nil {
			return wrap("archive a table entry", err)
		}
		if !archive {
			rows, err := q.ListCampaignContent(ctx, m.CampaignID)
			if err != nil {
				return wrap("list the table's content", err)
			}
			entries, err := readEntries(rows) // the list already has the entry unarchived
			if err != nil {
				return err
			}
			if _, err := s.checkOverlay(entries, int(rev), key); err != nil {
				return err
			}
		}
		e, err := decodeRow(row)
		if err != nil {
			return err
		}
		// The master's count: archiving does not change who uses the entry, and the
		// editor shows it ("Arquivado · 2 fichas usam").
		sheets, err := q.ListCampaignSheets(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the campaign's sheets", err)
		}
		uses, err := characterUses(sheets)
		if err != nil {
			return err
		}
		out, revision = entryToProto(e, uses[key]), rev
		offKeys, err := q.ListContentOff(ctx, m.CampaignID)
		if err != nil {
			return wrap("read the options switched off", err)
		}
		out.Off = slices.Contains(offKeys, key)
		return nil
	})
	if err == nil {
		event := "content.entry_archived"
		if !archive {
			event = "content.entry_unarchived"
		}
		logging.Event(ctx, s.logger, event, slog.String("entry_key", key), slog.Int("table_revision", int(revision)))
		s.publishContentChanged(m.CampaignID)
	}
	return out, revision, err
}
