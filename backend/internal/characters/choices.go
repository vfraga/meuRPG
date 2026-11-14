package characters

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The choices a class or a race asks (PM-05): which ones a sheet has, what is picked and
// what is open. The questions and their rules are package rules' (choicegroups.go);
// this file checks who may ask, turns the answer into messages and writes the picks of
// a sheet that is locked for the player (RN-01).

const (
	// maxChoicePicks bounds the picks of one CompleteCharacterChoices.
	maxChoicePicks = 50
	// maxChoiceKeyLength bounds the key of a choice in a request.
	maxChoiceKeyLength = 120

	// eventCharacterChoicesCompleted is the kind of the session event that tells the
	// master a player completed choices of a locked sheet (session_event_kinds).
	// Its payload holds IDs and content keys only.
	eventCharacterChoicesCompleted = "character_choices_completed"
)

// PreviewChoices implements charactersv1connect.CharacterServiceHandler.
func (s *Service) PreviewChoices(
	ctx context.Context,
	req *connect.Request[charactersv1.PreviewChoicesRequest],
) (*connect.Response[charactersv1.PreviewChoicesResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var sheet *charactersv1.CharacterSheet
	if req.Msg.GetCharacterId() == "" {
		kind, ok := kindToDB[req.Msg.GetKind()]
		if !ok {
			return nil, invalidArgument(fieldErr("kind", "is required"))
		}
		if err := checkCreatorKind(m, kind); err != nil {
			return nil, err
		}
		if sheet, err = checkNewSheet(content, m, kind, req.Msg.GetSheet()); err != nil {
			return nil, err
		}
	} else if sheet, err = s.sheetForChoices(ctx, m, content, req.Msg.GetCharacterId(), req.Msg.GetSheet()); err != nil {
		return nil, err
	}
	full := sheet.GetFull()
	if full == nil {
		return nil, invalidArgument(fieldErr("sheet.full", "is required: a basic sheet has no choices"))
	}
	build := buildOf(full)
	return connect.NewResponse(choicesToProto(viewContent(content, m), build)), nil
}

// sheetForChoices is the sheet PreviewChoices reads for a character: the draft the
// caller sends when they may edit the sheet now (the checks of UpdateCharacter), else
// the stored sheet with only the draft's picks (feature_choice_keys and
// feature_choice_text) laid over it, so the page that completes the choices of a locked
// sheet previews a pick without being able to read or change anything else.
func (s *Service) sheetForChoices(ctx context.Context, m authz.Membership, content *rules.Content, characterID string, raw *charactersv1.CharacterSheet) (*charactersv1.CharacterSheet, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return nil, errCharacterNotFound()
	}
	row, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return nil, s.dbError(ctx, "read a character to read its choices", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return nil, errCharacterNotFound()
	}
	stored, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, s.dbError(ctx, "read a character to read its choices", err)
	}
	if raw == nil {
		return stored, nil
	}
	if mayEditSheet(m, row) && row.Status != statusDead {
		return s.checkEditedSheet(ctx, m, content, characterID, raw)
	}
	full := stored.GetFull()
	if full == nil || raw.GetFull() == nil {
		return stored, nil
	}
	over := proto.CloneOf(full)
	over.FeatureChoiceKeys = slices.Clone(raw.GetFull().GetFeatureChoiceKeys())
	over.FeatureChoiceText = maps.Clone(raw.GetFull().GetFeatureChoiceText())
	if err := cleanChoiceTexts(over); err != nil {
		return nil, invalidArgument(err)
	}
	if len(over.FeatureChoiceKeys) > rules.MaxListLength {
		return nil, invalidArgument(fieldErr("sheet.full.feature_choice_keys", "must have at most %d entries", rules.MaxListLength))
	}
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: over}}, nil
}

