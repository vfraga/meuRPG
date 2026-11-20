package characters

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The writes of InfusionService: InfuseItem, MoveInfusion and EndInfusion. Each one runs under the session funnel
// (Service.inSession), as SetEquipped does, and locks the infuser's character row first: that lock is what serializes the
// count against the limit, so two infusions that would pass it cannot both succeed. A write is valid on a locked sheet,
// as equipping is, and refused for a dead character, as equipping is.

// infusionChoice is where an infusion goes: a gear key of the sheet, or an object the player names (gearKey empty).
type infusionChoice struct {
	gearKey, objectName string
}

func (c infusionChoice) object() bool { return c.gearKey == "" }

// infusionResult is what a write leaves for its answer.
type infusionResult struct {
	infusion *charactersv1.SheetInfusion
	endedID  string
	changed  bool
	revision int32 // the infuser's sheet revision after the write (or as it stands, for an answer that wrote nothing)
}

// infusionsChanged is the one call the three writes make after their change, with everything the write changed (the records
// that ended first, then the one infused or moved): one call for a write, so play writes its events in the order they happened
// and tells the streams once. It raises the infuser's revision, so a sheet that is open (and an editor that holds the old
// number) reads again, tells play, and tells the master and the owner that the infusions changed.
func (s *Service) infusionsChanged(ctx context.Context, tx pgx.Tx, t sessiontell.Tell, m authz.Membership, current charactersdb.Character, changes []link.InfusionChange) (charactersdb.Character, error) {
	q := s.queries.WithTx(tx)
	row, err := q.TouchCharacterRevision(ctx, charactersdb.TouchCharacterRevisionParams{CampaignID: m.CampaignID, ID: current.ID, Now: s.now()})
	if err != nil {
		return current, wrap("raise the revision of an infuser", err)
	}
	t.Touch()
	return row, s.reportInfusions(ctx, tx, t, m.CampaignID, m.UserID, row, changes)
}

// reportInfusions tells play what a write changed in the records, in the transaction of the write (play writes the session
// event of each change when the campaign has an open session; play may refuse, and the write rolls back), and tells the
// master and the owner of the artificer (the holder is always the artificer in this version) that its infusions changed once
// the transaction commits. It tells nothing for a write that changed nothing.
func (s *Service) reportInfusions(ctx context.Context, tx pgx.Tx, t sessiontell.Tell, campaignID, actorUserID string, current charactersdb.Character, changes []link.InfusionChange) error {
	if len(changes) == 0 {
		return nil
	}
	if s.reviewHost != nil {
		if err := s.reviewHost.InfusionChanged(ctx, tx, campaignID, actorUserID, changes, t); err != nil {
			return err
		}
	}
	t.CharacterEvent(conv.OrZero(current.PlayerUserID), current.ID, hintInfusionsChanged)
	return nil
}

// checkInfuser refuses what every write refuses of the locked infuser, in this order: a dead character, a stale revision.
func checkInfuser(current charactersdb.Character, revision int32) error {
	if current.Status == statusDead {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, current.ID)
	}
	if revision != 0 && revision != current.Revision {
		return errStaleRevision()
	}
	return nil
}

// checkEndInfuser is checkInfuser for ending a record: the infusions of a dead artificer stay until the master ends them, so
// the master may end one; the owner is refused as for any write.
func checkEndInfuser(m authz.Membership, current charactersdb.Character, revision int32) error {
	if current.Status == statusDead && isMaster(m) {
		if revision != 0 && revision != current.Revision {
			return errStaleRevision()
		}
		return nil
	}
	return checkInfuser(current, revision)
}

// infusionKeyOf checks the idempotency key of a request: empty is none, anything else is a UUID.
func infusionKeyOf(raw string) (string, error) {
	key, err := idem.Clean(raw)
	if err != nil || key == "" {
		return "", err
	}
	canonical, ok := uid.Canonical(key)
	if !ok {
		return "", invalidArgument(fieldErr("idempotency_key", "must be a UUID"))
	}
	return canonical, nil
}

