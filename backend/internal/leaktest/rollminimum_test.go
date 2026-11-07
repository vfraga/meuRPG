package leaktest

import (
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The table's roll_minimum (#757): the number a d20 of an attack or a die of damage counted as, and the feature that did
// it, go with the face of the die. They are the roller's player's and the master's: another player reads nothing of the
// roll, so nothing of the minimum either, in the combat, in the log, in the answers or in the live stream. The roller and
// the master are the positive controls (checkCanariesAreReachable finds each canary in their answers).
//
// The world is the attack rolls': Toren, the paladin of Caio, holds the table feat that gives both minimums.

const (
	// minimumAttack is the number an attack d20 of 2 counts as; minimumDamage the number the dice of a Divine Smite count as.
	minimumAttack = 12
	minimumDamage = 7
	// Numbers that turn up all over an answer are searched in the fields that name them.
	// minimumShot is the number the d6 of Pensantus's shortbow counts as: a damage with no parts of its own.
	minimumShot        = 6
	kindMinimumShot    = "minimum-shot-counted"
	kindMinimumSrc     = "minimum-source"
	kindMinimumShotSrc = "minimum-shot-source"
	kindMinimumD20     = "minimum-d20-counted"
	kindMinimumDamage  = "minimum-damage-counted"

	// The floors of the other rolls: Toren's grapple (a check) counts a 2 as minimumContest, Pensantus's initiative d20 of 10
	// as minimumInitiative, and the d20 of his saving throw against a trap as minimumSave.
	minimumContest    = 16
	minimumInitiative = 18
	minimumSave       = 15
	kindContestD20    = "minimum-contest-counted"
	kindContestSrc    = "minimum-contest-source"
	kindInitiativeD20 = "minimum-initiative-counted"
	kindInitiativeSrc = "minimum-initiative-source"
	kindSaveD20       = "minimum-save-counted"
	kindSaveSrc       = "minimum-save-source"

	// The names of the five feats, which the rolls name to the roller's player and the master (the *_name fields).
	nameAttackFeat, nameShotFeat, nameContestFeat, nameInitiativeFeat, nameSaveFeat = "Piso da Mesa", "Piso do Arco", "Piso da Disputa", "Piso da Iniciativa", "Piso da Salvacao"
)

// gift gives a character a table feat with one minimum for one kind of roll and returns the feat's key.
func (rt *rollsTable) gift(who **charactersv1.Character, name, roll string, value string) string {
	ctx := rt.t.Context()
	entry := must(rt.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: rt.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: name, DescPt: []string{"Um talento da mesa."},
			Effects: []*rulesv1.TableEffect{{Type: "roll_minimum", Rolls: []string{roll}, Value: value}},
		}},
	}))).GetEntry()
	sheet := (*who).GetSheet()
	sheet.GetFull().FeatKeys = append(sheet.GetFull().GetFeatKeys(), entry.GetKey())
	*who = must(rt.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: rt.campaign, CharacterId: (*who).GetId(), Revision: (*who).GetRevision(), Name: (*who).GetName(), Sheet: sheet,
	}))).GetCharacter()
	return entry.GetKey()
}

// minimumFeat gives Toren the table feat with a minimum for the attack d20 and one for the dice of damage, and returns its key.
func (rt *rollsTable) minimumFeat() string {
	ctx := rt.t.Context()
	entry := must(rt.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: rt.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: nameAttackFeat, DescPt: []string{"Um talento da mesa."},
			Effects: []*rulesv1.TableEffect{
				{Type: "roll_minimum", Rolls: []string{"attack"}, Value: "12"},
				{Type: "roll_minimum", Rolls: []string{"damage"}, Value: "7"},
			},
		}},
	}))).GetEntry()
	sheet := rt.toren.GetSheet()
	sheet.GetFull().FeatKeys = append(sheet.GetFull().GetFeatKeys(), entry.GetKey())
	rt.toren = must(rt.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: rt.campaign, CharacterId: rt.toren.GetId(), Revision: rt.toren.GetRevision(), Name: rt.toren.GetName(), Sheet: sheet,
	}))).GetCharacter()
	return entry.GetKey()
}