// choicesToProto answers every choice of a build, and where its spell pickers find the
// spells outside the class list.
func choicesToProto(content *rules.Content, b rules.Build) *charactersv1.PreviewChoicesResponse {
	set := content.Choices(b)
	done, total := set.Counts()
	res := &charactersv1.PreviewChoicesResponse{Done: i32(done), Total: i32(total), NotOffered: set.NotOffered}
	for _, g := range set.Groups {
		res.Groups = append(res.Groups, choiceGroupToProto(g))
	}
	for _, src := range content.SpellSources(b) {
		out := &charactersv1.ChoiceSpellSource{
			ClassKey: src.Class, Secrets: i32(src.Secrets), SecretsBeyondKnown: i32(src.SecretsBeyondKnown),
			SecretsMaxSpellLevel: i32(src.SecretsMaxSpellLevel),
		}
		for _, p := range src.Patron {
			out.PatronSpells = append(out.PatronSpells, &charactersv1.ChoicePatronSpell{SpellKey: p.Spell, ClassLevel: i32(p.ClassLevel)})
		}
		res.SpellSources = append(res.SpellSources, out)
	}
	return res
}

var (
	choiceOriginToProto = map[rules.ChoiceOrigin]charactersv1.ChoiceOrigin{
		rules.ChoiceOriginRace:       charactersv1.ChoiceOrigin_CHOICE_ORIGIN_RACE,
		rules.ChoiceOriginClass:      charactersv1.ChoiceOrigin_CHOICE_ORIGIN_CLASS,
		rules.ChoiceOriginSubclass:   charactersv1.ChoiceOrigin_CHOICE_ORIGIN_SUBCLASS,
		rules.ChoiceOriginBackground: charactersv1.ChoiceOrigin_CHOICE_ORIGIN_BACKGROUND,
		rules.ChoiceOriginFeat:       charactersv1.ChoiceOrigin_CHOICE_ORIGIN_FEAT,
	}
	choiceKindToProto = map[rules.ChoiceKind]charactersv1.ChoiceKind{
		rules.ChoiceKindOptions:   charactersv1.ChoiceKind_CHOICE_KIND_OPTIONS,
		rules.ChoiceKindAbilities: charactersv1.ChoiceKind_CHOICE_KIND_ABILITIES,
		rules.ChoiceKindEnemy:     charactersv1.ChoiceKind_CHOICE_KIND_ENEMY,
		rules.ChoiceKindLanguage:  charactersv1.ChoiceKind_CHOICE_KIND_LANGUAGE,
		rules.ChoiceKindSpells:    charactersv1.ChoiceKind_CHOICE_KIND_SPELLS,
	}
	prerequisiteKindToProto = map[string]charactersv1.ChoicePrerequisiteKind{
		rules.PrerequisiteLevel:   charactersv1.ChoicePrerequisiteKind_CHOICE_PREREQUISITE_KIND_LEVEL,
		rules.PrerequisiteSpell:   charactersv1.ChoicePrerequisiteKind_CHOICE_PREREQUISITE_KIND_SPELL,
		rules.PrerequisiteFeature: charactersv1.ChoicePrerequisiteKind_CHOICE_PREREQUISITE_KIND_FEATURE,
		rules.PrerequisiteTaken:   charactersv1.ChoicePrerequisiteKind_CHOICE_PREREQUISITE_KIND_TAKEN,
	}
)

func choiceGroupToProto(g rules.ChoiceGroup) *charactersv1.ChoiceGroup {
	out := &charactersv1.ChoiceGroup{
		Origin: choiceOriginToProto[g.Origin], SourceKey: g.SourceKey, SourceNamePt: g.SourceNamePT, ClassNamePt: g.ClassNamePT, Level: i32(g.Level),
	}
	for _, ch := range g.Choices {
		out.Choices = append(out.Choices, choiceToProto(ch))
	}
	return out
}

func choiceToProto(ch rules.Choice) *charactersv1.Choice {
	out := &charactersv1.Choice{
		Key: ch.Key, FeatureKey: ch.FeatureKey, Kind: choiceKindToProto[ch.Kind],
		TitlePt: ch.TitlePT, PartPt: ch.PartPT, LabelPt: ch.LabelPT, HintPt: ch.HintPT,
		Level: i32(ch.Level), Picks: i32(ch.Picks), Picked: ch.Picked, Texts: ch.Texts, ResultPt: ch.ResultPT, Advisory: ch.Advisory,
		Unmet: ch.Unmet, Overflow: ch.Overflow, Missing: i32(ch.Missing()),
	}
	for _, o := range ch.Options {
		po := &charactersv1.ChoiceOption{
			Key: o.Key, StoredKey: o.Stored, NamePt: o.NamePT, SummaryPt: o.SummaryPT, ReasonPt: o.ReasonPT,
			SpellLevel: i32(o.SpellLevel), SchoolPt: o.SchoolPT, ClassesPt: o.ClassesPT, NeedsText: o.NeedsText,
		}
		for _, p := range o.Prerequisites {
			po.Prerequisites = append(po.Prerequisites, &charactersv1.ChoicePrerequisite{
				Kind: prerequisiteKindToProto[p.Kind], Key: p.Key, Level: i32(p.Level), Met: p.Met, NamePt: p.NamePT, ReasonPt: p.ReasonPT,
			})
		}
		for _, cs := range o.CircleSpells {
			po.CircleSpells = append(po.CircleSpells, &charactersv1.ChoiceCircleSpell{Level: i32(cs.Level), SpellKey: cs.Key, NamePt: cs.NamePT, Reached: cs.Reached})
		}
		out.Options = append(out.Options, po)
	}
	return out
}