// infusionReplay finds the answer kept under the idempotency key, if the key was used: the same request has its first answer,
// another request is refused. It runs before any other check, so a retry is answered even when the sheet changed since.
func (s *Service) infusionReplay(ctx context.Context, q *charactersdb.Queries, campaignID, key string, hash *string) (charactersdb.CharacterInfusionRequest, bool, error) {
	if key == "" {
		return charactersdb.CharacterInfusionRequest{}, false, nil
	}
	stored, err := q.GetCharacterInfusionRequest(ctx, charactersdb.GetCharacterInfusionRequestParams{CampaignID: campaignID, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return stored, false, nil
	}
	if err != nil {
		return stored, false, wrap("read the answer of an idempotency key", err)
	}
	if err := idem.SameRequest(&stored.RequestHash, hash); err != nil {
		return stored, false, err
	}
	return stored, true, nil
}

// infusionRequestKey is the primary key of the answers kept under an idempotency key.
const infusionRequestKey = "character_infusion_requests_pkey"

// keepInfusionRequest keeps the answer under the key, in the transaction of the write.
func (s *Service) keepInfusionRequest(ctx context.Context, q *charactersdb.Queries, campaignID, key string, hash *string, infusionID, endedID string) error {
	if key == "" {
		return nil
	}
	var ended *string
	if endedID != "" {
		ended = &endedID
	}
	if err := q.InsertCharacterInfusionRequest(ctx, charactersdb.InsertCharacterInfusionRequestParams{
		CampaignID: campaignID, IdempotencyKey: key, RequestHash: *hash, InfusionID: infusionID, EndedInfusionID: ended, CreatedAt: s.now(),
	}); err != nil {
		if db.IsUniqueViolation(err, infusionRequestKey) {
			return idem.ErrReused() // another call kept an answer under the key since the replay was read: the key is taken
		}
		return wrap("keep the answer of an idempotency key", err)
	}
	return nil
}

// InfuseItem implements charactersv1connect.InfusionServiceHandler.
func (s *Service) InfuseItem(
	ctx context.Context,
	req *connect.Request[charactersv1.InfuseItemRequest],
) (*connect.Response[charactersv1.InfuseItemResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	msg := req.Msg
	id, ok := uid.Canonical(msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	if msg.GetInfusionKey() == "" {
		return nil, invalidArgument(fieldErr("infusion_key", "is required"))
	}
	if err := checkInfusionHolder(msg.GetHolderCharacterId(), id); err != nil {
		return nil, err
	}
	key, err := infusionKeyOf(msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var done infusionResult
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		done, err = s.infuseIn(ctx, tx, content, m, msg, key, t)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "infuse an item", err)
	}
	if done.changed {
		logging.Event(ctx, s.logger, "character.infused", slog.String("character_id", id), slog.String("infusion_key", msg.GetInfusionKey()),
			slog.Bool("by_master", isMaster(m)), slog.String("by_user_id", m.UserID))
	}
	return connect.NewResponse(&charactersv1.InfuseItemResponse{Infusion: done.infusion, EndedInfusionId: done.endedID, Revision: done.revision}), nil
}

// infuseIn is the transaction of InfuseItem: the infuser locked, the replay of the key answered, the infusion checked
// (known, not in use, the item taken or not, the limit), the records changed and the attunement made.
func (s *Service) infuseIn(ctx context.Context, tx pgx.Tx, content *rules.Content, m authz.Membership, msg *charactersv1.InfuseItemRequest, key string, t sessiontell.Tell) (infusionResult, error) {
	w := infusionTx{tx: tx, q: s.queries.WithTx(tx), content: content, m: m, t: t, key: key, hash: idem.Hash(msg)}
	current, replay, err := s.openInfusionWrite(ctx, w, msg.GetCharacterId(), msg.GetRevision())
	if err != nil || replay != nil {
		return orEmpty(replay), err
	}
	v, err := s.infusionViewOf(ctx, w.q, content, current)
	if err != nil {
		return infusionResult{}, err
	}
	plan, err := v.planInfuse(msg)
	if err != nil {
		return infusionResult{}, err
	}
	changes, err := s.endForInfusion(ctx, w, plan.taken, plan.ends)
	if err != nil {
		return infusionResult{}, err
	}
	rec, err := s.insertInfusion(ctx, w.q, newInfusion{
		CampaignID: m.CampaignID, ArtificerID: current.ID, HolderID: current.ID, InfusionKey: msg.GetInfusionKey(),
		ItemKind: plan.kind, GearKey: plan.choice.gearKey, ObjectName: plan.choice.objectName, Parameter: msg.GetParameter(),
	})
	if err != nil {
		return infusionResult{}, err
	}
	if msg.GetAttuneNow() {
		if err := s.attuneNew(ctx, w.q, content, m, current, v.raw, rec.ID); err != nil {
			return infusionResult{}, err
		}
	}
	return s.closeInfusionWrite(ctx, w, current, rec.ID, firstOf(plan.ends), append(changes, recordChange(link.InfusionInfused, rec)))
}

// infusionTx is what a write works on: the queries of its transaction, the content, the caller, what it tells, and the
// idempotency key of the request with its hash.
type infusionTx struct {
	tx      pgx.Tx
	q       *charactersdb.Queries
	content *rules.Content
	m       authz.Membership
	t       sessiontell.Tell
	key     string
	hash    *string
}

// firstOf is the first of the ids, empty when there is none: the one an answer names.
func firstOf(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func orEmpty(r *infusionResult) infusionResult {
	if r == nil {
		return infusionResult{}
	}
	return *r
}

// openInfusionWrite locks the infuser, answers a retry of the idempotency key (the replay is not nil then: nothing more
// is written) and refuses a dead character and a stale revision.
func (s *Service) openInfusionWrite(ctx context.Context, w infusionTx, characterID string, revision int32) (charactersdb.Character, *infusionResult, error) {
	id, _ := uid.Canonical(characterID)
	current, err := visibleForUpdate(ctx, w.q, w.m, id)
	if err != nil {
		return current, nil, err
	}
	stored, replayed, err := s.infusionReplay(ctx, w.q, w.m.CampaignID, w.key, w.hash)
	if err != nil || replayed {
		if err != nil {
			return current, nil, err
		}
		out, err := s.replayedInfusion(ctx, w.q, w.content, current, stored)
		return current, &out, err
	}
	return current, nil, checkInfuser(current, revision)
}

// endForInfusion ends the records a write gives up: the ones on the item that are not valid now (`taken`) and the one a new
// infusion replaces (`replaced`). It returns what ended, in that order, for the write to tell play.
func (s *Service) endForInfusion(ctx context.Context, w infusionTx, taken, replaced []string) ([]link.InfusionChange, error) {
	var ended []link.InfusionChange
	for _, end := range []struct {
		ids    []string
		reason string
	}{{taken, infusionEndedTaken}, {replaced, infusionEndedReplaced}} {
		for _, id := range end.ids {
			rec, ok, err := s.endInfusion(ctx, w.q, w.m.CampaignID, id, end.reason)
			if err != nil {
				return nil, err
			}
			if ok {
				ended = append(ended, endedChange(rec, end.reason))
			}
		}
	}
	return ended, nil
}

// closeInfusionWrite is what every write that changed a record ends with: the seam, the answer kept under the key, and the
// infusion as the sheet reads it now.
func (s *Service) closeInfusionWrite(ctx context.Context, w infusionTx, current charactersdb.Character, recordID, endedID string, changes []link.InfusionChange) (infusionResult, error) {
	row, err := s.infusionsChanged(ctx, w.tx, w.t, w.m, current, changes)
	if err != nil {
		return infusionResult{}, err
	}
	if err := s.keepInfusionRequest(ctx, w.q, w.m.CampaignID, w.key, w.hash, recordID, endedID); err != nil {
		return infusionResult{}, err
	}
	after, err := s.infusionViewOf(ctx, w.q, w.content, row)
	if err != nil {
		return infusionResult{}, err
	}
	infusion, err := s.infusionAnswerOf(ctx, w.q, after, recordID)
	return infusionResult{infusion: infusion, endedID: endedID, changed: true, revision: row.Revision}, err
}

// replayedInfusion answers a retry from what the first call kept, writing nothing.
func (s *Service) replayedInfusion(ctx context.Context, q *charactersdb.Queries, content *rules.Content, current charactersdb.Character, stored charactersdb.CharacterInfusionRequest) (infusionResult, error) {
	v, err := s.infusionViewOf(ctx, q, content, current)
	if err != nil {
		return infusionResult{}, err
	}
	infusion, err := s.infusionAnswerOf(ctx, q, v, stored.InfusionID)
	if err != nil {
		return infusionResult{}, err
	}
	out := infusionResult{infusion: infusion, revision: current.Revision}
	if stored.EndedInfusionID != nil {
		out.endedID = *stored.EndedInfusionID
	}
	return out, nil
}

// infusionPlan is what a request's checks decide: where the record goes, which records end (the ones that are not valid now
// on the item, with `taken`, and the one a new infusion replaces).
type infusionPlan struct {
	kind   string
	choice infusionChoice
	taken  []string
	ends   []string // the valid records that end with `replaced`: the one the caller named on the piece, or the one that makes room
}

// planInfuse checks an InfuseItem request against the sheet and its records, in the order the caller is answered: the
// infusion is known and not in use, the record to end is one the character holds, the parameter, the item, the limit.
// A write never pushes a valid record out silently: with the limit full the one record that gives way is a record that
// counts, which ends with its event; naming a record that does not count, or infusing a piece that carries one over the
// limit, is refused, since ending such a record frees no place.
func (v infusionView) planInfuse(msg *charactersv1.InfuseItemRequest) (infusionPlan, error) {
	info, ok := v.knownData(msg.GetInfusionKey())
	if !ok {
		return infusionPlan{}, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_INFUSION_NOT_KNOWN)
	}
	stale, err := v.staleOf(msg.GetInfusionKey())
	if err != nil {
		return infusionPlan{}, err
	}
	if endID := msg.GetEndInfusionId(); endID != "" && v.heldRecord(endID) == nil {
		return infusionPlan{}, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED)
	}
	if err := checkParameter(info, msg.GetParameter()); err != nil {
		return infusionPlan{}, err
	}
	choice, err := choiceOf(msg.GetItemKey(), msg.GetObjectNamePt())
	if err != nil {
		return infusionPlan{}, err
	}
	// A record named on the very piece is a replacement: the new infusion goes on the piece in its place.
	here := v.validOnPiece(msg.GetEndInfusionId(), choice)
	taken, err := v.checkItem(info, choice, here)
	if err != nil {
		return infusionPlan{}, err
	}
	plan := infusionPlan{kind: infusionItemGear, choice: choice, taken: addOnce(taken, stale)}
	if choice.object() {
		plan.kind = infusionItemObject
	}
	if msg.GetAttuneNow() && !info.RequiresAttunement {
		return infusionPlan{}, invalidArgument(fieldErr("attune_now", "is for an infusion that asks for attunement"))
	}
	plan.ends, err = v.makeRoom(msg.GetInfusionKey(), msg.GetEndInfusionId(), here, taken)
	plan.taken = slices.DeleteFunc(plan.taken, func(id string) bool { return slices.Contains(plan.ends, id) || id == here })
	if here != "" {
		plan.ends = append([]string{here}, slices.DeleteFunc(plan.ends, func(id string) bool { return id == here })...)
	}
	return plan, err
}

// makeRoom says which valid record ends for a new infusion of the key, and refuses what would push a valid record out with no
// event: with the limit full, a piece that carries a record over the limit of the host (here, or one of the records on it that
// are not valid now, taken) is refused, named or not, and so is a named record that does not count (see replaced). A named
// record on the piece that is not valid for another reason ends `taken` named or not: the name asks for nothing more.
func (v infusionView) makeRoom(infusionKey, named, here string, taken []string) ([]string, error) {
	host := v.content.InfusionHost(infusionKey)
	if v.hostFull(host) && v.anyOverLimit(host, append([]string{here}, taken...)) {
		return nil, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED)
	}
	if slices.Contains(taken, named) {
		named = ""
	}
	return v.replaced(infusionKey, named)
}

