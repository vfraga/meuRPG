package play

import (
	"testing"

	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Guard for the hints of a retried AddLastingEffect: the retry tells the streams again, and its
// log hint is secret when the first call's line was (an effect the players cannot see), so a retry
// does not tell the players there is a new line they cannot read. Besides the combat and its log, the retry
// of any combat change tells what the stored change does not say it did not tell (the replay rule of
// Service.tx): the master's casts and the vitals of the combat's players, each to the master and its own
// player. None of it is the secret line.
func TestAddLastingEffectRetryKeepsTheLogHintSecret(t *testing.T) {
	t.Parallel()
	const (
		bothHeard   = "master[EncounterChanged,CombatLogChanged] caio[EncounterChanged,CombatLogChanged] ana[EncounterChanged,CombatLogChanged] bia[EncounterChanged,CombatLogChanged]"
		masterAlone = "master[EncounterChanged,CombatLogChanged] caio[EncounterChanged] ana[EncounterChanged] bia[EncounterChanged]"
		// The retry: the replayed hints come first, the combat and its log after, as the handler tells them.
		bothAgain        = "master[SpellCastsChanged,VitalsChanged,VitalsChanged,VitalsChanged,EncounterChanged,CombatLogChanged] caio[VitalsChanged,EncounterChanged,CombatLogChanged] ana[VitalsChanged,EncounterChanged,CombatLogChanged] bia[VitalsChanged,EncounterChanged,CombatLogChanged]"
		masterAloneAgain = "master[SpellCastsChanged,VitalsChanged,VitalsChanged,VitalsChanged,EncounterChanged,CombatLogChanged] caio[VitalsChanged,EncounterChanged] ana[VitalsChanged,EncounterChanged] bia[VitalsChanged,EncounterChanged]"
	)
	cases := []struct {
		name       string
		catalogKey string
		hidden     bool // the Goblin is hidden
		hideEffect bool // the master keeps the effect from the players
		want       string
		again      string // what the retry tells
	}{
		{"a condition the master keeps from the players", "condition:blinded", false, true, masterAlone, masterAloneAgain},
		{"a master-only effect on a hidden NPC", holdPerson, true, true, masterAlone, masterAloneAgain},
		{"a public spell on a visible NPC", holdPerson, false, false, bothHeard, bothAgain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newEffectsParty(t)
			e := a.closeFight(t)
			if tc.hidden {
				a.execSQL(t, `UPDATE combatants SET hidden = true WHERE id = $1`, a.id(t, "Goblin"))
			}
			h := a.listen(t)
			key := newKey()
			edit := func(r *playv1.AddLastingEffectRequest) {
				r.IdempotencyKey, r.CatalogKey, r.TargetIds = key, tc.catalogKey, []string{a.id(t, "Goblin")}
				if tc.hideEffect {
					r.PlayerVisible = new(false)
				}
				if tc.catalogKey == holdPerson {
					r.CasterId = a.id(t, "Pensantus")
				}
			}

			first, err := a.addRaw(t, a.master, e, edit)
			if err != nil {
				t.Fatalf("AddLastingEffect() error = %v", err)
			}
			if got := a.heard(t, h); got != tc.want {
				t.Errorf("the first call told %s, want %s", got, tc.want)
			}

			again, err := a.addRaw(t, a.master, e, edit)
			if err != nil {
				t.Fatalf("the same call again: error = %v", err)
			}
			if got := a.heard(t, h); got != tc.again {
				t.Errorf("the retry told %s, want %s", got, tc.again)
			}
			if !proto.Equal(first, again) {
				t.Errorf("the retry answered %v, want the first answer %v", again, first)
			}
		})
	}
}
