// Package play is about playing a campaign at the table: it starts, ends
// and lists game sessions (PlayService), which is what locks the players'
// sheets (RN-01, MR-006, MR-011), and runs the live session (Etapa 5,
// live.go): the in-app notice that a session is open (RN-06), the
// session's live stream (ADR-0005), the master's correction of the
// characters' vitals (RN-02), each one recorded in session_events
// (ADR-0007), and what the session shows at the table: the current map and
// a gallery image (onscreen.go). Turns and actions come with combat (Etapa
// 6).
//
// Starting a session locks the sheets in the same transaction that opens
// it, through a SheetLocker, and the vitals live in the characters module
// too, reached through a VitalsKeeper: the characters module owns the
// characters tables, so this package never touches them. The campaigns a
// user belongs to come from the campaigns module (CampaignDirectory). The
// maps module checks and reveals the map the master makes current, and
// reads the gallery image the master shows (MapKeeper); the other way
// round, it reads what the session shows and publishes its changes on the
// live stream through OnScreen and Publish (its maps.LiveSession
// interface). cmd/api connects them; no package imports
// another's code.
//
// The SQL lives in queries.sql, and sqlc turns it into package playdb.
// Every write runs inside db.InTx, which retries CockroachDB's serialization
// errors (40001).
package play

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/nostore"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
	"github.com/PuraFome/meuRPG/backend/internal/platform/wiring"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// SheetLocker locks a campaign's player sheets when a game session starts
// (RN-01). The characters module implements it
// (characters.Service.LockSheets).
type SheetLocker interface {
	// LockSheets locks, inside tx, the sheets of the campaign's living
	// player characters that are still drafts, ends every permission to
	// edit a story in the campaign, and returns how many sheets it locked.
	LockSheets(ctx context.Context, tx pgx.Tx, campaignID string, at time.Time) (int64, error)
}