// choiceGroupsToProto converts the groups of a level-up offer.
func choiceGroupsToProto(groups []rules.ChoiceGroup) []*charactersv1.ChoiceGroup {
	var out []*charactersv1.ChoiceGroup
	for _, g := range groups {
		out = append(out, choiceGroupToProto(g))
	}
	return out
}

var choiceRefusalMessages = map[charactersv1.ChoiceRefusalReason]string{
	charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICES_MISSING:     "the sheet has choices still to make: complete them first",
	charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_PREREQUISITE_UNMET:  "an option asks for something the sheet does not have",
	charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED:  "a pick is not one the sheet offers",
	charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_ALREADY_MADE: "a choice already made does not change",
	charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OPEN:     "the sheet has no such choice open",
}

// errChoices is the failed_precondition of a pick the rules refuse, with the
// ChoiceRefusal detail that says which.
func errChoices(reason charactersv1.ChoiceRefusalReason, issues ...*charactersv1.ChoiceIssue) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(choiceRefusalMessages[reason]))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.ChoiceRefusal{Reason: reason, Issues: issues}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func choiceIssueOf(p rules.ChoiceProblem, field string) *charactersv1.ChoiceIssue {
	return &charactersv1.ChoiceIssue{
		Field: field, ChoiceKey: p.ChoiceKey, OptionKey: p.OptionKey, Picked: i32(p.Picked), Required: i32(p.Required), LabelPt: p.LabelPT,
	}
}

var choiceProblemReasons = map[string]charactersv1.ChoiceRefusalReason{
	rules.ChoiceProblemPrerequisite: charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_PREREQUISITE_UNMET,
	rules.ChoiceProblemNotOffered:   charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED,
	rules.ChoiceProblemMissing:      charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICES_MISSING,
}

// refuseChoices is what CreateCharacter and UpdateCharacter ask of the choices of a
// player's character saved by its player: nothing left open, and no pick whose
// prerequisite is unmet or that the sheet does not offer. It refuses with the first
// reason in that order and lists every issue of it. An NPC, and the master's edit of
// any sheet, are not refused: the master may leave a choice open. A save of a sheet
// that already had a problem (one made before the choices existed, one the master left
// open, one a claimed character came with) is not refused for that problem, only for a
// new one: before is the stored sheet, nil on a create.
func refuseChoices(content *rules.Content, m authz.Membership, kind string, sheet, before *charactersv1.FullSheet) error {
	if kind != kindPlayer || isMaster(m) || sheet == nil {
		return nil
	}
	problems := content.Choices(buildOf(sheet)).ChoiceProblems(true)
	if before != nil {
		type known struct {
			code   string
			choice string
			option string
		}
		had := map[known]bool{}
		for _, p := range content.Choices(buildOf(before)).ChoiceProblems(true) {
			had[known{p.Code, p.ChoiceKey, p.OptionKey}] = true
		}
		problems = slices.DeleteFunc(problems, func(p rules.ChoiceProblem) bool {
			return had[known{p.Code, p.ChoiceKey, p.OptionKey}]
		})
	}
	if len(problems) == 0 {
		return nil
	}
	first := problems[0].Code
	var issues []*charactersv1.ChoiceIssue
	for _, p := range problems {
		if p.Code == first {
			issues = append(issues, choiceIssueOf(p, "sheet."+p.Field))
		}
	}
	return errChoices(choiceProblemReasons[first], issues...)
}