// validOnPiece is the id when it names a valid record, or one the limit pushed out (only a host with a place free can hold
// one of another host, since a record is over the limit only while its host is full), that sits on the gear piece of the
// choice, "" otherwise.
func (v infusionView) validOnPiece(id string, c infusionChoice) string {
	if id == "" || c.object() {
		return ""
	}
	for _, st := range v.derived.Infusions {
		if st.ID == id && (st.Valid || st.OverLimit) && st.ItemKind == rules.InfusionItemGear && st.ItemKey == c.gearKey {
			return id
		}
	}
	return ""
}

// staleOf is the live record of the infusion that is not valid now, which a new infusion of the same key ends with `taken`
// (the list offers such an infusion as known). A valid one means the infusion is in use: ITEM_CHANGED.
func (v infusionView) staleOf(infusionKey string) (string, error) {
	live := v.liveOf(infusionKey)
	if live == nil {
		return "", nil
	}
	for _, st := range v.derived.Infusions {
		if st.ID == live.ID && !st.Valid {
			return live.ID, nil
		}
	}
	return "", errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED)
}

// addOnce adds id to ids unless it is empty or already there.
func addOnce(ids []string, id string) []string {
	if id == "" || slices.Contains(ids, id) {
		return ids
	}
	return append(ids, id)
}

