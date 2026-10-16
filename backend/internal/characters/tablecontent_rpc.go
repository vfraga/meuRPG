package characters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

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
	res := &rulesv1.ListTableEntriesResponse{}
	// One transaction: the revision, the rows and the counts are one snapshot.
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		listing, err := s.readTableListing(ctx, tx, m)
		if err != nil {
			return err
		}
		res = listing.response()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "list the table's content", err)
	}
	return connect.NewResponse(res), nil
}

// tableListing is what ListTableEntries read for one viewer, in its transaction.
type tableListing struct {
	master   bool
	revision int32
	entries  []entryRow
	content  *rules.Content
	// uses are the master's counts of the characters that use each entry (nil for a player).
	uses map[string]int
	// hidden is what a player never receives (RN-23): retired or switched off, SRD or
	// table; nil when nothing is.
	hidden func(string) bool
	// mine are the keys the player's own sheets use (nil for the master).
	mine map[string]bool
}

// readTableListing reads, in tx, the revision, the entries, the master's counts, the
// content and the player's own keys, in that order.
func (s *Service) readTableListing(ctx context.Context, tx pgx.Tx, m authz.Membership) (*tableListing, error) {
	q := s.queries.WithTx(tx)
	l := &tableListing{master: isMaster(m)}
	var err error
	if l.revision, err = contentRevision(ctx, q, m.CampaignID); err != nil {
		return nil, err
	}
	rows, err := q.ListCampaignContent(ctx, m.CampaignID)
	if err != nil {
		return nil, wrap("list the table's content", err)
	}
	if l.entries, err = readEntries(rows); err != nil {
		return nil, err
	}
	if l.master {
		if l.uses, err = sheetUses(ctx, q, m.CampaignID); err != nil {
			return nil, err
		}
	}
	if l.content, err = s.contentFor(ctx, tx, m.CampaignID); err != nil {
		return nil, err
	}
	if l.content.AnyHidden() {
		l.hidden = l.content.Hidden
	}
	if !l.master {
		if l.mine, err = callerKeys(ctx, q, m); err != nil {
			return nil, wrap("read the caller's sheets", err)
		}
	}
	return l, nil
}

// sheetUses counts, in q's transaction, the campaign's characters that use each table key.
func sheetUses(ctx context.Context, q *charactersdb.Queries, campaignID string) (map[string]int, error) {
	sheets, err := q.ListCampaignSheets(ctx, campaignID)
	if err != nil {
		return nil, wrap("list the campaign's sheets", err)
	}
	return characterUses(sheets)
}

// response is the listing as the viewer reads it, by kind then name.
func (l *tableListing) response() *rulesv1.ListTableEntriesResponse {
	res := &rulesv1.ListTableEntriesResponse{TableRevision: l.revision, ContentVersion: l.content.Version()}
	for _, e := range l.entries {
		if !l.master {
			if !l.playerReads(e) {
				continue
			}
			e = forPlayer(e, l.hidden)
		}
		pe := entryToProto(e, l.uses[e.row.ContentKey])
		pe.Off = l.content.Off(e.row.ContentKey)
		res.Entries = append(res.Entries, pe)
	}
	slices.SortStableFunc(res.Entries, func(a, b *rulesv1.TableEntry) int {
		if d := tableKindOrder(a.GetKind()) - tableKindOrder(b.GetKind()); d != 0 {
			return d
		}
		return strings.Compare(a.GetNamePt(), b.GetNamePt())
	})
	return res
}

// playerReads says whether a player receives the entry.
func (l *tableListing) playerReads(e entryRow) bool {
	key := e.row.ContentKey
	if e.row.ArchivedAt != nil || l.content.Off(key) {
		return false // a player never receives a retired or switched off entry, or a draft
	}
	// Nor an entry whose class or race is retired or off, unless their sheet uses it.
	return l.hidden == nil || (!parentHidden(e, l.hidden) && !l.hidden(key)) || l.mine[key]
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
	return prepareWith(featureKeys, key, kind, body, old, lead...)
}