// errCampaignNotFound is the answer to a player or a stranger asking for what only the
// master reads of a campaign: the same as for a campaign that does not exist.
func errCampaignNotFound() error {
	err := connect.NewError(connect.CodeNotFound, errors.New("campaign not found"))
	err.Meta().Set("Cache-Control", "no-store")
	return err
}

// CompleteCharacterChoices implements charactersv1connect.CharacterServiceHandler.
func (s *Service) CompleteCharacterChoices(
	ctx context.Context,
	req *connect.Request[charactersv1.CompleteCharacterChoicesRequest],
) (*connect.Response[charactersv1.CompleteCharacterChoicesResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	in, err := readChoicesRequest(req.Msg)
	if err != nil {
		return nil, err
	}
	// The content is read before the transaction too, so a campaign whose content
	// cannot be read answers before any row is locked; the transaction reads its own.
	if _, err := s.contentFor(ctx, nil, m.CampaignID); err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var done choicesCompleted
	// Under the funnel: the session's row first (the history line it appends takes it too), then the character's, and the write
	// moves the session's number once.
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		done, err = s.completeChoicesInTx(ctx, tx, m, in)
		if err == nil {
			t.XPChanged()
		}
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "complete the choices of a character", err)
	}
	logging.Event(ctx, s.logger, "character.choices_completed", slog.String("character_id", in.id), slog.Int("choices", len(done.labels)))
	c, err := s.character(ctx, done.content, done.row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a character with its choices made", err)
	}
	return connect.NewResponse(&charactersv1.CompleteCharacterChoicesResponse{Character: c}), nil
}

// choicesRequest is a CompleteCharacterChoices request whose shape is checked: the
// character's canonical id, the revision the caller read and the cleaned picks.
type choicesRequest struct {
	id       string
	revision int32
	picks    []*charactersv1.ChoicePick
}

// choicesCompleted is what the transaction of CompleteCharacterChoices leaves: the row
// written, the content it was checked with and the labels of the choices answered.
type choicesCompleted struct {
	row     charactersdb.Character
	content *rules.Content
	labels  []string
}

// readChoicesRequest checks what needs no database, in order: the id (an id that is no
// UUID names nothing), the revision and the shape of the picks.
func readChoicesRequest(msg *charactersv1.CompleteCharacterChoicesRequest) (choicesRequest, error) {
	id, ok := uid.Canonical(msg.GetCharacterId())
	if !ok {
		return choicesRequest{}, errCharacterNotFound()
	}
	revision := msg.GetExpectedRevision()
	if revision < 1 {
		return choicesRequest{}, invalidArgument(fieldErr("expected_revision", "must be at least 1"))
	}
	picks, err := cleanChoicePicks(msg.GetPicks())
	if err != nil {
		return choicesRequest{}, invalidArgument(err)
	}
	return choicesRequest{id: id, revision: revision, picks: picks}, nil
}

// completeChoicesInTx is one attempt of the transaction: it reads and locks the
// character, lays the picks over its sheet, writes it, and has the hit points and the
// master's history follow.
func (s *Service) completeChoicesInTx(ctx context.Context, tx pgx.Tx, m authz.Membership, in choicesRequest) (choicesCompleted, error) {
	q := s.queries.WithTx(tx)
	current, err := visibleForUpdate(ctx, q, m, in.id)
	if err != nil {
		return choicesCompleted{}, err
	}
	if current.Status == statusDead {
		return choicesCompleted{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, current.ID)
	}
	if current.Revision != in.revision {
		return choicesCompleted{}, errStaleRevision()
	}
	content, err := s.contentFor(ctx, tx, m.CampaignID)
	if err != nil {
		return choicesCompleted{}, wrap("read rules content", err)
	}
	sheet, labels, err := pickedSheet(content, current, in.picks)
	if err != nil {
		return choicesCompleted{}, err
	}
	doc, err := storeJSON.Marshal(sheet)
	if err != nil {
		return choicesCompleted{}, wrap("encode a sheet", err)
	}
	row, err := q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: m.CampaignID, ID: in.id, Revision: in.revision, Name: current.Name, Sheet: doc, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return choicesCompleted{}, errStaleRevision()
	}
	if err != nil {
		return choicesCompleted{}, wrap("update sheet", err)
	}
	// A pick can change the hit points (a +1 in Constitution): the current ones follow.
	if err := s.carryHitPoints(ctx, q, content, in.id, current.Sheet, doc); err != nil {
		return choicesCompleted{}, err
	}
	if err := s.logChoicesCompleted(ctx, tx, m, in.id, labels); err != nil {
		return choicesCompleted{}, err
	}
	return choicesCompleted{row: row, content: content, labels: labels}, nil
}

