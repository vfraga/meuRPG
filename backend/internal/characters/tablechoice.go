package characters

import (
	"context"
	"slices"
	"time"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What the table's own content means for a sheet (MR-025, RN-23, ADR-0018).

// newArchivedChoice finds a table entry that the master archived and that after
// picks but before did not: a retired entry is never a new choice, while a sheet
// that already has it keeps it (the key is in before, so it is not new). before is
// nil for a sheet that does not exist yet. It returns the first one, in the
// sheet's order, and the sheet field that holds it.
func newArchivedChoice(content *rules.Content, before, after *charactersv1.FullSheet) (key, field string, found bool) {
	archived := content.ArchivedKeys(buildOf(after))
	if len(archived) == 0 {
		return "", "", false
	}
	already := content.ArchivedKeys(buildOf(before))
	for _, kf := range rules.BuildKeys(buildOf(after)) {
		if slices.Contains(archived, kf.Key) && !slices.Contains(already, kf.Key) {
			return kf.Key, kf.Field, true
		}
	}
	return "", "", false
}

// newOffChoice is newArchivedChoice for the master's switches ("Opções para os
// jogadores", RN-23): the first option the master switched off (or whose class or
// race is off) that after picks and before did not. The caller asks it of a player
// only: the master's own characters may use what the players may not.
func newOffChoice(content *rules.Content, before, after *charactersv1.FullSheet) (key, field string, found bool) {
	off := content.OffKeys(buildOf(after))
	if len(off) == 0 {
		return "", "", false
	}
	already := content.OffKeys(buildOf(before))
	for _, kf := range rules.BuildKeys(buildOf(after)) {
		if slices.Contains(off, kf.Key) && !slices.Contains(already, kf.Key) {
			return kf.Key, kf.Field, true
		}
	}
	return "", "", false
}

// errOffChoice is the refusal of CreateCharacter and UpdateCharacter for a newly
// chosen option the master switched off.
func errOffChoice(characterID, key string) error {
	return errBlockedByContent(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT, characterID, key)
}

// errArchivedChoice is the refusal of CreateCharacter and UpdateCharacter for a
// newly chosen archived key. characterID is empty for a character that does not
// exist yet.
func errArchivedChoice(characterID, key string) error {
	return errBlockedByContent(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT, characterID, key)
}

// CheckSheetCanMove refuses a sheet that uses the table's own content ("...@mesa"
// keys) for any copy or move to another campaign: the content belongs to the
// campaign it was written for (ADR-0018, section 10), and the other has no such
// keys. It answers `failed_precondition` with the CharacterBlocked detail
// TABLE_CONTENT_STAYS and the first key, or nil. Nothing calls it yet: the copy of
// a character (MR-021) and the reuse of NPCs (MR-022) must, before they write.
func CheckSheetCanMove(sheet *charactersv1.CharacterSheet) error {
	full := sheet.GetFull()
	if full == nil {
		return nil // a basic sheet has no content keys
	}
	if keys := rules.TableKeys(buildOf(full)); len(keys) > 0 {
		return errBlockedByContent(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_TABLE_CONTENT_STAYS, "", keys[0])
	}
	return nil
}

// changedKinds says what changed, for the issue's text: "A classe mudou".
var changedKinds = map[string]string{
	"class": "A classe", "subclass": "A subclasse", "race": "A raça", "subrace": "A sub-raça",
	"background": "O antecedente", "spell": "A magia", "feat": "O talento", "option": "A opção", "classfeature": "A característica", "weapon": "A arma",
}

// IssueTableContentChanged is the code of the issue "A classe mudou" (and its
// kin), on the derived sheet of a sheet that uses a table entry which changed
// after the sheet was last saved and that has issues it did not have then.
const IssueTableContentChanged = "table_content_changed"

// issueID is an issue by its whole identity (rules.Issue.ID): the code, the field and the exact
// sentence ("Faltam 1 perícias" that becomes "Há 3 perícias; escolhe 2" is another
// issue, and two formula issues on different features do not collide); for a lost prerequisite
// the code, the list its pick is in and the entry's key.
func issueID(is rules.Issue) string { return is.ID() }

// issueIDs is a set of issues as the sheet stores it: sorted, no repeats.
func issueIDs(issues []rules.Issue) []string {
	var out []string
	for _, is := range issues {
		out = append(out, issueID(is))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// flag is a changed table entry a sheet uses, with the issues of the sheet that
// are new since it was last saved and depend on that entry.
type flag struct {
	entry  rules.ChangedEntry
	issues []rules.Issue
}

// flags works out, for a sheet and the issues Derive found on it now, which changed
// entries it is flagged for (RN-23): an entry the sheet uses that
// changed after the sheet was last known to fit it (full.ContentRevision, or the
// entry's own baseline), and a new issue that depends on that entry (Issue.Keys).
// Nothing else is blamed on the entry: a text edit that left no issue about it shows
// nothing, and another entry's issue is never attached to it.
func flags(content *rules.Content, full *charactersv1.FullSheet, build rules.Build, issues []rules.Issue) []flag {
	baselines := map[string]int{}
	for k, r := range full.GetContentBaselines() {
		baselines[k] = int(r)
	}
	changed := content.ChangedSince(build, int(full.GetContentRevision()), baselines)
	if len(changed) == 0 {
		return nil
	}
	known := rules.KnownIssues(full.GetKnownIssues(), issues)
	var out []flag
	for _, ch := range changed {
		f := flag{entry: ch}
		for i, is := range issues {
			if slices.Contains(is.Keys, ch.Key) && !known[i] && told(content, is, ch.Key, ch.Since) {
				f.issues = append(f.issues, is)
			}
		}
		if len(f.issues) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// told says the issue is one to tell for the changed table entry key, which the sheet fits as of revision since. All are but
// the notice that a weapon leaves the shield idle: that one only when the weapon became two-handed after since, so a sheet
// that already had the weapon as it is (saved before the notice existed, with no identity stored) is told nothing when the
// weapon is edited for anything else.
func told(content *rules.Content, is rules.Issue, key string, since int) bool {
	return is.Idle == nil || content.TwoHandedSince(key) > since
}

// sinceOf is the revision of the table's content the sheet fits key as of: the baseline it keeps for the entry, or the
// revision it was saved at.
func sinceOf(full *charactersv1.FullSheet, key string) int {
	if r, ok := full.GetContentBaselines()[key]; ok {
		return int(r)
	}
	return int(full.GetContentRevision())
}

// issuesTiedTo are the new issues of a sheet (not in its known issues) that depend
// on a table entry: what a change of the entry just did to it, for the master's
// "Fichas com aviso".
func issuesTiedTo(content *rules.Content, full *charactersv1.FullSheet, key string) []rules.Issue {
	var out []rules.Issue
	issues := rules.Derive(buildOf(full), content).AllIssues()
	known := rules.KnownIssues(full.GetKnownIssues(), issues)
	for i, is := range issues {
		if slices.Contains(is.Keys, key) && !known[i] && told(content, is, key, sinceOf(full, key)) {
			out = append(out, is)
		}
	}
	return out
}

// carryFlags keeps a save from clearing what is still wrong (state 8: the
// warning goes away when the numbers match again, never through a save). next is
// the sheet about to be saved (checkSheet has set its revision and its known
// issues from everything now); stored is the sheet it replaces. The issues stored
// was flagged for, and that next still has, are taken out of next's known issues,
// and the entries they are tied to keep the baseline stored had for them, so the
// next read flags them again. Fixing what the issue says is what clears it.
func carryFlags(content *rules.Content, stored, next *charactersv1.FullSheet) {
	if stored == nil || next == nil {
		return
	}
	nextBuild := buildOf(next)
	issues := rules.Derive(nextBuild, content).AllIssues()
	// The flags are worked out against what stored knew, with next's own build and issues.
	fl := flags(content, stored, nextBuild, issues)
	if len(fl) == 0 {
		next.ContentBaselines = nil
		return
	}
	drop := map[string]bool{}
	baselines := map[string]int32{}
	for _, f := range fl {
		baselines[f.entry.Key] = i32(f.entry.Since)
		for _, is := range f.issues {
			drop[issueID(is)] = true
		}
	}
	next.KnownIssues = slices.DeleteFunc(next.KnownIssues, func(id string) bool { return drop[id] })
	next.ContentBaselines = baselines
}

// addChangedContent tells the sheet's owner that a table entry it uses changed
// and no longer fits (RN-23). The numbers already use
// the new rules; this is the notice. It is worked out on every read from the
// sheet's own issues (flags): it shows only for an entry that changed and that has
// a new issue tied to it, with the sentences of exactly those issues, and goes
// away by itself when none remains. A save does not clear it either (carryFlags).
// It is never a Validate error, so it blocks nothing. issues are what Derive found
// (with their keys); d is the same, as the message.
func addChangedContent(d *rulesv1.DerivedSheet, issues []rules.Issue, content *rules.Content, build rules.Build, full *charactersv1.FullSheet, r reader) {
	for _, f := range flags(content, full, build, issues) {
		name := content.NamePT(f.entry.Key)
		var messages, codes []string
		for _, is := range f.issues {
			codes = append(codes, is.Code)
			// The sentence as a change reads it ("agora dá 2 perícias"), when the
			// engine has one for this issue; otherwise the issue's own.
			if is.Lost != nil || is.Idle != nil {
				messages = append(messages, r.say(is))
				continue
			}
			messages = append(messages, changeSentence(is, f.entry.Key, name))
		}
		d.ChangedContent = append(d.ChangedContent, &rulesv1.ChangedContent{
			Key: f.entry.Key, NamePt: name, Field: f.entry.Field, Revision: i32(f.entry.Revision), SavedRevision: i32(f.entry.Since),
			ChangedAt: timestampOf(f.entry.ChangedAt), Messages: messages, Codes: codes,
		})
		d.Issues = append(d.Issues, &rulesv1.Issue{
			Code: IssueTableContentChanged, Field: f.entry.Field,
			Message: changedKinds[keyKind(f.entry.Key)] + " " + name + " mudou: a ficha ficou com avisos que não tinha. Os números já usam as regras novas.",
		})
	}
}

// reader is who reads a sheet's issues: the sentence of a lost prerequisite names the character and, for the
// campaign's master alone, a condition on an entry the players may not read (board 580-05, RN-23). It is built on
// each read; the sheet stores no sentence of it, only the issue's identity (known issues).
type reader struct {
	name   string
	master bool
}

// say is the issue's sentence for the reader: a lost prerequisite and a notice are built for the reader, the master reads
// an issue's named sentence, any other is as stored.
func (r reader) say(is rules.Issue) string {
	switch {
	case is.Lost != nil:
		return is.Lost.SentencePT(r.name, r.master)
	case is.Idle != nil:
		return is.Idle.SentencePT(r.name)
	case is.Named != "" && r.master:
		return is.Named
	}
	return is.Message
}

// tell rewrites, in the sheet's derived issues (made from issues, in its order), the message of each lost
// prerequisite to the reader's sentence. issues are Derived.Issues alone: the notices (Derived.Notices) are no part of
// the derived sheet's issues, so the two lists stay aligned.
func (r reader) tell(d *rulesv1.DerivedSheet, issues []rules.Issue) {
	for i, is := range issues {
		if (is.Lost != nil || is.Named != "") && i < len(d.GetIssues()) {
			d.Issues[i].Message = r.say(is)
		}
	}
}

// derived is the DerivedSheet of d as the reader gets it: each lost prerequisite in the reader's sentence.
func (r reader) derived(d rules.Derived) *rulesv1.DerivedSheet {
	out := derivedToProto(d)
	r.tell(out, d.Issues)
	return out
}

// changeSentence is how "A classe mudou" tells an issue: the sentence of the
// change, naming the class or subclass it is about only when that is the entry that
// changed ("Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3."),
// and without a name for any other (an SRD class's number changed by a race's
// trait is never blamed on the SRD class). An issue with no change sentence is told
// with its own.
func changeSentence(is rules.Issue, changed, name string) string {
	if is.ChangeMessage == "" {
		return is.Message
	}
	if is.ChangeSubject != "" && is.ChangeSubject == changed {
		return name + " " + is.ChangeMessage
	}
	r, size := utf8.DecodeRuneInString(is.ChangeMessage)
	return string(unicode.ToUpper(r)) + is.ChangeMessage[size:]
}

func timestampOf(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// callerKeys are the content keys (the SRD's and the table's) the caller's own
// characters in the campaign use, read with q (a transaction's, or the pool's
// outside one): what lets a player keep reading about an option the master archived
// or switched off after their sheet took it. The content says which table class features the sheets have.
func callerKeys(ctx context.Context, q *charactersdb.Queries, m authz.Membership, content *rules.Content) (map[string]bool, error) {
	sheets, err := q.ListCampaignSheets(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, row := range sheets {
		if conv.OrZero(row.PlayerUserID) != m.UserID {
			continue
		}
		sheet, err := loadSheet(row.ID, row.Sheet)
		if err != nil {
			return nil, err
		}
		for _, k := range sheetContentKeys(content, sheet) {
			out[k] = true
		}
	}
	return out, nil
}

// sheetContentKeys are the content keys a sheet uses. A table class feature is no key of the build: the sheet has it
// through its class and level, unless the master switched it off, and then the sheet no longer has it.
func sheetContentKeys(content *rules.Content, sheet *charactersv1.CharacterSheet) []string {
	full := sheet.GetFull()
	if full == nil {
		return nil
	}
	b := buildOf(full)
	var keys []string
	for _, kf := range rules.BuildKeys(b) {
		keys = append(keys, kf.Key)
	}
	for _, kf := range content.ClassFeatureKeys(b) {
		if !content.Off(kf.Key) {
			keys = append(keys, kf.Key)
		}
	}
	return keys
}

// callerUses says whether one of the caller's own characters uses the key.
func (s *Service) callerUses(ctx context.Context, m authz.Membership, content *rules.Content, key string) (bool, error) {
	keys, err := callerKeys(ctx, s.queries, m, content)
	return keys[key], err
}