// prepareWith is prepare with the way the features get their keys.
func prepareWith(
	keys func(entryKey string, body, old tableBody) []*rulesv1.TableContentViolation,
	key string, kind rulesv1.TableContentKind, body, old tableBody, lead ...*rulesv1.TableContentViolation,
) (stored, error) {
	b := proto.Clone(body).(tableBody)
	// Every problem of the body comes at once: the shape, the feature count and the keys.
	problems := slices.Clone(lead)
	problems = append(problems, checkShape(b)...)
	tooMany := checkFeatureCount(kind, b)
	problems = append(problems, tooMany...)
	if len(tooMany) == 0 { // the keys of that many features are not worth making
		problems = append(problems, keys(key, b, old)...)
	}
	if len(problems) > len(lead) {
		return stored{}, errRefusedContent(problems)
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
	if err := checkWriteBody(body, kind); err != nil {
		return nil, err
	}
	idemKey, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first entry only when it is the same request.
	call := &createCall{s: s, campaignID: m.CampaignID, body: body, kind: kind}
	call.scopedKey, call.requestHash = idem.Scope(m.CampaignID, idemKey), idem.Hash(req.Msg)
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error { return call.run(ctx, tx) })
	if call.replayed && errors.Is(err, errReplayed) {
		return connect.NewResponse(call.res), nil // the first call wrote and announced the entry
	}
	if err != nil {
		return nil, s.dbError(ctx, "create a table entry", err)
	}
	res := call.res
	logging.Event(ctx, s.logger, "content.entry_written", slog.String("entry_key", res.GetEntry().GetKey()), slog.String("kind", kindPrefix(kind)), slog.Int("table_revision", int(res.GetTableRevision())), slog.Bool("created", true))
	s.publishContentChanged(m.CampaignID)
	return connect.NewResponse(res), nil
}

// checkWriteBody refuses, before the transaction, a write without a body, and one over the
// feature limits: the cheap size refusal never holds the campaign's content revision.
func checkWriteBody(body tableBody, kind rulesv1.TableContentKind) error {
	if body == nil {
		return invalidArgument(fieldErr("body", "is required: exactly one of the table_* messages"))
	}
	if v := checkFeatureCount(kind, body); len(v) > 0 {
		return errRefusedContent(v)
	}
	return nil
}

// createCall is one CreateTableEntry: the request and what its transaction found. The
// transaction's closure runs again when CockroachDB retries it, so run starts it all again.
type createCall struct {
	s          *Service
	campaignID string
	body       tableBody
	kind       rulesv1.TableContentKind
	// scopedKey and requestHash are the idempotency key (nil without one) and the request's hash.
	scopedKey, requestHash *string

	res      *rulesv1.CreateTableEntryResponse
	replayed bool
}

// run is the transaction of the call: bump the revision, answer a replay, check the entry
// against the whole campaign, and insert it.
func (c *createCall) run(ctx context.Context, tx pgx.Tx) error {
	q := c.s.queries.WithTx(tx)
	c.replayed = false
	rev, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: c.campaignID, Now: c.s.now()})
	if err != nil {
		return wrap("bump the content revision", err)
	}
	// The bump holds the campaign's content until this transaction ends, so a call with the
	// same key waits here for the first one, and then finds its entry.
	if done, err := c.replay(ctx, q); done {
		return err
	}
	rows, err := q.ListCampaignContent(ctx, c.campaignID)
	if err != nil {
		return wrap("list the table's content", err)
	}
	entries, err := readEntries(rows)
	if err != nil {
		return err
	}
	key := entryKey(c.kind, c.body.GetNamePt(), rows)
	// A name another entry has is one more violation of this entry, listed with the ones the rules find (not instead of them).
	lead := duplicateLead(entries, c.kind, key, c.body.GetNamePt())
	st, err := prepare(key, c.kind, c.body, nil, lead...)
	if err != nil {
		return err
	}
	now := c.s.now()
	row := charactersdb.CampaignContent{
		CampaignID: c.campaignID, ContentKey: key, Kind: kindPrefix(c.kind), NamePt: c.body.GetNamePt(), Data: st.data,
		Revision: rev, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := c.s.checkOverlay(withRow(entries, st, row), int(rev), key, lead...); err != nil {
		return err
	}
	if row, err = c.insert(ctx, q, st, rev, now); err != nil {
		return wrap("insert a table entry", err)
	}
	c.res = &rulesv1.CreateTableEntryResponse{
		// No sheet can use an entry that did not exist a moment ago: the count is 0, set
		// for the master as every write response does.
		Entry: entryToProto(entryRow{row: row, kind: c.kind, body: st.body}, 0), TableRevision: rev,
	}
	return nil
}