// shotFeat gives Pensantus, Ana's barbarian, a feat with a minimum for the dice of damage and returns its key.
func (rt *rollsTable) shotFeat() string {
	ctx := rt.t.Context()
	entry := must(rt.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: rt.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: nameShotFeat, DescPt: []string{"Um talento da mesa."},
			Effects: []*rulesv1.TableEffect{{Type: "roll_minimum", Rolls: []string{"damage"}, Value: "6"}},
		}},
	}))).GetEntry()
	sheet := rt.pens.GetSheet()
	sheet.GetFull().FeatKeys = append(sheet.GetFull().GetFeatKeys(), entry.GetKey())
	rt.pens = must(rt.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: rt.campaign, CharacterId: rt.pens.GetId(), Revision: rt.pens.GetRevision(), Name: rt.pens.GetName(), Sheet: sheet,
	}))).GetCharacter()
	return entry.GetKey()
}

func TestRollMinimumReachesOnlyWhoMayReadIt(t *testing.T) {
	var feat, shotFeat, contestFeat, initiativeFeat, saveFeat string
	rt := newRollsTableOf(t, true, true, func(rt *rollsTable) {
		feat, shotFeat = rt.minimumFeat(), rt.shotFeat()
		contestFeat = rt.gift(&rt.toren, nameContestFeat, "check", "16")
		initiativeFeat = rt.gift(&rt.pens, nameInitiativeFeat, "check", "18")
		saveFeat = rt.gift(&rt.pens, nameSaveFeat, "save", "15")
	})
	w, m, caio, ana := rt.world, rt.master, rt.caio, rt.ana
	rt.buildRows()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	w.secrets.add(&canary{needle: feat, kind: kindMinimumSrc, readers: names([]*person{caio})})
	for _, n := range []struct {
		needle string
		reader *person
	}{{nameAttackFeat, caio}, {nameShotFeat, ana}, {nameContestFeat, caio}, {nameInitiativeFeat, ana}, {nameSaveFeat, ana}} {
		w.secrets.add(&canary{needle: n.needle, kind: "minimum-name-" + n.needle, readers: names([]*person{n.reader})})
	}
	w.secrets.add(&canary{needle: "12", kind: kindMinimumD20, numeric: true, value: minimumAttack, fields: []string{"treated_as"}, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: "7", kind: kindMinimumDamage, numeric: true, value: minimumDamage, fields: []string{"counted_as"}, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: shotFeat, kind: kindMinimumShotSrc, readers: names([]*person{ana})})
	w.secrets.add(&canary{needle: "6", kind: kindMinimumShot, numeric: true, value: minimumShot, fields: []string{"counted_as"}, readers: names([]*person{ana})})
	for _, label := range []string{labelToren, labelPens, "Esqueleto", "Arbusto", "Bandido", "Lobo"} {
		w.secrets.id("minimum-combatant", rt.ids[label], ana, caio)
	}
	w.secrets.add(&canary{needle: contestFeat, kind: kindContestSrc, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: "16", kind: kindContestD20, numeric: true, value: minimumContest, fields: []string{"treated_as"}, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: initiativeFeat, kind: kindInitiativeSrc, readers: names([]*person{ana})})
	w.secrets.add(&canary{needle: "18", kind: kindInitiativeD20, numeric: true, value: minimumInitiative, fields: []string{"treated_as"}, readers: names([]*person{ana})})
	w.secrets.add(&canary{needle: saveFeat, kind: kindSaveSrc, readers: names([]*person{ana})})
	w.secrets.add(&canary{needle: "15", kind: kindSaveD20, numeric: true, value: minimumSave, fields: []string{"treated_as"}, readers: names([]*person{ana})})
	rt.rows = append(rt.rows, read{
		procedure: playv1connect.ContestServiceGetContestStateProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.GetContestStateRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
		},
	})
	watchers := map[*person]*streamWatcher{m: w.watchStream(m), ana: w.watchStream(ana), caio: w.watchStream(caio)}
	rt.toTurn(labelToren)
	rt.ask(t, "the combat begins")

	// 0. Pensantus's initiative d20 shows a 10 and counts as 18: the master's and his player's; Caio reads none of it.
	for who, p := range map[string]*person{"the master": m, "Ana": ana} {
		c := rt.combatantOf(p, labelPens)
		if c.GetInitiativeTreatedAs() != minimumInitiative || c.GetInitiativeTreatedAsSource() != initiativeFeat || c.GetInitiativeTreatedAsSourceName() != nameInitiativeFeat {
			t.Errorf("%s reads Pensantus's initiative counted as %d by %q, want %d by the feat (the positive control)", who, c.GetInitiativeTreatedAs(), c.GetInitiativeTreatedAsSource(), minimumInitiative)
		}
	}
	if c := rt.combatantOf(caio, labelPens); c.InitiativeTreatedAs != nil || c.GetInitiativeTreatedAsSource() != "" || c.GetInitiativeTreatedAsSourceName() != "" {
		t.Errorf("Caio reads Pensantus's initiative counted as %d by %q, want nothing of another player's roll", c.GetInitiativeTreatedAs(), c.GetInitiativeTreatedAsSource())
	}

	// 1. Toren's attack shows a 2, which counts as 12 and hits the skeleton: the d20 counted is the roller's and the master's.
	hit := rt.mustAttack(caio, labelToren, dagger, "Esqueleto", []int32{2}, nil)
	roll := hit.GetRoll().GetD20()
	if hit.GetPendingDamage() == nil || roll.GetTreatedAs() != minimumAttack || roll.GetTreatedAsSource() != feat || roll.GetTreatedAsSourceName() != nameAttackFeat {
		t.Fatalf("Toren's attack = %v, want the 2 counted as %d by the feat, and a hit", hit.GetRoll(), minimumAttack)
	}
	rt.ask(t, "the attack counted the minimum")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		lines := rt.attackLines(p, "Esqueleto")
		if len(lines) != 1 || lines[0].GetAttackRoll().GetTreatedAs() != minimumAttack || lines[0].GetAttackRoll().GetTreatedAsSource() != feat || lines[0].GetAttackRoll().GetTreatedAsSourceName() != nameAttackFeat {
			t.Errorf("%s reads %v, want the d20 counted as %d by the feat in the log (the positive control)", who, lines, minimumAttack)
		}
	}
	for _, en := range rt.attackLines(ana, "Esqueleto") {
		if en.GetAttackRoll() != nil && (en.GetAttackRoll().TreatedAs != nil || en.GetAttackRoll().GetTreatedAsSource() != "" || en.GetAttackRoll().GetTreatedAsSourceName() != "") {
			t.Errorf("Ana reads the d20 counted as %v by %q, want nothing of another player's roll", en.GetAttackRoll().GetTreatedAs(), en.GetAttackRoll().GetTreatedAsSource())
		}
	}

	// 2. The damage is a Divine Smite: the weapon's d4 shows a 1 and counts as its highest face, the smite's d8 dice and the
	// extra die (against an undead, so it counts) show 1 and count as the minimum.
	w.dice.ScriptFor(t, 4, 1)
	w.dice.ScriptFor(t, 8, 1, 1, 1)
	rt.damage(caio, hit.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
		r.ExtrasChosen = true
		r.SelectedExtras = []*playv1.SelectedExtra{{Key: "divine-smite", SlotLevel: 1}}
		r.Roll = &playv1.RollDamageRequest_RollInApp{RollInApp: true}
	})
	rt.ask(t, "the damage counted the minimum")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		lines := rt.attackLines(p, "Esqueleto")
		var smite *playv1.DamagePartRoll
		if len(lines) == 1 {
			for _, pr := range lines[0].GetDamage().GetParts() {
				if pr.GetPartKey() == "divine-smite" {
					smite = pr
				}
			}
		}
		if smite == nil || len(smite.GetCountedDice()) != 2 || smite.GetCountedDice()[0].GetCountedAs() != minimumDamage || smite.GetCountedDice()[0].GetSourceKey() != feat || smite.GetCountedDice()[0].GetSourceName() != nameAttackFeat {
			t.Errorf("%s reads the smite %v, want its dice counted as %d by the feat in the log (the positive control)", who, smite, minimumDamage)
		}
	}
	for _, en := range rt.attackLines(ana, "Esqueleto") {
		if len(en.GetDamage().GetParts()) != 0 {
			t.Errorf("Ana reads the parts %v, want nothing of another player's damage", partLines(en))
		}
	}

	// 3. Pensantus's shortbow has no parts: the d6 shows a 1 and counts as 6. The plain roll's counted die is Ana's (the roller)
	// and the master's, in the answer, the pending damage and the log; Caio reads none of it.
	rt.endTurn(caio)
	rt.toTurn(labelPens)
	shot := rt.mustAttack(ana, labelPens, shortbow, "Bandido", []int32{15, 15}, nil)
	if shot.GetPendingDamage() == nil {
		t.Fatalf("Pensantus's shot = %v, want a hit", shot.GetRoll())
	}
	w.dice.ScriptFor(t, 6, 1)
	dealt := rt.damage(ana, shot.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) { r.Roll = &playv1.RollDamageRequest_RollInApp{RollInApp: true} }).GetPendingDamage()
	counted := dealt.GetRoll().GetCountedDice()
	if len(counted) != 1 || counted[0].GetCountedAs() != minimumShot || counted[0].GetSourceKey() != shotFeat || counted[0].GetSourceName() != nameShotFeat {
		t.Fatalf("Ana's damage = %v, want the 1 counted as %d by her feat", dealt.GetRoll(), minimumShot)
	}
	rt.ask(t, "the shot counted the minimum")
	for who, p := range map[string]*person{"the master": m, "Ana": ana} {
		lines := rt.attackLines(p, "Bandido")
		var roll *playv1.DiceRoll
		for _, en := range lines {
			if en.GetDamage().GetRoll() != nil && en.GetActorId() == rt.ids[labelPens] {
				roll = en.GetDamage().GetRoll()
			}
		}
		if roll == nil || len(roll.GetCountedDice()) != 1 || roll.GetCountedDice()[0].GetSourceKey() != shotFeat || roll.GetCountedDice()[0].GetSourceName() != nameShotFeat {
			t.Errorf("%s reads the shot's roll %v, want the counted die with its source in the log (the positive control)", who, roll)
		}
	}
	for _, en := range rt.attackLines(caio, "Bandido") {
		if en.GetActorId() == rt.ids[labelPens] && len(en.GetDamage().GetRoll().GetCountedDice()) != 0 {
			t.Errorf("Caio reads the counted dice %v of another player's shot, want none", en.GetDamage().GetRoll().GetCountedDice())
		}
	}

	// 4. The master fires a trap at Pensantus: the d20 of his saving throw shows a 2 and counts as 15. The save is his player's and
	// the master's, in the log; Caio reads the outcome alone.
	trap := rt.point(rt.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, "Fosso", "Um fosso.", 10, 12, func(r *mapsv1.CreateMapPointRequest) { r.Trap = rt.trapSpec() })
	w.dice.ScriptFor(t, 20, 2)
	must(m.play.FireTrap(t.Context(), rq(&playv1.FireTrapRequest{
		CampaignId: w.campaign, MapId: w.fogMap, PointId: trap.GetId(), TargetIds: []string{rt.ids[labelPens]}, IdempotencyKey: newKey(),
	})))
	rt.ask(t, "the trap save counted the minimum")
	trapSave := func(p *person) *playv1.TrapSaveRoll {
		var save *playv1.TrapSaveRoll
		for _, r := range rt.logOf(p).GetRounds() {
			for _, en := range r.GetEntries() {
				for _, c := range en.GetTrap().GetCaught() {
					if c.GetTargetLabel() == labelPens {
						save = c.GetSave()
					}
				}
			}
		}
		return save
	}
	for who, p := range map[string]*person{"the master": m, "Ana": ana} {
		if roll := trapSave(p).GetRoll(); roll.GetTreatedAs() != minimumSave || roll.GetTreatedAsSource() != saveFeat || roll.GetTreatedAsSourceName() != nameSaveFeat {
			t.Errorf("%s reads the save %v, want the 2 counted as %d by the feat (the positive control)", who, trapSave(p), minimumSave)
		}
	}
	if save := trapSave(caio); save == nil || save.GetRoll() != nil {
		t.Errorf("Caio reads the save %v, want the outcome alone", save)
	}

	// 5. Toren grapples Pensantus on the next round: the Athletics d20 of the grappler shows a 2 and counts as 16, and Pensantus's
	// shows a 1 and counts as 18 (the same table minimum as his initiative). Each is its roller's and the master's: the other
	// player is a party to the contest and reads the outcome, and none of the dice.
	rt.endTurn(ana)
	rt.toTurn(labelToren)
	must(m.combat.MoveCombatant(t.Context(), rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelPens], IdempotencyKey: newKey(), Col: 4, Row: 4, Forced: true})))
	faces := func(f int32) *playv1.CheckRollInput {
		return &playv1.CheckRollInput{Roll: &playv1.CheckRollInput_D20Faces{D20Faces: &playv1.D20Faces{Faces: []int32{f}}}}
	}
	started := must(caio.contests.StartContest(t.Context(), rq(&playv1.StartContestRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), InitiatorId: rt.ids[labelToren], TargetId: rt.ids[labelPens],
		Purpose: playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE, Roll: faces(2),
	}))).GetContest()
	if roll := started.GetInitiatorRoll(); roll.GetTreatedAs() != minimumContest || roll.GetTreatedAsSource() != contestFeat || roll.GetTreatedAsSourceName() != nameContestFeat {
		t.Fatalf("Toren's grapple = %v, want the 2 counted as %d by the feat", started.GetInitiatorRoll(), minimumContest)
	}
	rt.ask(t, "the contest waits for the defender")
	contestOf := func(p *person) *playv1.ContestView {
		for _, c := range must(p.contests.GetContestState(t.Context(), rq(&playv1.GetContestStateRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId()}))).GetContests() {
			if c.GetId() == started.GetId() {
				return c
			}
		}
		return nil
	}
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		if roll := contestOf(p).GetInitiatorRoll(); roll.GetTreatedAs() != minimumContest || roll.GetTreatedAsSource() != contestFeat || roll.GetTreatedAsSourceName() != nameContestFeat {
			t.Errorf("%s reads the contest %v, want the grappler's 2 counted as %d by the feat (the positive control)", who, contestOf(p), minimumContest)
		}
	}
	if v := contestOf(ana); v == nil || v.GetInitiatorRoll() != nil {
		t.Errorf("Ana reads the contest %v, want a party to it that reads the outcome and not the grappler's roll", v)
	}
	must(ana.contests.RespondContest(t.Context(), rq(&playv1.RespondContestRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), ContestId: started.GetId(),
		Skill: playv1.ContestSkill_CONTEST_SKILL_ATHLETICS, Roll: faces(1),
	})))
	rt.ask(t, "the contest is settled")
	for who, p := range map[string]*person{"the master": m, "Ana": ana} {
		if roll := contestOf(p).GetDefenderRoll(); roll.GetTreatedAs() != minimumInitiative || roll.GetTreatedAsSource() != initiativeFeat || roll.GetTreatedAsSourceName() != nameInitiativeFeat {
			t.Errorf("%s reads the contest %v, want the defender's 1 counted as %d by the feat (the positive control)", who, contestOf(p), minimumInitiative)
		}
	}
	if v := contestOf(caio); v == nil || v.GetDefenderRoll() != nil {
		t.Errorf("Caio reads the contest %v, want the outcome and not the defender's roll", v)
	}

	// 6. The master retires the attack's feat and the grapple's: the rolls keep their keys and name nothing, to the roller and
	// to the master alike (RN-23).
	must(m.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: feat})))
	must(m.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: contestFeat})))
	rt.ask(t, "the feats were retired")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		lines := rt.attackLines(p, "Esqueleto")
		if len(lines) != 1 || lines[0].GetAttackRoll().GetTreatedAsSource() != feat || lines[0].GetAttackRoll().GetTreatedAsSourceName() != "" {
			t.Errorf("%s reads %v after the feat was retired, want the key kept and no name", who, lines)
		}
		if roll := contestOf(p).GetInitiatorRoll(); roll.GetTreatedAsSource() != contestFeat || roll.GetTreatedAsSourceName() != "" {
			t.Errorf("%s reads the grapple %v after the feat was retired, want the key kept and no name", who, roll)
		}
	}

	// The end: the stream told the master and the roller of the steps, and carried none of the minimum to Ana.
	must(m.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey()})))
	must(m.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
		}
		for _, ev := range sw.events {
			body, err := protojson.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal an event: %v", err)
			}
			r := reply{status: 200, body: body, msg: ev}
			rt.got.keep(p, r)
			if p == m {
				continue
			}
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, rt.got) })
}