// replaced says which valid record ends to make room for a new infusion of the host: none under the limit, where a name asks
// for nothing; at it, the one the caller named, which must count (a valid record of the same feature: one the limit pushed
// out frees no place, and ending it would push the oldest valid record out with no event), else the oldest valid one (since,
// then id). The derivation never counts more valid records than the limit, so one makes room; a record the limit pushed out
// stays live and ends by no write of its own (the player ends it with EndInfusion). A host with no limit has no room to make:
// the character does not know the infusion.
func (v infusionView) replaced(infusionKey, named string) ([]string, error) {
	host := v.content.InfusionHost(infusionKey)
	limit, used := v.hostUse(host)
	if used < limit {
		return nil, nil
	}
	if limit <= 0 {
		return nil, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_INFUSION_NOT_KNOWN)
	}
	if named != "" {
		if !v.endableOf(host, named) {
			return nil, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED)
		}
		return []string{named}, nil
	}
	for _, st := range v.derived.Infusions { // the order of the records: since, then id
		if st.Valid && st.HostKey == host {
			return []string{st.ID}, nil
		}
	}
	return nil, nil
}

// hostUse is the limit of the host at the character's level and how many valid records it holds.
func (v infusionView) hostUse(host string) (limit, used int) {
	for _, h := range v.derived.InfusionHosts {
		if h.HostKey == host {
			limit, used = h.Limit, h.Used
		}
	}
	return limit, used
}