// VitalsKeeper keeps the player characters' vitals: hit points, spell
// slots, hit dice (RN-02, table character_vitals). The characters module
// implements it (characters.Service), because the vitals belong to the
// character and last from one session to the next; this package declares
// the messages that go through it (playv1.CharacterVitals), and serves them
// live. The methods take no caller: they run after this package's own
// authorization check, and their errors are Connect errors to return as
// they are.
type VitalsKeeper interface {
	// ListVitals returns the vitals of the campaign's living, active player
	// characters, oldest first.
	ListVitals(ctx context.Context, campaignID string) ([]*playv1.CharacterVitals, error)
	// GetVitals returns one living, active player character's vitals, or
	// `not_found`.
	GetVitals(ctx context.Context, campaignID, characterID string) (*playv1.CharacterVitals, error)
	// GetVitalsTx is GetVitals inside tx.
	GetVitalsTx(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (*playv1.CharacterVitals, error)
	// AdjustVitals applies the vitals fields of req inside tx, and returns
	// the vitals before and after: `not_found` for anything but a living,
	// active player character of the campaign; `invalid_argument` for a
	// value outside 0 to its maximum.
	AdjustVitals(ctx context.Context, tx pgx.Tx, campaignID, characterID string, req *playv1.AdjustCharacterVitalsRequest) (before, after *playv1.CharacterVitals, err error)

	// The druid's Wild Shape form and the familiar's eyes live on the vitals too
	// (MR-037, MR-036). This package decides when they start and end, and what they
	// cost; the characters module keeps them and says what the character is in the
	// form (its speed, size and jumps, which a combatant copies).

	// AssumeWildShape turns the character into the beast inside tx, with the beast
	// at full hit points, and returns the vitals before and after and the
	// character's numbers as a combatant (the beast's). The errors are
	// link.ErrBeastNotAllowed (the beast is not one its level allows, or it has no
	// Wild Shape) and link.ErrAlreadyInWildShape; `not_found` for any other
	// character. It spends nothing: the caller spends the use and the action.
	AssumeWildShape(ctx context.Context, tx pgx.Tx, campaignID, characterID, beast string) (before, after *playv1.CharacterVitals, body link.Character, err error)
	// SetWildShape puts the form as it says inside tx: the beast with its current
	// hit points (1 or more), or its own shape for an empty beast. It is how the form
	// ends and how an undo puts it back.
	SetWildShape(ctx context.Context, tx pgx.Tx, campaignID, characterID, beast string, hp int32) (after *playv1.CharacterVitals, body link.Character, err error)
	// FamiliarOf returns the character's live familiar and false when it has none.
	FamiliarOf(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (link.Creature, bool, error)
	// SetFamiliarSight records that the character's player looks through the
	// creature's eyes (an empty creatureID: they stopped), whether it started in a
	// combat and the conditions it gave the combatant, and returns the vitals after.
	SetFamiliarSight(ctx context.Context, tx pgx.Tx, campaignID, characterID, creatureID string, inCombat bool, conditions []string) (*playv1.CharacterVitals, error)
}

// ShownCopy is a copy of a gallery image made to show it, not yet in the gallery
// (see MapKeeper.PrepareShow).
type ShownCopy = interface {
	// Insert adds the copy's gallery row inside tx and returns the image to show:
	// the copy, or the one another show committed first (created false: the
	// caller then discards these files, as when the show does not happen).
	Insert(ctx context.Context, tx pgx.Tx) (id string, created bool, err error)
	Discard(ctx context.Context)
}

// MapKeeper is what the session's screen needs from the maps module
// (maps.SessionMaps), whose tables the maps and the gallery images are: it
// checks and reveals the map the master makes current (SetCurrentMap), and
// reads the gallery image the master shows (SetShownImage), and keeps the
// images the master leaves with the players.
type MapKeeper interface {
	// RevealMap reveals the campaign's map inside tx (a revealed map stays
	// as it is), or returns a `not_found` Connect error when mapID is not a
	// map of the campaign.
	RevealMap(ctx context.Context, tx pgx.Tx, campaignID, mapID string, at time.Time) error
	// MapShown tells the maps module, after the commit, that the map became the
	// session's current one: a map with the fog on records the players' first
	// view of it (MR-036).
	MapShown(ctx context.Context, campaignID, mapID string)
	// VisionChanged tells the maps module, after the commit, that what the players
	// of the map see changed with no token moving: a druid took a beast's senses or
	// left them, a player started or stopped looking through their familiar's eyes
	// (MR-036, MR-037). Nothing happens for a map without fog.
	VisionChanged(ctx context.Context, campaignID, mapID string)
	// ShownImage returns the campaign's gallery image imageID as the
	// session shows it, or a `not_found` Connect error when it is not an
	// image of the campaign's gallery.
	ShownImage(ctx context.Context, campaignID, imageID string) (*playv1.ShownImage, error)
	// PrepareShow is ShownImage for an image the master is about to show: one that
	// is the background of a map with the fog of war on comes back as a copy of its
	// own (the one made before, or a new one), whose ID the caller stores (RN-10,
	// MR-036). A new copy has only its files; the caller adds its gallery row inside
	// the transaction that shows it (ShownCopy.Insert, `resource_exhausted` when the
	// gallery has no room) or deletes the files (Discard). The copy is nil when
	// there is nothing to add.
	PrepareShow(ctx context.Context, campaignID, imageID string) (*playv1.ShownImage, ShownCopy, error)
	// LeaveImage adds the campaign's image to the images left with the
	// players inside tx. An image already left stays as it is, and one that
	// is not the campaign's anymore is skipped.
	LeaveImage(ctx context.Context, tx pgx.Tx, campaignID, imageID string, at time.Time) error
	// ListLeftImages returns the images left with the players, oldest
	// first.
	ListLeftImages(ctx context.Context, campaignID string) ([]*playv1.ShownImage, error)
	// TakeBackImage removes the image from the left list, or returns a
	// `not_found` Connect error when it is not on it.
	TakeBackImage(ctx context.Context, campaignID, imageID string) error
	// MapGrid, ScenePoint and MapTokens read inside tx when the caller has one
	// (nil: the pool): a change that holds a transaction must pass it.
	//
	// MapGrid returns the battle grid of the campaign's map, the zero Grid
	// when it has none, or a `not_found` Connect error (MR-013).
	MapGrid(ctx context.Context, tx pgx.Tx, campaignID, mapID string) (link.Grid, error)
	// BattlePoint returns a battle point of the campaign, or a `not_found`
	// Connect error for any other point.
	BattlePoint(ctx context.Context, campaignID, pointID string) (link.BattlePoint, error)
	// ScenePoint returns a SCENE point of the campaign, hidden or not, with its
	// actions and their DCs (MR-015), and the master's hooks and clues (MR-029,
	// never for a player), or a `not_found` Connect error for any other point.
	ScenePoint(ctx context.Context, tx pgx.Tx, campaignID, pointID string) (link.Scene, error)
	// DiscoverScene records inside tx that the group discovered the scene (the
	// master opened it, MR-030); one already discovered stays as it is.
	DiscoverScene(ctx context.Context, tx pgx.Tx, campaignID, pointID string, at time.Time) error
	// MapTokens returns where the map's tokens stand, hidden ones included.
	MapTokens(ctx context.Context, tx pgx.Tx, mapID string) ([]link.TokenPosition, error)
	// SetTokenPositions moves the characters' tokens on the map inside tx,
	// creating the ones that are missing.
	SetTokenPositions(ctx context.Context, tx pgx.Tx, mapID string, positions []link.TokenPosition, at time.Time) error
	// TreasureFoundIn returns the gold pieces each character found in the game
	// session, by character ID: the treasures marked found while it was open,
	// each value split among its finders and rounded down (MR-032, "Mais
	// tesouro encontrado").
	TreasureFoundIn(ctx context.Context, sessionID string) (map[string]int32, error)
}

// CombatRoster tells who can fight and with which numbers (MR-013). The
// characters module implements it (characters.Service), because the sheets
// are its own. The methods take no caller: they run after this package's
// own authorization check.
type CombatRoster interface {
	// CombatParty returns the campaign's living, active player characters,
	// oldest first.
	CombatParty(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.Character, error)
	// CombatCharacters returns those of ids that are living characters of
	// the campaign, players' or NPCs; the others are left out.
	CombatCharacters(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) ([]link.Character, error)
	// PartyLevels returns the campaign's living, active player characters, oldest
	// first, with their total level: the party an encounter's difficulty is measured
	// against (MR-043).
	PartyLevels(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.PartyMember, error)
	// RulesContent is the campaign's rules content, for the SRD creatures and the
	// encounter budget (MR-043).
	RulesContent(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, error)
	// SessionCharacters returns those of ids that are characters of the
	// campaign whatever their status (a dead one too), with only their name and
	// player filled: the session summary names who fought, even if they died.
	SessionCharacters(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) ([]link.Character, error)
	// Every read of this interface takes the caller's transaction (nil: the pool). A
	// change that holds one must pass it: the read then sees what the change wrote
	// (the character's vitals or Wild Shape form), and it takes no second connection,
	// which would wait for the one the transaction keeps (docs/architecture.md,
	// "Dentro de uma transação, nenhuma leitura pelo pool").

	// CombatSheet returns what an attack needs from the sheet of a living
	// character of the campaign, a player's or an NPC's: its armor class, its
	// attacks and the standard actions. Its armor class never goes to a
	// player (RN-20). `not_found` for any other character.
	CombatSheet(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (link.Sheet, error)
	// SpendAmmunition spends one piece of the character's stack of ammunition for a ranged
	// attack (SRD 5.1 "Ammunition"), counting it for the battle's recovery.
	SpendAmmunition(ctx context.Context, tx pgx.Tx, campaignID, characterID, itemID string) error
	// ItemForUse describes an inventory line the combatant may use as an action, and
	// ApplyItemUse does what the use did to the inventory (a potion drunk, a scroll read,
	// charges spent, a shield worn). See the inventory in docs/architecture.md.
	ItemForUse(ctx context.Context, tx pgx.Tx, campaignID, characterID, itemID string) (link.UsableItem, error)
	ApplyItemUse(ctx context.Context, tx pgx.Tx, campaignID, characterID string, w link.ItemUseWrite) error
	// CombatTurnOptions works out what the character can do now (MR-014),
	// from its sheet, what it used this turn and the slots it spent: the rules
	// engine's TurnOptions. `not_found` for any other character.
	CombatTurnOptions(ctx context.Context, tx pgx.Tx, campaignID, characterID string, turn link.Turn) (*rulesv1.TurnOptions, error)
	// CombatSpell returns the spell as the character casts it with a slot of
	// slotLevel (0 for a cantrip): its range, attack or save, damage or healing
	// at that level, with the character's attack bonus, save DC and
	// spellcasting modifier. It does not check that the character may cast it
	// (CombatTurnOptions does). `not_found` for any other character or spell.
	CombatSpell(ctx context.Context, tx pgx.Tx, campaignID, characterID, spellKey string, slotLevel int, damageType string) (link.Spell, error)
	// CombatSave returns the character's saving throw bonus for an ability
	// ("dex"). A basic-sheet NPC has none: Known is false.
	CombatSave(ctx context.Context, tx pgx.Tx, campaignID, characterID, ability string) (link.Save, error)
	// MarkDead marks a player's character dead inside tx, as the master's
	// MarkCharacterDead does (RN-03): the master confirmed its death in a combat.
	// It is idempotent.
	MarkDead(ctx context.Context, tx pgx.Tx, campaignID, characterID string, at time.Time) error
	// SceneOptions returns, for each key of a scene's checks, the character's
	// bonus and passive value (the rules engine's SceneOptions), in the order of
	// keys. `not_found` for any other character.
	SceneOptions(ctx context.Context, tx pgx.Tx, campaignID, characterID string, keys []string) ([]link.SceneOption, error)
	// SceneCheckName is the Portuguese name of a scene check by its key, "" for
	// an unknown one.
	SceneCheckName(key string) string
	// Conditions lists the SRD's conditions (RN-22), with their Portuguese
	// names, sorted by key.
	Conditions() []link.Named
	// ContentNames returns the campaign's naming function: the Portuguese name of a
	// content key ("spell:shield" is "Escudo Arcano"), or "" for an unknown key. The
	// content is read once, when it is asked for, not on every name.
	ContentNames(ctx context.Context, tx pgx.Tx, campaignID string) (func(key string) string, error)

	// The character's creatures (MR-037, Etapa 9). The ones that write take the
	// change's transaction.

	// CharacterCreatures returns the live creatures of the given characters,
	// oldest first: the ones that join a combat with their owners.
	CharacterCreatures(ctx context.Context, tx pgx.Tx, campaignID string, characterIDs []string) ([]link.Creature, error)
	// ConcentrationCreatures returns the live creatures of a character that last
	// only while it concentrates.
	ConcentrationCreatures(ctx context.Context, tx pgx.Tx, campaignID, characterID string) ([]link.Creature, error)
	// CheckSummon says what the spell and the character's sheet allow for a
	// casting (circle 0 is the spell's own) and returns the creatures of the
	// choice, or one of rules.ErrNotSummonSpell, ErrSummonCircle,
	// ErrSummonOption, ErrSummonCount, ErrSummonCreature.
	CheckSummon(ctx context.Context, tx pgx.Tx, campaignID, characterID, spellKey string, circle, option int, keys []string) (link.SummonSpell, error)
	// SummonCreatures records a casting: it creates the creatures, and a new
	// familiar dismisses the old one.
	SummonCreatures(ctx context.Context, tx pgx.Tx, sm link.Summon) (link.SummonResult, error)
	// DismissCreatures sends the live ones of these creatures away with the
	// reason, and returns the IDs it dismissed. ReviveCreatures brings back
	// those dismissed for the reason (an undo), DeleteCreatures takes a casting's
	// creatures away for good.
	DismissCreatures(ctx context.Context, tx pgx.Tx, campaignID string, ids []string, reason string, at time.Time) ([]string, error)
	ReviveCreatures(ctx context.Context, tx pgx.Tx, campaignID string, ids []string, reason string) ([]string, error)
	DeleteCreatures(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) error
	// SyncCreatures keeps the creatures in step with the combat: a defeated one
	// is dismissed, one dismissed as defeated and up again is back.
	SyncCreatures(ctx context.Context, tx pgx.Tx, campaignID string, states []link.CreatureState, at time.Time) (link.CreatureChanges, error)
	// WriteBackCreatures writes the creatures' hit points back when a combat
	// ends or they leave it.
	WriteBackCreatures(ctx context.Context, tx pgx.Tx, campaignID string, states []link.CreatureState, at time.Time) error
	// CreatureSheet, CreatureTurnOptions and CreatureSave are CombatSheet,
	// CombatTurnOptions and CombatSave for a creature, from its stat block and
	// what its spell lets it attack with ("none", "reaction", "full"); false for
	// a key that is not an SRD creature.
	CreatureSheet(ctx context.Context, tx pgx.Tx, campaignID, monsterKey, attack string) (link.Sheet, bool, error)
	CreatureTurnOptions(ctx context.Context, tx pgx.Tx, campaignID, monsterKey, attack string, turn link.Turn) (*rulesv1.TurnOptions, bool, error)
	CreatureSave(ctx context.Context, tx pgx.Tx, campaignID, monsterKey, ability string) (link.Save, error)
	// DamageModifiers are the damage types the target takes double, half or none
	// of (SRD 5.1), from the stat block of the creature it is: the monsterKey when
	// it is a character's creature, otherwise the one on the basic sheet of an NPC
	// made from a creature. Empty for anyone else.
	DamageModifiers(ctx context.Context, tx pgx.Tx, campaignID, characterID, monsterKey string) (combat.TypeModifiers, error)
	// CreatureEyes is what a creature notices a trap with, from its stat block:
	// its passive Perception and its senses (MR-035). False for a key that is not an
	// SRD creature.
	CreatureEyes(ctx context.Context, tx pgx.Tx, campaignID, monsterKey string) (maplink.Eyes, bool, error)

	// The monsters of "Pôr no combate" (MR-042, RN-29).

	// MonsterHitPoints is the creature's average hit points and hit dice; false
	// for a key that is not an SRD creature.
	MonsterHitPoints(ctx context.Context, tx pgx.Tx, campaignID, monsterKey string) (link.MonsterHitPoints, bool, error)
	// MonsterNpc returns the NPC the app keeps for a creature in the campaign,
	// one per campaign and creature, made inside tx the first time (a minion with
	// the creature's basic sheet and average hit points, `combat_only`, never in
	// the master's list) and reused after. It is a combat character; the monsters
	// of a combat are copies of it. False for a key that is not an SRD creature.
	MonsterNpc(ctx context.Context, tx pgx.Tx, campaignID, masterUserID, monsterKey string, at time.Time) (link.Character, bool, error)
}

// DiceForce is what the campaign's dice setting makes a player do (RN-18).
type DiceForce int

const (
	// DiceChoice is the setting "each player chooses", so the player picks
	// on every roll, the app's or a typed result. Their saved preference is only
	// the default the screen highlights.
	DiceChoice DiceForce = iota
	// DiceForcedInApp makes everybody roll in the app, a typed value is refused.
	DiceForcedInApp
	// DiceForcedPhysical makes everybody roll real dice and types the result, the
	// app's roll is refused.
	DiceForcedPhysical
)

// refuses says whether the forced mode does not allow this way of rolling.
func (f DiceForce) refuses(inApp bool) bool {
	return (f == DiceForcedInApp && !inApp) || (f == DiceForcedPhysical && inApp)
}

// CombatDefaults tells what the table's rules say about a combat (RN-24): how it
// starts, what a critical hit does and who sees the death saves. cmd/api wires it
// to campaigns.Service. The rules are read, never cached, in the caller's own
// transaction (PR #121): a change of the rules applies from the next roll.
type CombatDefaults interface {
	// StoredTableRules returns the campaign's table rules: the defaults (the zero
	// value) when it never saved any. tx is the caller's open transaction, or nil
	// for a read outside one.
	StoredTableRules(ctx context.Context, tx pgx.Tx, campaignID string) (tablerules.Rules, error)
	// CombatWithoutMap says whether a combat started without a chosen mode is a
	// combat without a map (the table's rule "combate com mapa" is off). tx is the
	// caller's open transaction, or nil for a read.
	CombatWithoutMap(ctx context.Context, tx pgx.Tx, campaignID string) (bool, error)
}

// DiceModes tells what the campaign's dice setting forces on a player (RN-18).
// cmd/api wires it to campaigns.Service.CampaignDiceMode.
type DiceModes interface {
	// ForcedDice returns what the campaign's setting forces on userID, an
	// active member of the campaign.
	ForcedDice(ctx context.Context, tx pgx.Tx, campaignID, userID string) (DiceForce, error)
}

// CampaignDirectory tells which campaigns a user belongs to. The campaigns
// module implements it (campaigns.Service.ActiveCampaigns), so this
// package never reads campaign_members itself.
type CampaignDirectory interface {
	// ActiveCampaigns returns the campaigns userID is an active member of
	// (never a pending one), with their name and the user's role (my_role).
	ActiveCampaigns(ctx context.Context, userID string) ([]*campaignsv1.Campaign, error)
}

// The live stream's timing (docs/operations.md). Tests shorten them through
// Config.Live.
const (
	// DefaultHeartbeat: a stream with nothing to say sends a heartbeat this
	// often, so the app can tell a dead stream, and proxies and Cloud Run
	// don't take it for an idle one.
	DefaultHeartbeat = 25 * time.Second
	// DefaultRecheck: how often a stream reads the login session and the
	// membership again (ADR-0005).
	DefaultRecheck = 60 * time.Second
	// DefaultMaxLifetime: a stream ends after this long, and the app opens
	// a new one. It bounds what a forgotten tab can hold, and keeps each
	// stream well inside Cloud Run's request timeout.
	DefaultMaxLifetime = 30 * time.Minute
)

// LiveConfig sets the live stream's timing. Zero values mean the defaults.
type LiveConfig struct {
	Heartbeat   time.Duration
	Recheck     time.Duration
	MaxLifetime time.Duration
	// Buffer is how many events a stream may fall behind before it is
	// dropped (live.DefaultBuffer).
	Buffer int
}

// Config holds what the play service needs.
type Config struct {
	// Pool is the CockroachDB connection pool. Required.
	Pool *pgxpool.Pool
	// Sheets locks the players' sheets when a session starts. Required.
	Sheets SheetLocker
	// Vitals keeps the characters' vitals. Required.
	Vitals VitalsKeeper
	// Campaigns lists a user's campaigns, for the session notice. Required.
	Campaigns CampaignDirectory
	// Maps checks and reveals the session's current map, reads the image it
	// shows, and gives a combat its grid and tokens. Required.
	Maps MapKeeper
	// Roster says who can fight, for a combat. Required.
	Roster CombatRoster
	// Dice says where a player rolls (RN-18). Required.
	Dice DiceModes
	// Defaults gives the mode of a combat started without one (RN-24, RN-25).
	// Optional: nil means every such combat is on a map, as before the modes.
	Defaults CombatDefaults
	// Terrain gives a combat the walls, difficult terrain and cover of its map
	// (RN-21, D2). Optional: cmd/api sets it with SetTerrain once the maps module
	// exists (the two need each other); nil means open floor.
	Terrain TerrainSource
	// Roller rolls the NPCs' dice and the app's rolls. Nil means the
	// operating system's random source (dice.Crypto); tests pass faces.
	Roller dice.Roller
	// Live sets the live stream's timing; the zero value is the defaults.
	Live LiveConfig
	// Logger receives errors, without personal data. Nil means
	// slog.Default().
	Logger *slog.Logger
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
}

// Service implements the PlayService Connect API.
type Service struct {
	pool      *pgxpool.Pool
	queries   *playdb.Queries
	sheets    SheetLocker
	vitals    VitalsKeeper
	campaigns CampaignDirectory
	maps      MapKeeper
	roster    CombatRoster
	dice      DiceModes
	defaults  CombatDefaults
	terrain   TerrainSource
	doors     DoorKeeper // the maps module, when the terrain source is one (SetTerrain)
	fog       FogSource
	traps     TrapBook
	puzzles   puzzleDeps // the puzzles' maps seam and seed source (puzzles.go)
	// afterSightRead is a test hook: it runs between the moment a change reads the
	// sight and the moment it opens its transaction.
	afterSightRead func(encounterID string)
	// afterTrapRead is a test hook: it runs between the moment a firing outside a combat
	// reads where the tokens stand and the moment it opens its transaction.
	afterTrapRead func()
	roller        dice.Roller
	logger        *slog.Logger
	now           func() time.Time
	// conditionNames are the SRD conditions' Portuguese names, by key, for the
	// labels the master marks (RN-22).
	conditionNames map[string]string

	// hub fans the live events out to the open streams, in memory: one
	// server instance only (docs/operations.md).
	hub  *live.Hub
	live LiveConfig
}

// namerFor is the Portuguese name of a content key the combat shows, bound to one
// campaign's content: an SRD condition's, or any other key's (a spell's) from the
// content, which is read once for all the names.
func (s *Service) namerFor(ctx context.Context, tx pgx.Tx, campaignID string) (func(key string) string, error) {
	base, err := s.roster.ContentNames(ctx, tx, campaignID)
	if err != nil {
		return nil, err
	}
	return func(key string) string {
		if n, ok := s.conditionNames[key]; ok {
			return n
		}
		return base(key)
	}, nil
}

// namesFor is namerFor for the views, which name what they can: when the content
// cannot be read, the names are only the conditions' (the key is still shown),
// and the failure is logged once, with the error and no names or IDs.
func (s *Service) namesFor(ctx context.Context, campaignID string) func(key string) string {
	namer, err := s.namerFor(ctx, nil, campaignID)
	if err != nil {
		s.logger.WarnContext(ctx, "play: cannot read the content names", "error", err)
		return func(key string) string { return s.conditionNames[key] }
	}
	return namer
}

// The compiler checks that Service implements the handler.
var (
	_ playv1connect.PlayServiceHandler   = (*Service)(nil)
	_ playv1connect.CombatServiceHandler = (*Service)(nil)
	_ playv1connect.PuzzleServiceHandler = (*Service)(nil)
)

// New returns a Service.
func New(cfg Config) (*Service, error) {
	switch {
	case cfg.Pool == nil:
		return nil, errors.New("play: a Pool is required")
	case cfg.Sheets == nil:
		return nil, errors.New("play: Sheets is required")
	case cfg.Vitals == nil:
		return nil, errors.New("play: Vitals is required")
	case cfg.Campaigns == nil:
		return nil, errors.New("play: Campaigns is required")
	case cfg.Maps == nil:
		return nil, errors.New("play: Maps is required")
	case cfg.Roster == nil:
		return nil, errors.New("play: Roster is required")
	case cfg.Dice == nil:
		return nil, errors.New("play: Dice is required")
	}
	s := &Service{
		pool:      cfg.Pool,
		queries:   playdb.New(cfg.Pool),
		sheets:    cfg.Sheets,
		vitals:    cfg.Vitals,
		campaigns: cfg.Campaigns,
		maps:      cfg.Maps,
		roster:    cfg.Roster,
		dice:      cfg.Dice,
		defaults:  cfg.Defaults,
		terrain:   cfg.Terrain,
		doors:     doorKeeperOf(cfg.Terrain),
		roller:    cfg.Roller,
		logger:    cfg.Logger,
		now:       cfg.Now,
		hub:       live.New(cfg.Live.Buffer),
		live:      cfg.Live,
	}
	s.conditionNames = map[string]string{}
	for _, c := range cfg.Roster.Conditions() {
		s.conditionNames[c.Key] = c.NamePT
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.puzzles.hints.logger = s.logger
	if s.roller == nil {
		s.roller = dice.Crypto{}
	}
	if s.live.Heartbeat <= 0 {
		s.live.Heartbeat = DefaultHeartbeat
	}
	if s.live.Recheck <= 0 {
		s.live.Recheck = DefaultRecheck
	}
	if s.live.MaxLifetime <= 0 {
		s.live.MaxLifetime = DefaultMaxLifetime
	}
	return s, nil
}

// Close ends every open live stream, and refuses new ones. cmd/api calls it
// when the server's graceful shutdown starts: a stream never finishes on
// its own, so the shutdown would otherwise wait for its whole deadline.
// Each app reconnects, to the next server. Closing twice is fine.
func (s *Service) Close() { s.hub.Close() }

// Sessions is what this package needs to know who is calling: an
// interceptor that finds the caller's session, and the authz.Caller that
// reads it back and, for the live stream, reads the session again
// (authz.SessionRechecker). *identity.Service is the real one; tests pass
// a fake.
type Sessions interface {
	// Interceptor finds the caller's session (from the session cookie).
	Interceptor() connect.Interceptor
	authz.SessionRechecker
}

// Mount registers PlayService, CombatService, PuzzleService and EncounterService on a mux. handle is usually
// httpserver.Server.Handle or http.ServeMux.Handle.
//
// sessions tells who is calling (the identity service in production), and
// members tells each caller's role in a campaign (the campaigns service).
// Mount adds, in this order, an interceptor that marks every response
// `Cache-Control: no-store`, the sessions interceptor, and the authz
// interceptor. opts are the Connect options shared by every service.
func (s *Service) Mount(handle func(pattern string, handler http.Handler), sessions Sessions, members authz.MembershipSource, opts ...connect.HandlerOption) {
	// Clip so append copies instead of writing into the caller's array.
	opts = append(slices.Clip(opts), connect.WithInterceptors(
		nostore.Interceptor(),                          // no response is cacheable
		sessions.Interceptor(),                         // who is calling
		authz.Interceptor(sessions, members, s.logger), // what they may do, memoized per request
	))
	handle(playv1connect.NewPlayServiceHandler(s, opts...))
	handle(playv1connect.NewCombatServiceHandler(s, opts...))
	handle(playv1connect.NewPuzzleServiceHandler(s, opts...))
	handle(playv1connect.NewEncounterServiceHandler(s, opts...))
}

// queriesIn is the queries on the transaction, or on the pool when tx is nil. A
// read made while the caller holds a transaction must use the transaction: a
// read through the pool takes a second connection (see docs/architecture.md,
// "Dentro de uma transação, nenhuma leitura pelo pool").
func (s *Service) queriesIn(tx pgx.Tx) *playdb.Queries {
	if tx == nil {
		return s.queries
	}
	return s.queries.WithTx(tx)
}

// CheckWired fails when a collaborator that cmd/api connects after New is
// still nil (see platform/wiring). A nil fog source is the dangerous one: it
// means "no fog of war", and the players would see the NPCs the master hid.
func (s *Service) CheckWired() error {
	return wiring.Check("play",
		wiring.Dep{Setter: "SetTerrain", Missing: s.terrain == nil},
		wiring.Dep{Setter: "SetTerrain (a DoorKeeper)", Missing: s.doors == nil},
		wiring.Dep{Setter: "SetPuzzleMaps", Missing: s.puzzles.maps == nil},
		wiring.Dep{Setter: "SetFog", Missing: s.fog == nil},
		wiring.Dep{Setter: "SetTraps", Missing: s.traps == nil})
}