// pickedSheet is the stored sheet with the picks laid over it, checked as any sheet
// is, and the labels of the choices answered.
func pickedSheet(content *rules.Content, current charactersdb.Character, picks []*charactersv1.ChoicePick) (*charactersv1.CharacterSheet, []string, error) {
	stored, err := loadSheet(current.ID, current.Sheet)
	if err != nil {
		return nil, nil, err
	}
	full := stored.GetFull()
	if full == nil {
		return nil, nil, invalidArgument(fieldErr("character_id", "is a character with a basic sheet, which has no choices"))
	}
	next, labels, err := applyChoicePicks(content, full, picks)
	if err != nil {
		return nil, nil, err
	}
	sheet, err := checkSheet(content, &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: next}})
	if err != nil {
		return nil, nil, invalidArgument(err)
	}
	// The picks may not make the sheet worse: a prerequisite they unmeet, or a pick
	// the sheet does not offer, that it did not have before.
	if err := refuseNewChoiceProblems(content, full, sheet.GetFull()); err != nil {
		return nil, nil, err
	}
	carryFlags(content, full, sheet.GetFull())
	return sheet, labels, nil
}

// cleanChoicePicks checks the shape of the picks and trims their texts.
func cleanChoicePicks(picks []*charactersv1.ChoicePick) ([]*charactersv1.ChoicePick, error) {
	if len(picks) == 0 {
		return nil, fieldErr("picks", "is required")
	}
	if len(picks) > maxChoicePicks {
		return nil, fieldErr("picks", "must have at most %d entries", maxChoicePicks)
	}
	out := make([]*charactersv1.ChoicePick, 0, len(picks))
	for _, p := range picks {
		clean, err := cleanChoicePick(p)
		if err != nil {
			return nil, err
		}
		out = append(out, clean)
	}
	return out, nil
}

// cleanChoicePick checks the shape of one pick and returns a copy with its texts
// trimmed and the repeats of an option next to each other dropped.
func cleanChoicePick(p *charactersv1.ChoicePick) (*charactersv1.ChoicePick, error) {
	if p.GetChoiceKey() == "" || len(p.GetChoiceKey()) > maxChoiceKeyLength {
		return nil, fieldErr("picks.choice_key", "must be 1 to %d characters", maxChoiceKeyLength)
	}
	if len(p.GetOptionKeys()) > rules.MaxListLength || len(p.GetTexts()) > rules.MaxChoiceTexts {
		return nil, fieldErr("picks", "has too many option keys or texts")
	}
	clean := &charactersv1.ChoicePick{ChoiceKey: p.GetChoiceKey(), OptionKeys: slices.Compact(slices.Clone(p.GetOptionKeys()))}
	for _, text := range p.GetTexts() {
		t, err := cleanChoiceText(text)
		if err != nil {
			return nil, err
		}
		clean.Texts = append(clean.Texts, t)
	}
	return clean, nil
}

// cleanChoiceText trims a text of a pick. A blank one is not written yet, so it stays
// empty and the choice stays open.
func cleanChoiceText(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	t, err := names.Clean(strings.TrimSpace(text), rules.MaxChoiceTextLength)
	if err != nil {
		return "", &fieldError{field: "picks.texts", err: err}
	}
	return t, nil
}

// applyChoicePicks lays the picks over a stored sheet: only the choices that are open,
// only options the choice offers and the sheet can take, and nothing already made
// changes. It returns the new sheet (a copy) and the labels of the choices answered.
// Every pick is checked against what the stored sheet offers.
func applyChoicePicks(content *rules.Content, full *charactersv1.FullSheet, picks []*charactersv1.ChoicePick) (*charactersv1.FullSheet, []string, error) {
	set := content.Choices(buildOf(full))
	next := proto.CloneOf(full)
	texts := maps.Clone(full.GetFeatureChoiceText())
	if texts == nil {
		texts = map[string]string{}
	}
	var labels []string
	seen := map[string]bool{}
	for _, p := range picks {
		if seen[p.GetChoiceKey()] {
			return nil, nil, invalidArgument(fieldErr("picks.choice_key", "repeats a choice"))
		}
		seen[p.GetChoiceKey()] = true
		choice, fresh, err := applyChoicePick(set, p, texts)
		if err != nil {
			return nil, nil, err
		}
		next.FeatureChoiceKeys = append(next.FeatureChoiceKeys, fresh...)
		labels = append(labels, choice.LabelPT)
	}
	next.FeatureChoiceText = texts
	return next, labels, nil
}