// hostFull says the host's limit is full (or there is none): a new infusion of the host makes one valid record give way.
func (v infusionView) hostFull(host string) bool {
	limit, used := v.hostUse(host)
	return used >= limit
}

// anyOverLimit says one of the records is over the limit of the host.
func (v infusionView) anyOverLimit(host string, ids []string) bool {
	for _, st := range v.derived.Infusions {
		if st.OverLimit && st.HostKey == host && slices.Contains(ids, st.ID) {
			return true
		}
	}
	return false
}

// endableOf says whether the record is a valid one of the host: the only kind whose ending frees a place.
func (v infusionView) endableOf(host, id string) bool {
	for _, st := range v.derived.Infusions {
		if st.ID == id && st.Valid && st.HostKey == host {
			return true
		}
	}
	return false
}

// liveOf is the live record of the infusion, nil when it has none.
func (v infusionView) liveOf(infusionKey string) *rules.InfusionRecord {
	for i := range v.records {
		if v.records[i].InfusionKey == infusionKey {
			return &v.records[i]
		}
	}
	return nil
}

// heldRecord is the live record the character holds with the id, nil when there is none: another character's, another
// campaign's or an ended one is not there, and the answer does not say which.
func (v infusionView) heldRecord(id string) *rules.InfusionRecord {
	for i := range v.records {
		if v.records[i].ID == id {
			return &v.records[i]
		}
	}
	return nil
}

// checkParameter checks the value an infusion asks for when it is put on: one of its damage types for a resistance label,
// none for any other.
func checkParameter(info rules.TableInfusion, parameter string) error {
	if info.Effect != rules.InfusionEffectResistanceLabel {
		if parameter != "" {
			return invalidArgument(fieldErr("parameter", "is for an infusion that asks for a damage type"))
		}
		return nil
	}
	for _, d := range info.DamageTypes {
		if d == parameter {
			return nil
		}
	}
	return invalidArgument(fieldErr("parameter", "must be one of the damage types of the infusion"))
}

