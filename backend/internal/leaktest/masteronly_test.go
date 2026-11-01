package leaktest

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The fields a player always reads empty (layer 3 of inspect). Each entry names the
// message and the field, and why: the comment in the .proto file says the same. A field
// is listed here when no value of it is ever a player's; a field that is the player's in
// some cases (their own character's) carries an `unless`. TestMasterOnlyFieldsExist fails
// when a name here is not in the descriptors, so a rename cannot hide a field.

// own is true when the message is the caller's own: `mine` says it in map tokens and in
// combatants.
func own(w *world, p *person, m protoreflect.Message) bool {
	fd := m.Descriptor().Fields().ByName("mine")
	return (fd != nil && m.Get(fd).Bool()) || ownsCreature(w, p, m)
}

// ownsCreature is true for a creature combatant whose owner is the caller's character: its player plays it, so the
// numbers of its turn are theirs.
func ownsCreature(w *world, p *person, m protoreflect.Message) bool {
	kind, owner := m.Descriptor().Fields().ByName("kind"), m.Descriptor().Fields().ByName("owner_character_id")
	if kind == nil || owner == nil || m.Get(kind).Enum() != 3 { // COMBATANT_KIND_CREATURE
		return false
	}
	return m.Get(owner).String() == w.ownCharacter(p).GetId() && (p == w.ana || p == w.caio || p == w.pending)
}

// notAPlayerCharacter is true for a combatant that is an NPC or a creature.
func playerCharacter(_ *world, _ *person, m protoreflect.Message) bool {
	fd := m.Descriptor().Fields().ByName("kind")
	return fd != nil && m.Get(fd).Enum() == 1 // COMBATANT_KIND_PLAYER
}

// targetsPlayer is true for a damage on a player's character, as the master's copy of the
// combat says who that is.
func targetsPlayer(w *world, m protoreflect.Message) bool {
	fd := m.Descriptor().Fields().ByName("target_id")
	if fd == nil || w.encounter == nil {
		return false
	}
	for _, c := range w.encounter.GetCombatants() {
		if c.GetId() == m.Get(fd).String() {
			return c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER
		}
	}
	return false
}

func ownOrAnyPlayerCharacter(w *world, p *person, m protoreflect.Message) bool {
	return own(w, p, m) || playerCharacter(w, p, m)
}

