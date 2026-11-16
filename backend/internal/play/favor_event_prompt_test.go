package play

import (
	"testing"
)

// The spend that opens a refund prompt stamps its id on every event it writes (favor_prompt next to favor), wherever the spend is stamped:
// an undo that is added later gives the Inspiração back by looking at the prompt, and finds none if an event has only the flag.

// countStamped is how many records of a roll that says it was made with the Inspiração hold the prompt's id as well: the events of the
// session (a scene check, a trap search, a concentration save) and the rolls the contests, the Hide and the group checks keep in their own
// tables (contestRoll).
func countStamped(t *testing.T, f *favorParty, promptID string) (withFavor, withPrompt int) {
	t.Helper()
	err := f.h.pool.QueryRow(t.Context(), `SELECT count(*), count(*) FILTER (WHERE doc LIKE '%"favor_prompt": "' || $2::text || '"%') FROM (
			SELECT payload::text AS doc FROM session_events WHERE game_session_id = $1
			UNION ALL SELECT initiator_roll::text FROM combat_contests WHERE initiator_roll IS NOT NULL
			UNION ALL SELECT defender_roll::text FROM combat_contests WHERE defender_roll IS NOT NULL
			UNION ALL SELECT roll::text FROM combat_hide_attempts
			UNION ALL SELECT roll::text FROM group_check_members WHERE roll IS NOT NULL
		) d WHERE doc LIKE '%"favor": true%'`, f.sessionID, promptID).Scan(&withFavor, &withPrompt)
	if err != nil {
		t.Fatalf("count the records of the spend: %v", err)
	}
	return withFavor, withPrompt
}

func (f *favorParty) onlyPromptID(t *testing.T) string {
	t.Helper()
	var ids []string
	rows, err := f.h.pool.Query(t.Context(), `SELECT id::text FROM favor_refund_prompts WHERE game_session_id = $1`, f.sessionID)
	if err != nil {
		t.Fatalf("read the prompts: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("read a prompt: %v", err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 1 {
		t.Fatalf("the spend opened %d refund prompts, want 1", len(ids))
	}
	return ids[0]
}

// The checks the roller makes for itself and the combat's checks, each over an advantage only the master reads: the event each writes holds
// the prompt's id.
func TestTheEventOfASpendThatOpenedAPromptHoldsItsIDOnACheck(t *testing.T) {
	t.Parallel()
	forEachCheck(t, func(t *testing.T, e *checkEnv) {
		t.Helper()
		if e.name == hintTryName {
			t.Skip("a hint try keeps no event of the roll and no contestRoll: its row has no favor flag to stamp")
		}
		e.give(t, e.who)
		if e.fight != nil {
			e.hideOnToren(t, false, "["+checkAdvAll+"]")
		} else {
			e.hideOutside(t, e.who.GetId(), "["+checkAdvAll+"]")
		}
		e.h.roller.queue(4, 17)
		if _, err := e.send(t, newKey(), true); err != nil {
			t.Fatalf("%s with the Inspiration error = %v", e.name, err)
		}
		withFavor, withPrompt := countStamped(t, e.favorParty, e.onlyPromptID(t))
		if withFavor == 0 || withPrompt != withFavor {
			t.Errorf("%s kept %d records of the spend, %d of them with the prompt's id, want all of them", e.name, withFavor, withPrompt)
		}
	})
}

// The concentration save of a window.
func TestTheEventOfAConcentrationSaveThatOpenedAPromptHoldsItsID(t *testing.T) {
	t.Parallel()
	p := favorSavePaths[0]
	f, w, u, who := p.setup(t)
	f.give(t, who)
	f.hiddenEffect(t, p.actor, false, "["+saveAdvCon+"]")
	f.h.roller.queue(18, 17)
	if _, err := p.send(t, f, w, u, newKey(), true); err != nil {
		t.Fatalf("the concentration save with the Inspiration error = %v", err)
	}
	withFavor, withPrompt := countStamped(t, f.favorParty, f.onlyPromptID(t))
	if withFavor == 0 || withPrompt != withFavor {
		t.Errorf("the save kept %d records of the spend, %d of them with the prompt's id, want all of them", withFavor, withPrompt)
	}
}
