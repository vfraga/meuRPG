package migrations

import (
	"fmt"
	"testing"
)

// TestCombatantMountMigration: 00333 gives a combatant the columns of the mounted combat with the defaults that keep
// every old row unmounted, refuses a mount that carries itself, a second rider on one mount and an unknown kind, lets a
// deleted mount free its rider, adds the two event kinds, runs again without changing anything and comes down cleanly.
func TestCombatantMountMigration(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	ctx := t.Context()
	provider, err := NewTestProvider(ctx, db)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if _, err := provider.UpTo(ctx, 329); err != nil {
		t.Fatalf("UpTo(329) error = %v", err)
	}
	scan := func(query string, dest ...any) {
		t.Helper()
		if err := db.QueryRowContext(ctx, query).Scan(dest...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	var user, campaign, session, character, encounter string
	scan(`INSERT INTO users DEFAULT VALUES RETURNING id`, &user)
	scan(fmt.Sprintf(`INSERT INTO campaigns (name, xp_mode, created_by) VALUES ('Mirathel', 'enemies', '%s') RETURNING id`, user), &campaign)
	scan(fmt.Sprintf(`INSERT INTO game_sessions (campaign_id, session_number, started_at) VALUES ('%s', 1, now()) RETURNING id`, campaign), &session)
	scan(fmt.Sprintf(`INSERT INTO characters (campaign_id, kind, name, sheet, master_user_id, created_at, updated_at)
		VALUES ('%s', 'minion', 'Cavalo', '{}', '%s', now(), now()) RETURNING id`, campaign, user), &character)
	scan(fmt.Sprintf(`INSERT INTO encounters (game_session_id, name, status, grid_columns, grid_rows, created_at, mode)
		VALUES ('%s', 'No mapa', 'ended', 20, 10, now(), 'grid') RETURNING id`, session), &encounter)
	combatant := func(label string) string {
		var id string
		scan(fmt.Sprintf(`INSERT INTO combatants (encounter_id, character_id, label, kind, hidden, initiative_bonus, order_index, speed_ft, created_at, hp_current, hp_max, hp_temp)
			VALUES ('%s', '%s', '%s', 'npc', false, 0, 0, 30, now(), 7, 7, 0) RETURNING id`, encounter, character, label), &id)
		return id
	}
	old := combatant("Antiga")
	horse, rider, other := combatant("Cavalo"), combatant("Cavaleiro"), combatant("Outro")

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	// An old row is unmounted: no steed, no kind, nothing kept, no move used.
	var steed *string
	var kind string
	var prior *string
	var used bool
	scan(fmt.Sprintf(`SELECT mounted_on_id, mount_kind, mount_prior_init::text, mount_move_used FROM combatants WHERE id = '%s'`, old), &steed, &kind, &prior, &used)
	if steed != nil || kind != "" || prior != nil || used {
		t.Errorf("an old row = steed %v, kind %q, prior %v, used %v; want none, empty, none, false", steed, kind, prior, used)
	}

	refused := func(what, query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err == nil {
			t.Errorf("%s was accepted", what)
		}
	}
	refused("a mount that carries itself", fmt.Sprintf(`UPDATE combatants SET mounted_on_id = id WHERE id = '%s'`, rider))
	refused("an unknown mount kind", fmt.Sprintf(`UPDATE combatants SET mount_kind = 'wild' WHERE id = '%s'`, horse))
	refused("a prior initiative that is not an object", fmt.Sprintf(`UPDATE combatants SET mount_prior_init = '[]' WHERE id = '%s'`, horse))
	exec(t, db, fmt.Sprintf(`UPDATE combatants SET mount_kind = 'controlled', mount_prior_init = '{"initiative": 12, "face": 9, "tie": false}' WHERE id = '%s'`, horse))
	exec(t, db, fmt.Sprintf(`UPDATE combatants SET mounted_on_id = '%s' WHERE id = '%s'`, horse, rider))
	refused("a second rider on one mount", fmt.Sprintf(`UPDATE combatants SET mounted_on_id = '%s' WHERE id = '%s'`, horse, other))

	// A mount that leaves the combat frees its rider.
	exec(t, db, fmt.Sprintf(`DELETE FROM combatants WHERE id = '%s'`, horse))
	scan(fmt.Sprintf(`SELECT mounted_on_id FROM combatants WHERE id = '%s'`, rider), &steed)
	if steed != nil {
		t.Errorf("the rider of a deleted mount is still on %v", *steed)
	}

	var kinds int
	scan(`SELECT count(*) FROM session_event_kinds WHERE kind IN ('combatant_mounted', 'combatant_dismounted', 'mount_kind_set')`, &kinds)
	if kinds != 3 {
		t.Errorf("event kinds = %d, want 3", kinds)
	}

	// A second run changes nothing and a way down removes the columns and the kinds.
	exec(t, db, "DELETE FROM goose_db_version WHERE version_id = 333")
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("second Up() error = %v", err)
	}
	if _, err := provider.DownTo(ctx, 329); err != nil {
		t.Fatalf("DownTo(329) error = %v", err)
	}
	var columns int
	scan(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'combatants' AND column_name IN ('mounted_on_id', 'mount_kind', 'mount_prior_init', 'mount_move_used')`, &columns)
	scan(`SELECT count(*) FROM session_event_kinds WHERE kind IN ('combatant_mounted', 'combatant_dismounted', 'mount_kind_set')`, &kinds)
	if columns != 0 || kinds != 0 {
		t.Errorf("after the way down: %d columns, %d kinds; want none", columns, kinds)
	}
}