// replay finds the entry an earlier call with the same idempotency key made. It reports
// whether the transaction ends here: with errReplayed (the bump is rolled back) when it found
// one, or with the error of the search or of another request under the key.
func (c *createCall) replay(ctx context.Context, q *charactersdb.Queries) (bool, error) {
	if c.scopedKey == nil {
		return false, nil
	}
	prior, err := q.GetCampaignContentByCreateKey(ctx, c.scopedKey)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return true, wrap("find the entry of the key", err)
	}
	if err := idem.SameRequest(prior.CreateHash, c.requestHash); err != nil {
		return true, err
	}
	entry, err := decodeRow(prior)
	if err != nil {
		return true, err
	}
	// The table's revision as it is now. The read sees this transaction's own bump, which
	// the rollback undoes, so the revision is one less: a retry changes nothing.
	current, err := q.GetContentRevision(ctx, c.campaignID)
	if err != nil {
		return true, wrap("read the content revision", err)
	}
	c.res = &rulesv1.CreateTableEntryResponse{Entry: entryToProto(entry, 0), TableRevision: current - 1}
	c.replayed = true
	return true, errReplayed
}

// insert writes the new row, with the idempotency key when the call has one.
func (c *createCall) insert(ctx context.Context, q *charactersdb.Queries, st stored, rev int32, now time.Time) (charactersdb.CampaignContent, error) {
	if c.scopedKey != nil {
		return q.InsertCampaignContentWithKey(ctx, charactersdb.InsertCampaignContentWithKeyParams{
			CampaignID: c.campaignID, ContentKey: st.key, Kind: kindPrefix(c.kind), NamePt: c.body.GetNamePt(), Data: st.data,
			Revision: rev, CreateKey: c.scopedKey, CreateHash: c.requestHash, Now: now,
		})
	}
	return q.InsertCampaignContent(ctx, charactersdb.InsertCampaignContentParams{
		CampaignID: c.campaignID, ContentKey: st.key, Kind: kindPrefix(c.kind), NamePt: c.body.GetNamePt(), Data: st.data,
		Revision: rev, Now: now,
	})
}

// duplicateLead is the violations an entry has before the rules run: its name, when another
// entry of the kind has it.
func duplicateLead(entries []entryRow, kind rulesv1.TableContentKind, key, name string) []*rulesv1.TableContentViolation {
	if v := duplicateName(entries, kind, key, name); v != nil {
		return []*rulesv1.TableContentViolation{v}
	}
	return nil
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
	body, kind, err := checkUpdateRequest(req.Msg)
	if err != nil {
		return nil, err
	}
	call := &updateCall{s: s, campaignID: m.CampaignID, key: key, expected: req.Msg.GetExpectedRevision(), body: body, kind: kind}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error { return call.run(ctx, tx) })
	if err != nil {
		return nil, s.dbError(ctx, "update a table entry", err)
	}
	res := call.res
	logging.Event(ctx, s.logger, "content.entry_written", slog.String("entry_key", key), slog.Int("table_revision", int(res.GetTableRevision())), slog.Bool("created", false))
	s.publishContentChanged(m.CampaignID)
	if res.AffectedCharacters, err = s.affectedToProto(ctx, call.affected); err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}
	return connect.NewResponse(res), nil
}

// checkUpdateRequest refuses, before the transaction, a key that is not the table's, a
// write without a body, a change of kind, and a body over the feature limits.
func checkUpdateRequest(msg *rulesv1.UpdateTableEntryRequest) (tableBody, rulesv1.TableContentKind, error) {
	key := msg.GetKey()
	if !rules.IsTableKey(key) {
		return nil, rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED, errNoTableEntry()
	}
	body, kind := bodyOf(msg)
	if body == nil {
		return nil, rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED, invalidArgument(fieldErr("body", "is required: exactly one of the table_* messages"))
	}
	if kindPrefix(kind) != keyKind(key) {
		return nil, rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED, errRefusedContent([]*rulesv1.TableContentViolation{{
			Field: "body", Reason: reasonImmutable, Message: "the kind of an entry never changes",
		}})
	}
	if err := checkWriteBody(body, kind); err != nil {
		return nil, rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED, err
	}
	return body, kind, nil
}