// applyChoicePick checks one pick against its open choice and returns the choice and
// the stored keys of the options it adds. The texts of an option that takes them go
// into texts.
func applyChoicePick(set rules.ChoiceSet, p *charactersv1.ChoicePick, texts map[string]string) (rules.Choice, []string, error) {
	choice, ok := findChoice(set, p.GetChoiceKey())
	if !ok {
		return rules.Choice{}, nil, errChoices(charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OPEN, &charactersv1.ChoiceIssue{Field: "picks.choice_key", ChoiceKey: p.GetChoiceKey()})
	}
	issue := &charactersv1.ChoiceIssue{Field: "picks", ChoiceKey: choice.Key, Picked: i32(choice.Done()), Required: i32(choice.Picks), LabelPt: choice.LabelPT}
	if choice.Missing() == 0 {
		return rules.Choice{}, nil, errChoices(charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_ALREADY_MADE, issue)
	}
	fresh, takesText, err := freshChoiceOptions(choice, p.GetOptionKeys(), issue)
	if err != nil {
		return rules.Choice{}, nil, err
	}
	if len(fresh) > choice.Missing() {
		return rules.Choice{}, nil, errChoices(charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED, issue)
	}
	if len(fresh) == 0 && !takesText {
		return rules.Choice{}, nil, errChoices(charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICES_MISSING, issue)
	}
	if takesText {
		if err := writeChoiceTexts(choice, p.GetTexts(), texts, issue); err != nil {
			return rules.Choice{}, nil, err
		}
	}
	return choice, fresh, nil
}

// freshChoiceOptions walks the options of a pick in order: an option the choice does
// not list, or one the sheet cannot take, refuses the pick (issue names it); one made
// already stays as it is. It returns the stored keys of the new ones, and whether an
// option of the pick takes texts.
func freshChoiceOptions(choice rules.Choice, keys []string, issue *charactersv1.ChoiceIssue) ([]string, bool, error) {
	var fresh []string
	takesText := false
	for _, key := range keys {
		opt, found := choiceOption(choice, key)
		if !found {
			issue.OptionKey = key
			return nil, false, errChoices(charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICE_NOT_OFFERED, issue)
		}
		takesText = takesText || opt.NeedsText
		if slices.Contains(choice.Picked, key) {
			continue // made already: it stays as it is
		}
		if opt.Blocked() {
			issue.OptionKey = key
			return nil, false, errChoices(charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_PREREQUISITE_UNMET, issue)
		}
		fresh = append(fresh, opt.Stored)
	}
	return fresh, takesText, nil
}

// writeChoiceTexts writes the texts of a pick whose option takes them: the humanoid
// favored enemy names two races, both written.
func writeChoiceTexts(choice rules.Choice, picked []string, texts map[string]string, issue *charactersv1.ChoiceIssue) error {
	if len(picked) != len(choiceTextSlots) || slices.Contains(picked, "") {
		return errChoices(charactersv1.ChoiceRefusalReason_CHOICE_REFUSAL_REASON_CHOICES_MISSING, issue)
	}
	for i, t := range picked {
		texts[rules.ChoiceTextKey(choice.Key, i+1)] = t
	}
	return nil
}

// choiceTextSlots is the number of races the humanoid favored enemy names.
var choiceTextSlots = make([]struct{}, 2)

func findChoice(set rules.ChoiceSet, key string) (rules.Choice, bool) {
	for _, g := range set.Groups {
		for _, ch := range g.Choices {
			if ch.Key == key {
				return ch, true
			}
		}
	}
	return rules.Choice{}, false
}

func choiceOption(ch rules.Choice, key string) (rules.ChoiceOption, bool) {
	for _, o := range ch.Options {
		if o.Key == key {
			return o, true
		}
	}
	return rules.ChoiceOption{}, false
}