var masterOnly = []masterOnlyField{
	// --- maps
	{"meurpg.maps.v1.Map", "base_light", "the light of an unlit square is the master's setting", nil},
	{"meurpg.maps.v1.Map", "generated_dungeon", "only the master reads a dungeon's rooms", nil},
	{"meurpg.maps.v1.MapImage", "name", "the gallery is the master's preparation", nil},
	{"meurpg.maps.v1.MapPoint", "hooks", "the master's private text on a scene (RN-20)", nil},
	{"meurpg.maps.v1.MapPoint", "clues", "a clue not revealed is the master's; revealed ones are in the notes", nil},
	{"meurpg.maps.v1.MapPoint", "light", "a player never receives a light point", nil},
	{"meurpg.maps.v1.MapPoint", "trap_revealed_to", "who knows a trap is the master's card", nil},
	{"meurpg.maps.v1.MapPoint", "treasure_converted", "XP bookkeeping", nil},
	{"meurpg.maps.v1.TrapSpec", "preset_key", "RN-10: a player gets a trap's area and state only", nil},
	{"meurpg.maps.v1.TrapSpec", "notice_dc", "RN-10", nil},
	{"meurpg.maps.v1.TrapSpec", "find_dc", "RN-10", nil},
	{"meurpg.maps.v1.TrapSpec", "trigger", "RN-10", nil},
	{"meurpg.maps.v1.TrapSpec", "effect", "RN-10", nil},
	{"meurpg.maps.v1.SceneAction", "dc", "the fixture never turns \"Mostrar a CD\" on (RN-20)", nil},
	{"meurpg.maps.v1.MapToken", "hidden", "a hidden token is not sent to a player at all", nil},
	{"meurpg.maps.v1.MapToken", "carried_light", "only the master and the character's own player", func(w *world, p *person, m protoreflect.Message) bool { return own(w, p, m) }},

	// --- characters: reserved characters and claim links (MR-049)
	{"meurpg.characters.v1.CharacterSummary", "reserved", "only the master's list holds a reserved character", nil},
	{"meurpg.characters.v1.CharacterSummary", "claim_state", "the state of a claim link is the master's", nil},
	{"meurpg.characters.v1.CharacterSummary", "claim_expires_at", "when a claim link ends is the master's", nil},
	{"meurpg.characters.v1.CharacterSummary", "claimed_by_display_name", "who took a character through a link is the master's", nil},

	// --- play: the open scene and the stage
	{"meurpg.play.v1.OpenSceneInfo", "hooks", "the master's private text (RN-20)", nil},
	{"meurpg.play.v1.OpenSceneInfo", "clues", "the clues are in the player's notes, not on the scene", nil},
	{"meurpg.play.v1.StageNpc", "character_id", "a player gets an NPC's name and portrait only (RN-20)", nil},
	{"meurpg.play.v1.SceneActionView", "dc", "the fixture never turns \"Mostrar a CD\" on (RN-20)", nil},

	// --- the dead (RN-10, Revivify): the reasons a creature cannot be revived are the master's
	{"meurpg.characters.v1.Character", "revivify_blocked", "the master's switch \"Revivificar não funciona nesta morte\"", nil},
	{"meurpg.play.v1.CombatDeath", "fits_revivify", "whether the minute has run out is the master's", nil},
	{"meurpg.play.v1.CombatDeath", "revivify_blocked", "the master's switch", nil},
	{"meurpg.play.v1.PreviewRevivifyResponse", "unavailable", "why a creature is not a target is the master's alone", nil},

	// --- play: the combat (RN-20)
	{"meurpg.play.v1.Encounter", "map_point_id", "the battle point the master started from", nil},
	{"meurpg.play.v1.Encounter", "pending_hidden_reveals", "the questions an area spell left the master: they name the hidden creatures it hit", nil},
	{"meurpg.play.v1.CastSpellResponse", "hidden_hits", "the hidden creatures an area hit are the master's", nil},
	{"meurpg.play.v1.CastSpellResponse", "pending_reveal_id", "a question exists only when the spell hit a hidden creature", nil},
	{"meurpg.play.v1.AreaTarget", "hidden", "who is hidden is the master's", nil},
	{"meurpg.play.v1.CombatLogSpellTarget", "hidden", "who was hidden when the spell hit is the master's", nil},
	{"meurpg.play.v1.Combatant", "hidden", "a hidden combatant is not sent to a player", nil},
	{"meurpg.play.v1.Combatant", "tie_unresolved", "the master's bookkeeping", nil},
	{"meurpg.play.v1.Combatant", "armor_class", "a player never learns an armor class", nil},
	{"meurpg.play.v1.Combatant", "xp_value", "what an NPC gives is the master's", nil},
	{"meurpg.play.v1.Combatant", "portrait_url", "a player sees a portrait only on the stage", nil},
	{"meurpg.play.v1.Combatant", "bestiary_creature_key", "which creature an NPC is", nil},
	{"meurpg.play.v1.Combatant", "challenge_rating", "which creature an NPC is", nil},
	{"meurpg.play.v1.Combatant", "character_id", "an NPC's character id is the master's", func(w *world, p *person, m protoreflect.Message) bool { return playerCharacter(w, p, m) }},
	{"meurpg.play.v1.Combatant", "hit_points_current", "an NPC's numbers", func(w *world, p *person, m protoreflect.Message) bool { return ownOrAnyPlayerCharacter(w, p, m) }},
	{"meurpg.play.v1.Combatant", "hit_points_max", "an NPC's numbers", func(w *world, p *person, m protoreflect.Message) bool { return ownOrAnyPlayerCharacter(w, p, m) }},
	{"meurpg.play.v1.Combatant", "hit_points_temporary", "an NPC's numbers", func(w *world, p *person, m protoreflect.Message) bool { return ownOrAnyPlayerCharacter(w, p, m) }},
	{"meurpg.play.v1.Combatant", "initiative_bonus", "only the master and the combatant's player", func(w *world, p *person, m protoreflect.Message) bool { return own(w, p, m) }},
	{"meurpg.play.v1.Combatant", "initiative_face", "only the master and the combatant's player", func(w *world, p *person, m protoreflect.Message) bool { return own(w, p, m) }},
	{"meurpg.play.v1.Combatant", "death_successes", "the table keeps the death saves to the owner and the master (RN-24)", func(w *world, p *person, m protoreflect.Message) bool { return own(w, p, m) }},
	{"meurpg.play.v1.PendingDamage", "steps", "an NPC's resistances are the master's; a player's character's go to its owner", func(w *world, _ *person, m protoreflect.Message) bool { return targetsPlayer(w, m) }},
	{"meurpg.play.v1.PendingDamage", "amount_after_steps", "the damage after an NPC's resistances is the master's", func(w *world, _ *person, m protoreflect.Message) bool { return targetsPlayer(w, m) }},
	{"meurpg.play.v1.ReactionWindow", "trigger", "the numbers of what happened (total, armor class, damage) are the master's (RN-20)", nil},
	{"meurpg.play.v1.ReactionWindow", "answer_now", "the order the master answers in is his", nil},
	{"meurpg.play.v1.Combatant", "death_failures", "the table keeps the death saves to the owner and the master (RN-24)", func(w *world, p *person, m protoreflect.Message) bool { return own(w, p, m) }},

	// --- play: the contests and the special actions (RN-10, RN-20)
	{"meurpg.play.v1.ContestView", "escape_dc", "the fixed escape DC of a grapple that came from an attack is the master's", nil},
	{"meurpg.play.v1.GrappleView", "escape_dc", "the fixed escape DC of a grapple that came from an attack is the master's", nil},
	{"meurpg.play.v1.HideAttemptView", "observers", "who could see a hider, and what the app compares, is the master's", nil},
	{"meurpg.play.v1.HideObserver", "combatant_id", "who noticed a hider is the master's", nil},
	{"meurpg.play.v1.HideObserver", "passive_perception", "a creature's passive Perception is the master's", nil},
	{"meurpg.play.v1.HideObserver", "known", "whether the app knows a creature's Perception is the master's", nil},
	{"meurpg.play.v1.HideObserver", "noticed", "who noticed a hider is the master's", nil},
	{"meurpg.play.v1.GroupCheckView", "dc", "the DC of a group check is never a player's, even when the master shows passed and failed", nil},
	{"meurpg.play.v1.GroupCheckView", "passed_count", "how many passed is the master's", nil},
	{"meurpg.play.v1.GroupCheckView", "asked_count", "how many characters the master asked is the master's", nil},
	{"meurpg.play.v1.GroupCheckView", "needed", "how many are needed is the master's", nil},
}