// updateCall is one UpdateTableEntry: the request and what its transaction found. The
// transaction's closure runs again when CockroachDB retries it, so run sets it all again.
type updateCall struct {
	s          *Service
	campaignID string
	key        string
	expected   int32
	body       tableBody
	kind       rulesv1.TableContentKind

	res      *rulesv1.UpdateTableEntryResponse
	affected []affectedSheet
}

// run is the transaction of the call: bump the revision, read the entry as stored, check
// the new body against the whole campaign, write it, and find the sheets it leaves with
// issues.
func (c *updateCall) run(ctx context.Context, tx pgx.Tx) error {
	q := c.s.queries.WithTx(tx)
	rev, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: c.campaignID, Now: c.s.now()})
	if err != nil {
		return wrap("bump the content revision", err)
	}
	cur, err := c.current(ctx, q)
	if err != nil {
		return err
	}
	plan, err := c.check(ctx, q, cur, rev)
	if err != nil {
		return err
	}
	row, err := q.UpdateCampaignContent(ctx, charactersdb.UpdateCampaignContentParams{
		CampaignID: c.campaignID, ContentKey: c.key, NamePt: c.body.GetNamePt(), Data: plan.st.data, Revision: rev, Now: plan.now,
	})
	if err != nil {
		return wrap("update a table entry", err)
	}
	sheets, err := q.ListCampaignSheets(ctx, c.campaignID)
	if err != nil {
		return wrap("list the campaign's sheets", err)
	}
	if c.affected, err = affectedSheets(sheets, plan.content, c.key); err != nil {
		return err
	}
	uses, err := characterUses(sheets)
	if err != nil {
		return err
	}
	c.res = &rulesv1.UpdateTableEntryResponse{
		Entry: entryToProto(entryRow{row: row, kind: c.kind, body: plan.st.body}, uses[c.key]), TableRevision: rev,
	}
	c.res.Entry.Off, err = switchedOff(ctx, q, c.campaignID, c.key)
	return err
}

// current reads the entry as stored, refusing a missing one and an editor's stale revision.
// An archived entry can still be edited (it stays archived).
func (c *updateCall) current(ctx context.Context, q *charactersdb.Queries) (charactersdb.CampaignContent, error) {
	cur, err := tableRow(ctx, q, c.campaignID, c.key)
	if err != nil {
		return cur, err
	}
	if cur.Revision != c.expected {
		return cur, errBlockedContent(connect.CodeAborted, rulesv1.TableContentBlockedReason_TABLE_CONTENT_BLOCKED_REASON_STALE, c.key, "the entry changed since you opened it; reload it and try again")
	}
	return cur, nil
}

// updatePlan is an update that passed its checks: the body to store, the campaign's content
// with it, and the time of the write.
type updatePlan struct {
	st      stored
	content *rules.Content
	now     time.Time
}

// check refuses what never changes, then checks the new body (its name among the kind's,
// its shape and keys) and the whole campaign's overlay with it.
func (c *updateCall) check(ctx context.Context, q *charactersdb.Queries, cur charactersdb.CampaignContent, rev int32) (updatePlan, error) {
	old, err := decodeRow(cur)
	if err != nil {
		return updatePlan{}, err
	}
	if v := immutableChange(c.kind, old.body, c.body); v != nil {
		return updatePlan{}, errRefusedContent([]*rulesv1.TableContentViolation{v})
	}
	rows, err := q.ListCampaignContent(ctx, c.campaignID)
	if err != nil {
		return updatePlan{}, wrap("list the table's content", err)
	}
	entries, err := readEntries(rows)
	if err != nil {
		return updatePlan{}, err
	}
	lead := duplicateLead(entries, c.kind, c.key, c.body.GetNamePt())
	st, err := prepare(c.key, c.kind, c.body, old.body, lead...)
	if err != nil {
		return updatePlan{}, err
	}
	now := c.s.now()
	next := cur
	next.NamePt, next.Data, next.Revision, next.UpdatedAt = c.body.GetNamePt(), st.data, rev, now
	content, err := c.s.checkOverlay(withRow(entries, st, next), int(rev), c.key, lead...)
	if err != nil {
		return updatePlan{}, err
	}
	return updatePlan{st: st, content: content, now: now}, nil
}