// choiceOf checks the shape of the item of a request: a gear key, or the name of an object, cleaned as names are (1 to 40
// characters, one line, no control or hidden characters, NFC).
func choiceOf(itemKey, objectName string) (infusionChoice, error) {
	if itemKey != "" {
		if strings.TrimSpace(objectName) != "" {
			return infusionChoice{}, invalidArgument(fieldErr("object_name_pt", "is for an object: leave it empty when the item is a piece of the sheet"))
		}
		return infusionChoice{gearKey: itemKey}, nil
	}
	name, err := names.Clean(objectName, maxObjectNameRunes)
	if err != nil {
		return infusionChoice{}, invalidArgument(&fieldError{field: "object_name_pt", err: err})
	}
	return infusionChoice{objectName: name}, nil
}

// checkItem says whether the infusion goes on the item, and which records on it end because they are not valid now. self is
// the record that moves, which does not take its own item.
func (v infusionView) checkItem(info rules.TableInfusion, c infusionChoice, self string) ([]string, error) {
	notAccepted := errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_NOT_ACCEPTED)
	if c.object() {
		if !rules.InfusionTakesObject(info.Accepts) {
			return nil, notAccepted
		}
		return nil, nil // an object is new: it has no record on it yet
	}
	var piece *rules.GearPiece
	for _, p := range rules.DeriveGear(v.build, v.content, v.derived) {
		if p.Key == c.gearKey {
			piece = &p
			break
		}
	}
	if piece == nil || !v.content.InfusionAcceptsGear(info.Accepts, piece.Kind, piece.Key) {
		return nil, notAccepted
	}
	if _, magical := rules.MagicalGear(v.raw, c.gearKey); magical {
		return nil, notAccepted
	}
	var taken []string
	for _, st := range v.derived.Infusions {
		if st.ID == self || st.ItemKind != rules.InfusionItemGear || st.ItemKey != c.gearKey {
			continue
		}
		if st.Valid {
			return nil, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_ALREADY_INFUSED)
		}
		taken = append(taken, st.ID)
	}
	return taken, nil
}

// attuneNew attunes the holder to the new record with the checks of SetAttunement: the owner needs the window of a rest,
// a fourth item is refused to both, and the window is spent. A refusal rolls the whole infusion back.
func (s *Service) attuneNew(ctx context.Context, q *charactersdb.Queries, content *rules.Content, m authz.Membership, current charactersdb.Character, raw rules.Build, recordID string) error {
	build, err := s.withInfusions(ctx, q, current.ID, raw)
	if err != nil {
		return err
	}
	facts := checkFacts{met: rules.RestrictionMet, attuned: rules.AttunedCount(build, content)}
	if err := s.checkAttunement(ctx, q, m, current, attunementRequest{id: current.ID, entryID: infusionEntryID(recordID), attuned: true}, facts); err != nil {
		return err
	}
	if live, err := s.setInfusionAttuned(ctx, q, m.CampaignID, recordID, true); err != nil || !live {
		return err
	}
	if !isMaster(m) {
		if err := q.SetAttunementOpen(ctx, charactersdb.SetAttunementOpenParams{CharacterID: current.ID, Open: false, Now: s.now()}); err != nil {
			return wrap("spend the window to attune", err)
		}
	}
	return nil
}

// infusionAnswerOf is the infusion with the id as the sheet reads it now. A record that ended meanwhile (a retry that comes
// after it) is answered as the row it was, not valid now.
func (s *Service) infusionAnswerOf(ctx context.Context, q *charactersdb.Queries, v infusionView, id string) (*charactersv1.SheetInfusion, error) {
	for _, si := range sheetInfusionsOf(v.row.ID, v.derived, v.records) {
		if si.GetId() == id {
			return si, nil
		}
	}
	r, err := q.GetCharacterInfusion(ctx, charactersdb.GetCharacterInfusionParams{CampaignID: conv.OrZero(v.row.CampaignID), ID: id})
	if err != nil {
		return nil, wrap("read an infusion", err)
	}
	return &charactersv1.SheetInfusion{
		Id: r.ID, InfusionKey: r.InfusionKey, NamePt: v.content.NamePT(r.InfusionKey), HolderCharacterId: r.HolderCharacterID,
		ItemKind: infusionItemKindToProto[r.ItemKind], ItemKey: itemKeyOfGear(rules.InfusionRecord{ItemKind: r.ItemKind, ItemKey: r.ItemKey}),
		ObjectNamePt: r.ObjectName, Parameter: r.Parameter, Since: timestamppb.New(r.Since), Attuned: r.Attuned,
	}, nil
}