// refuseNewChoiceProblems refuses a sheet whose picks break a rule the stored one did
// not break: an unmet prerequisite, a pick the sheet does not offer.
func refuseNewChoiceProblems(content *rules.Content, before, after *charactersv1.FullSheet) error {
	had := map[string]bool{}
	for _, p := range content.Choices(buildOf(before)).ChoiceProblems(false) {
		had[p.Code+"|"+p.ChoiceKey+"|"+p.OptionKey] = true
	}
	for _, p := range content.Choices(buildOf(after)).ChoiceProblems(false) {
		if !had[p.Code+"|"+p.ChoiceKey+"|"+p.OptionKey] {
			return errChoices(choiceProblemReasons[p.Code], choiceIssueOf(p, "sheet."+p.Field))
		}
	}
	return nil
}

// choicesCompletedPayload is what the session event keeps: IDs and content keys only,
// never a name (docs/privacy.md).
type choicesCompletedPayload struct {
	CharacterID string `json:"character_id"`
	Count       int    `json:"count"`
}

// logChoicesCompleted tells the master, in the history of the open session, that a
// character completed choices of its sheet. No session open, no line.
func (s *Service) logChoicesCompleted(ctx context.Context, tx pgx.Tx, m authz.Membership, characterID string, labels []string) error {
	if s.creatureHost == nil {
		return nil
	}
	body, err := json.Marshal(choicesCompletedPayload{CharacterID: characterID, Count: len(labels)})
	if err != nil {
		return wrap("encode the event payload", err)
	}
	if _, err := s.creatureHost.AppendEvent(ctx, tx, m.CampaignID, eventCharacterChoicesCompleted, m.UserID, body, s.now()); err != nil {
		return wrap("append the choices event", err)
	}
	return nil
}

// GetCampaignOpenChoices implements charactersv1connect.CharacterServiceHandler.
func (s *Service) GetCampaignOpenChoices(
	ctx context.Context,
	req *connect.Request[charactersv1.GetCampaignOpenChoicesRequest],
) (*connect.Response[charactersv1.GetCampaignOpenChoicesResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if !isMaster(m) {
		return nil, errCampaignNotFound()
	}
	rows, err := s.queries.ListCharacters(ctx, charactersdb.ListCharactersParams{CampaignID: m.CampaignID})
	if err != nil {
		return nil, s.dbError(ctx, "list characters", err)
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	res := &charactersv1.GetCampaignOpenChoicesResponse{}
	for _, row := range rows {
		open, err := openChoicesOf(content, row)
		if err != nil {
			return nil, s.dbError(ctx, "read the open choices", err)
		}
		if open != nil {
			res.Characters = append(res.Characters, open)
		}
	}
	return connect.NewResponse(res), nil
}

// openChoicesOf is a character's line in the master's list of open choices: nil for
// one that is not a living player character with a full sheet and something open.
func openChoicesOf(content *rules.Content, row charactersdb.ListCharactersRow) (*charactersv1.OpenChoicesCharacter, error) {
	if row.Kind != kindPlayer || row.Status != statusActive {
		return nil, nil
	}
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, err
	}
	full := sheet.GetFull()
	if full == nil {
		return nil, nil
	}
	pending := content.Choices(buildOf(full)).Pending()
	if len(pending) == 0 {
		return nil, nil
	}
	open := &charactersv1.OpenChoicesCharacter{CharacterId: row.ID, Name: row.Name}
	for _, p := range pending {
		open.PendingCount += i32(p.Missing)
		open.LabelsPt = append(open.LabelsPt, p.LabelPT)
	}
	return open, nil
}

// pendingChoiceCount is the count CharacterSummary.pending_choice_count shows, for the
// master and the owning player only: how many selections of a living player character
// are open. Nil for every other character, whose count is not for the caller to know
// (RN-10).
func pendingChoiceCount(content *rules.Content, m authz.Membership, kind, status string, playerUserID *string, sheet *charactersv1.CharacterSheet) *int32 {
	owner := playerUserID != nil && *playerUserID == m.UserID
	if kind != kindPlayer || status == statusDead || sheet.GetFull() == nil || (!isMaster(m) && !owner) {
		return nil
	}
	n := i32(content.Choices(buildOf(sheet.GetFull())).PendingCount())
	return &n
}