// tableRow reads the stored row of a table key: not_found when the campaign has none.
func tableRow(ctx context.Context, q *charactersdb.Queries, campaignID, key string) (charactersdb.CampaignContent, error) {
	cur, err := q.GetCampaignContent(ctx, charactersdb.GetCampaignContentParams{CampaignID: campaignID, ContentKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return cur, errNoTableEntry()
	}
	if err != nil {
		return cur, wrap("read a table entry", err)
	}
	return cur, nil
}

// switchedOff says whether the master switched the key off, read in q's transaction.
func switchedOff(ctx context.Context, q *charactersdb.Queries, campaignID, key string) (bool, error) {
	offKeys, err := q.ListContentOff(ctx, campaignID)
	if err != nil {
		return false, wrap("read the options switched off", err)
	}
	return slices.Contains(offKeys, key), nil
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
		var err error
		out, revision, err = s.setArchivedIn(ctx, s.queries.WithTx(tx), m.CampaignID, key, archive)
		return err
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

// setArchivedIn is the transaction of setArchived: bump the revision, check the entry's
// state, write the mark, check the overlay of an entry brought back, and answer the entry as
// the master reads it.
func (s *Service) setArchivedIn(ctx context.Context, q *charactersdb.Queries, campaignID, key string, archive bool) (*rulesv1.TableEntry, int32, error) {
	rev, err := q.BumpContentRevision(ctx, charactersdb.BumpContentRevisionParams{CampaignID: campaignID, Now: s.now()})
	if err != nil {
		return nil, 0, wrap("bump the content revision", err)
	}
	cur, err := tableRow(ctx, q, campaignID, key)
	if err != nil {
		return nil, 0, err
	}
	if err := checkArchiveState(cur, key, archive); err != nil {
		return nil, 0, err
	}
	row, err := q.SetCampaignContentArchived(ctx, charactersdb.SetCampaignContentArchivedParams{
		CampaignID: campaignID, ContentKey: key, Archived: archive, Now: s.now(),
	})
	if err != nil {
		return nil, 0, wrap("archive a table entry", err)
	}
	if !archive {
		if err := s.recheckOverlay(ctx, q, campaignID, rev, key); err != nil {
			return nil, 0, err
		}
	}
	e, err := decodeRow(row)
	if err != nil {
		return nil, 0, err
	}
	// The master's count: archiving does not change who uses the entry, and the
	// editor shows it ("Arquivado · 2 fichas usam").
	uses, err := sheetUses(ctx, q, campaignID)
	if err != nil {
		return nil, 0, err
	}
	out := entryToProto(e, uses[key])
	if out.Off, err = switchedOff(ctx, q, campaignID, key); err != nil {
		return out, rev, err
	}
	return out, rev, nil
}

// checkArchiveState refuses archiving an archived entry, and bringing back one that is not.
func checkArchiveState(cur charactersdb.CampaignContent, key string, archive bool) error {
	switch {
	case archive && cur.ArchivedAt != nil:
		return errBlockedContent(connect.CodeFailedPrecondition, rulesv1.TableContentBlockedReason_TABLE_CONTENT_BLOCKED_REASON_ARCHIVED, key, "the entry is already archived")
	case !archive && cur.ArchivedAt == nil:
		return errBlockedContent(connect.CodeFailedPrecondition, rulesv1.TableContentBlockedReason_TABLE_CONTENT_BLOCKED_REASON_NOT_ARCHIVED, key, "the entry is not archived")
	}
	return nil
}

// recheckOverlay runs the whole campaign's overlay again, as the transaction sees it (the
// list already has the entry brought back).
func (s *Service) recheckOverlay(ctx context.Context, q *charactersdb.Queries, campaignID string, rev int32, key string) error {
	rows, err := q.ListCampaignContent(ctx, campaignID)
	if err != nil {
		return wrap("list the table's content", err)
	}
	entries, err := readEntries(rows)
	if err != nil {
		return err
	}
	_, err = s.checkOverlay(entries, int(rev), key)
	return err
}