// MoveInfusion implements charactersv1connect.InfusionServiceHandler.
func (s *Service) MoveInfusion(
	ctx context.Context,
	req *connect.Request[charactersv1.MoveInfusionRequest],
) (*connect.Response[charactersv1.MoveInfusionResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	msg := req.Msg
	id, ok := uid.Canonical(msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	if msg.GetInfusionId() == "" {
		return nil, invalidArgument(fieldErr("infusion_id", "is required"))
	}
	key, err := infusionKeyOf(msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var done infusionResult
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		done, err = s.moveIn(ctx, tx, content, m, msg, key, t)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "move an infusion", err)
	}
	if done.changed {
		logging.Event(ctx, s.logger, "character.infusion_moved", slog.String("character_id", id), slog.Bool("by_master", isMaster(m)), slog.String("by_user_id", m.UserID))
	}
	return connect.NewResponse(&charactersv1.MoveInfusionResponse{Infusion: done.infusion, Revision: done.revision}), nil
}

// moveKeepsValid says the move leaves every other valid record valid. A move starts the record's time again, so a record that
// is not valid now (one the limit pushed out, one whose piece left the sheet, one that lost its place on a piece) may count
// after it, as the newest; with the limit full the oldest valid record would then go over the limit with no event. It
// reads the sheet as it will be after the move: the record on its new item with the time of the move, the records on the
// new piece that end with it gone. A record that cannot count after the move (the pick or the prerequisite lost, another
// artificer's) is refused before, as an infusion the character does not know, or leaves the valid ones as they are.
func (v infusionView) moveKeepsValid(rec charactersdb.CharacterInfusion, c infusionChoice, ended []string, at time.Time) bool {
	after := slices.DeleteFunc(slices.Clone(v.records), func(r rules.InfusionRecord) bool { return slices.Contains(ended, r.ID) })
	for i := range after {
		if after[i].ID != rec.ID {
			continue
		}
		after[i].ItemKind, after[i].ItemKey, after[i].ObjectName = infusionItemGear, c.gearKey, ""
		if c.object() {
			after[i].ItemKind, after[i].ItemKey, after[i].ObjectName = infusionItemObject, objectItemKey(rec.ID), c.objectName
		}
		after[i].Attuned, after[i].Since = false, at
	}
	b := v.build
	b.Infusions = after
	now := map[string]bool{}
	for _, st := range rules.Derive(b, v.content).Infusions {
		now[st.ID] = st.Valid
	}
	for _, st := range v.derived.Infusions {
		if st.Valid && st.ID != rec.ID && !now[st.ID] {
			return false
		}
	}
	return true
}

// liveRecordRow reads the record the request names, locked, and refuses it with ITEM_CHANGED when it is not a live record of
// the infuser's.
func (s *Service) liveRecordRow(ctx context.Context, q *charactersdb.Queries, current charactersdb.Character, rawID string) (charactersdb.CharacterInfusion, error) {
	changed := errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED)
	id, ok := uid.Canonical(rawID)
	if !ok {
		return charactersdb.CharacterInfusion{}, changed
	}
	r, err := q.GetCharacterInfusionForUpdate(ctx, charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: conv.OrZero(current.CampaignID), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, changed
	}
	if err != nil {
		return r, wrap("read an infusion", err)
	}
	if r.EndedAt != nil || r.ArtificerCharacterID != current.ID || r.HolderCharacterID != current.ID {
		return r, changed
	}
	return r, nil
}

