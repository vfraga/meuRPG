package characters

import (
	"slices"
	"strings"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// leftOut is a stored entry that the content was built without.
// It carries every violation the engine found in it.
type leftOut struct {
	Key        string
	Kind       rulesv1.TableContentKind
	Violations []*rulesv1.TableContentViolation
	// Archived says the master retired the entry: it is still left out, but nobody has to fix it.
	Archived bool
	// Parent is the key of the class or race the entry depends on, "" for an entry that stands alone.
	Parent string
}

// buildTolerant is srd.With the overlay of entries, built without the entries the engine refuses.
// The entries that depend on a refused one are left out too.
// An error that names no entry, or names an entry in strict, is returned as it is.
// Each pass leaves out at least one live entry or returns, so the loop ends after at most len(entries)+1 builds.
// The engine stops at the first entry it refuses, so a pass cannot name more than one culprit.
func buildTolerant(srd *rules.Content, entries []entryRow, revision int, off []string, strict ...string) (*rules.Content, []leftOut, error) {
	content, out, _, err := buildCounting(srd, entries, revision, off, strict)
	return content, out, err
}

// buildCounting is buildTolerant that also says how many builds it took.
func buildCounting(srd *rules.Content, entries []entryRow, revision int, off, strict []string) (*rules.Content, []leftOut, int, error) {
	var out []leftOut
	live := entries
	for builds := 1; ; builds++ {
		overlay, idx := overlayFromEntries(live, revision)
		overlay.Off = off
		content, err := srd.With(overlay)
		if err == nil {
			return content.WithRefused(refusedOf(out)), out, builds, nil
		}
		blamed, ok := blameLive(violationsOf(err, idx, ""), live, strict)
		if !ok {
			return nil, nil, builds, err
		}
		out = append(out, blamed...)
		live = withoutKeys(live, blamed)
	}
}

// blameLive groups the violations by the live entry they name.
// It reports false when one names no entry, an entry that is not live or one that must stay.
func blameLive(violations []*rulesv1.TableContentViolation, live []entryRow, strict []string) ([]leftOut, bool) {
	var out []leftOut
	for _, v := range violations {
		i := slices.IndexFunc(live, func(e entryRow) bool { return e.row.ContentKey == v.GetKey() })
		if v.GetKey() == "" || i < 0 || slices.Contains(strict, v.GetKey()) {
			return nil, false
		}
		j := slices.IndexFunc(out, func(l leftOut) bool { return l.Key == v.GetKey() })
		if j < 0 {
			out = append(out, leftOut{Key: v.GetKey(), Kind: live[i].kind, Archived: live[i].row.ArchivedAt != nil, Parent: parentOf(live[i])})
			j = len(out) - 1
		}
		out[j].Violations = append(out[j].Violations, v)
	}
	return out, len(out) > 0
}

func withoutKeys(entries []entryRow, out []leftOut) []entryRow {
	return slices.DeleteFunc(slices.Clone(entries), func(e entryRow) bool {
		return slices.ContainsFunc(out, func(l leftOut) bool { return l.Key == e.row.ContentKey })
	})
}

// parentOf is the key of the class or race a subclass, a class feature or a subrace depends on.
func parentOf(e entryRow) string {
	switch b := e.body.(type) {
	case *rulesv1.TableSubclass:
		return b.GetClassKey()
	case *rulesv1.TableClassFeature:
		return b.GetClassKey()
	case *rulesv1.TableSubrace:
		return b.GetRaceKey()
	}
	return ""
}

// unfinished is the entries of out that the master still has to fix.
// An archived entry is retired, so it is not one, and neither is what is out only because its parent is: a child whose
// violation is the dangling reference to its retired parent, as deep as the chain goes.
// A live child with a violation of its own (not the missing parent) is still the master's to fix, so it stays.
func unfinished(out []leftOut) []leftOut {
	retired := map[string]bool{}
	for _, l := range out {
		if l.Archived {
			retired[l.Key] = true
		}
	}
	for grew := true; grew; { // a dependent of a dependent, as deep as the chain goes
		grew = false
		for _, l := range out {
			if !retired[l.Key] && l.Parent != "" && retired[l.Parent] && onlyTheParent(l) {
				retired[l.Key], grew = true, true
			}
		}
	}
	return slices.DeleteFunc(slices.Clone(out), func(l leftOut) bool { return retired[l.Key] })
}

// onlyTheParent says the entry is out for the reference to its parent and nothing else.
func onlyTheParent(l leftOut) bool {
	return len(l.Violations) > 0 && l.Violations[0].GetReason() == "dangling_reference"
}

// refusedOf is what the content reports of the entries left out that the master has to fix.
// It keeps the first violation of each.
func refusedOf(out []leftOut) []rules.Refused {
	out = unfinished(out)
	if len(out) == 0 {
		return nil
	}
	r := make([]rules.Refused, 0, len(out))
	for _, l := range out {
		v := l.Violations[0]
		r = append(r, rules.Refused{Key: l.Key, Field: v.GetField(), Reason: v.GetReason(), Message: v.GetMessage()})
	}
	slices.SortFunc(r, func(a, b rules.Refused) int { return strings.Compare(a.Key, b.Key) })
	return r
}

// decodeTolerant decodes the stored entries and leaves out the rows that cannot be read.
func decodeTolerant(rows []charactersdb.CampaignContent) ([]entryRow, []leftOut) {
	entries := make([]entryRow, 0, len(rows))
	var out []leftOut
	for _, r := range rows {
		e, err := decodeRow(r)
		if err != nil {
			out = append(out, leftOut{Key: r.ContentKey, Kind: kindOfPrefix(r.Kind), Archived: r.ArchivedAt != nil, Violations: []*rulesv1.TableContentViolation{{
				Key: r.ContentKey, Field: "data", Reason: rules.ReasonValue, Message: "the entry could not be read",
			}}})
			continue
		}
		entries = append(entries, e)
	}
	return entries, out
}

// buildFromRows is buildTolerant over stored rows.
// A row that cannot be read is left out too.
func buildFromRows(srd *rules.Content, rows []charactersdb.CampaignContent, revision int, off []string) (*rules.Content, error) {
	entries, unreadable := decodeTolerant(rows)
	content, out, err := buildTolerant(srd, entries, revision, off)
	if err != nil {
		return nil, err
	}
	if len(unreadable) == 0 {
		return content, nil
	}
	return content.WithRefused(refusedOf(append(unreadable, out...))), nil
}

// onlyThisEntry says whether every violation is in the entry being written.
func onlyThisEntry(violations []*rulesv1.TableContentViolation) bool {
	return !slices.ContainsFunc(violations, func(v *rulesv1.TableContentViolation) bool {
		return v.GetKey() != "" || !strings.HasPrefix(v.GetField(), "table_")
	})
}

// withoutStoredLeftOut is after without the entries the stored rows already leave out.
// The entry key is never dropped.
// It reports false when there is none to drop.
func withoutStoredLeftOut(srd *rules.Content, stored, after []entryRow, revision int, key string) ([]entryRow, bool) {
	if len(stored) == 0 {
		return nil, false
	}
	_, out, err := buildTolerant(srd, stored, revision, nil)
	if err != nil {
		return nil, false
	}
	out = slices.DeleteFunc(out, func(l leftOut) bool { return l.Key == key })
	if len(out) == 0 {
		return nil, false
	}
	return withoutKeys(after, out), true
}