// moveIn is the transaction of MoveInfusion.
func (s *Service) moveIn(ctx context.Context, tx pgx.Tx, content *rules.Content, m authz.Membership, msg *charactersv1.MoveInfusionRequest, key string, t sessiontell.Tell) (infusionResult, error) {
	w := infusionTx{tx: tx, q: s.queries.WithTx(tx), content: content, m: m, t: t, key: key, hash: idem.Hash(msg)}
	current, replay, err := s.openInfusionWrite(ctx, w, msg.GetCharacterId(), msg.GetRevision())
	if err != nil || replay != nil {
		return orEmpty(replay), err
	}
	rec, err := s.liveRecordRow(ctx, w.q, current, msg.GetInfusionId())
	if err != nil {
		return infusionResult{}, err
	}
	v, err := s.infusionViewOf(ctx, w.q, content, current)
	if err != nil {
		return infusionResult{}, err
	}
	info, ok := v.knownData(rec.InfusionKey)
	if !ok {
		return infusionResult{}, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_INFUSION_NOT_KNOWN)
	}
	choice, err := choiceOf(msg.GetItemKey(), msg.GetObjectNamePt())
	if err != nil {
		return infusionResult{}, err
	}
	taken, err := v.checkItem(info, choice, rec.ID)
	if err != nil {
		return infusionResult{}, err
	}
	if !choice.object() && rec.ItemKind == infusionItemGear && rec.ItemKey == choice.gearKey {
		infusion, err := s.infusionAnswerOf(ctx, w.q, v, rec.ID) // already there: nothing moves
		return infusionResult{infusion: infusion, revision: current.Revision}, err
	}
	if !v.moveKeepsValid(rec, choice, taken, s.now()) {
		return infusionResult{}, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED)
	}
	changes, err := s.endForInfusion(ctx, w, taken, nil)
	if err != nil {
		return infusionResult{}, err
	}
	kind, itemKey := infusionItemGear, choice.gearKey
	if choice.object() {
		kind, itemKey = infusionItemObject, objectItemKey(rec.ID)
	}
	if _, err := s.moveInfusion(ctx, w.q, m.CampaignID, rec.ID, kind, choice.gearKey, choice.objectName); err != nil {
		return infusionResult{}, err
	}
	moved := recordChange(link.InfusionMoved, rec)
	moved.ItemKind, moved.ItemKey = kind, itemKey
	return s.closeInfusionWrite(ctx, w, current, rec.ID, "", append(changes, moved))
}

// EndInfusion implements charactersv1connect.InfusionServiceHandler.
func (s *Service) EndInfusion(
	ctx context.Context,
	req *connect.Request[charactersv1.EndInfusionRequest],
) (*connect.Response[charactersv1.EndInfusionResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	msg := req.Msg
	id, ok := uid.Canonical(msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	if msg.GetInfusionId() == "" {
		return nil, invalidArgument(fieldErr("infusion_id", "is required"))
	}
	var ended endedInfusion
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		ended, err = s.endIn(ctx, tx, m, msg, t)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "end an infusion", err)
	}
	logging.Event(ctx, s.logger, "character.infusion_ended", slog.String("character_id", id), slog.Bool("by_master", isMaster(m)), slog.String("by_user_id", m.UserID))
	return connect.NewResponse(&charactersv1.EndInfusionResponse{EndedInfusionId: ended.id, Revision: ended.revision}), nil
}

// endedInfusion is what EndInfusion answers: the record that ended and the infuser's revision after it.
type endedInfusion struct {
	id       string
	revision int32
}

// endIn is the transaction of EndInfusion.
func (s *Service) endIn(ctx context.Context, tx pgx.Tx, m authz.Membership, msg *charactersv1.EndInfusionRequest, t sessiontell.Tell) (endedInfusion, error) {
	q := s.queries.WithTx(tx)
	id, _ := uid.Canonical(msg.GetCharacterId())
	current, err := visibleForUpdate(ctx, q, m, id)
	if err != nil {
		return endedInfusion{}, err
	}
	if err := checkEndInfuser(m, current, msg.GetRevision()); err != nil {
		return endedInfusion{}, err
	}
	rec, err := s.liveRecordRow(ctx, q, current, msg.GetInfusionId())
	if err != nil {
		return endedInfusion{}, err
	}
	ended, ok, err := s.endInfusion(ctx, q, m.CampaignID, rec.ID, infusionEndedEnded)
	if err != nil {
		return endedInfusion{}, err
	}
	if !ok { // the row was locked and live a moment ago: a record that ended anyway is a change, not a zero record for play
		return endedInfusion{}, errInfusionBlocked(charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED)
	}
	row, err := s.infusionsChanged(ctx, tx, t, m, current, []link.InfusionChange{endedChange(ended, infusionEndedEnded)})
	return endedInfusion{id: rec.ID, revision: row.Revision}, err
}
