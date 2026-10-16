/**
 * The view-models and the port of the session page (`/campaigns/:id/session`,
 * MR-011, MR-012, RN-02). Nothing here
 * imports generated code: the
 * `LiveSessionSourceLive` maps `PlayService`, `CharacterService` and
 * `CampaignService` onto these shapes, and the page, its children and their
 * tests only see these.
 */

import type { FamiliarSightVm } from './familiar-eyes';
import type { SearchSkills } from '../traps/trap-search';
import type { HitDieSize } from '../resources/hit-dice-text';
import type { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { CombatantMove, TurnChange } from '../combat/combat-state';
import type {
  PactSlotsVm,
  ResourceUsageVm,
  SlotUsageVm,
  WildShapeVm,
} from './resource-usage.types';

/** A player character's live numbers (`CharacterVitals`, RN-02). */
export interface VitalsVm {
  readonly characterId: string;
  readonly name: string;
  /** Empty when the player deleted their account (RN-16). */
  readonly playerUserId: string;
  readonly hitPointsCurrent: number;
  readonly hitPointsMax: number;
  readonly hitPointsTemporary: number;
  /** What Ajuda adds to the maximum above (0 or absent without it): `hitPointsMax` already counts it, the sheet's own is the difference. */
  readonly hitPointsMaxBonus?: number;
  /** The level of exhaustion, 0 to 6 (the master sets it); from level 4 `hitPointsMax` is already half the sheet's. Absent as 0. */
  readonly exhaustionLevel?: number;
  /** The base armor class an effect that lasts gives (Armadura Arcana: 13 + Destreza); absent or 0 without one. */
  readonly armorClassBase?: number;
  /** The combined level of slots Recuperação Arcana gives back (half the wizard level, rounded up); absent or 0 for no wizard level. */
  readonly arcaneRecoveryAllowance?: number;
  /** Only the levels with slots, lowest first. */
  readonly spellSlots: readonly SlotUsageVm[];
  readonly pactSlots: PactSlotsVm | null;
  /** The hit dice by size, "3d6", or "2d10 e 1d8" for a multiclass character (kept apart by size, SRD "Multiclassing"). */
  readonly hitDice: string;
  /** Each size with how many dice there are and how many are spent, largest die first (`hit_dice` with `hit_dice_used_by_die`). */
  readonly hitDiceSizes: readonly HitDieSize[];
  /** All the dice, of every size: the character's level. */
  readonly hitDiceTotal: number;
  /** All the dice spent, of every size. */
  readonly hitDiceUsed: number;
  /** Of two copies of the same character's vitals, the larger is newer. */
  readonly revision: number;
  /** The player looks through their familiar's eyes (MR-036); absent otherwise. */
  readonly familiarSight?: FamiliarSightVm | null;
  /** The uses of the character's resources (Forma Selvagem: "restam 2 de 2 usos"); absent for older copies. */
  readonly resources?: readonly ResourceUsageVm[];
  /** The beast a druid is in now; `null` or absent in the character's own shape. While it is set, the numbers above wait. */
  readonly wildShape?: WildShapeVm | null;
}

/** The open game session being watched. */
export interface LiveSessionVm {
  readonly sessionId: string;
  readonly sessionNumber: number;
  readonly startedAt: Date;
}

/** The gallery image the master shows the players (MR-028, `ShownImage`):
 * what a player's block needs, so the frame is reserved from the size
 * before the bytes arrive. */
export interface ShownImageVm {
  readonly id: string;
  /** The image's name in the gallery: the caption. */
  readonly name: string;
  readonly width: number;
  readonly height: number;
  readonly url: string;
}

/** `GetLiveSession`: the session and the vitals the caller may see (the
 * master: every living player character; a player: only their own), the
 * session's current map and the image being shown. */
export interface LiveSnapshotVm {
  readonly session: LiveSessionVm;
  readonly vitals: readonly VitalsVm[];
  /** The current map's ID, or `null` while the master has not chosen one. */
  readonly currentMapId: string | null;
  readonly shownImage: ShownImageVm | null;
  /** The master's "Deixar com os jogadores" switch for the image shown;
   * always false for a player (`shown_image_keep`). */
  readonly shownImageKeep: boolean;
}

/** One event of `WatchGameSession`. */
export type LiveEventVm =
  | { readonly kind: 'ready' }
  | { readonly kind: 'heartbeat' }
  | { readonly kind: 'vitals'; readonly vitals: VitalsVm }
  | { readonly kind: 'ended' }
  /** `content_changed` (RN-23, RN-10): the table's content changed; no content travels, each screen reads what it shows again. */
  | { readonly kind: 'contentChanged' }
  /** `puzzle_changed` (MR-038): a puzzle of the session changed; the hint names only its ID, the apps read it again. */
  | { readonly kind: 'puzzleChanged'; readonly puzzleId: string }
  /** `current_map_changed`; `mapId` is `null` when the map was cleared. */
  | { readonly kind: 'currentMap'; readonly mapId: string | null }
  /** `map_changed`: read the map again. */
  | { readonly kind: 'mapChanged'; readonly mapId: string }
  | {
      readonly kind: 'tokenMoved';
      readonly mapId: string;
      readonly characterId: string;
      readonly xBp: number;
      readonly yBp: number;
    }
  /** `vision_changed` (MR-036): what this player sees of a fog map changed; read the vision and the map again. */
  | { readonly kind: 'visionChanged'; readonly mapId: string }
  /** `shown_image_changed`; `image` is `null` when it stopped. */
  | { readonly kind: 'shownImage'; readonly image: ShownImageVm | null }
  /** `left_images_changed`: read the left images again. */
  | { readonly kind: 'leftImages' }
  /** `encounter_changed`: the combat changed, read it again if `revision`
   * is newer than the one on screen. */
  | {
      readonly kind: 'encounterChanged';
      readonly encounterId: string;
      readonly revision: number;
      readonly mode?: number;
    }
  /** `reaction_window_opened` (PM-04): a window waits for this player or for the master; read the combat again for the prompt. */
  | {
      readonly kind: 'reactionWindowOpened';
      readonly encounterId: string;
      readonly windowId: string;
    }
  /** `reaction_window_closed`: a window is no longer open. `text` is the reason when it closed by itself, written for this person. */
  | {
      readonly kind: 'reactionWindowClosed';
      readonly encounterId: string;
      readonly windowId: string;
      readonly closedByItself: boolean;
      readonly text: string;
    }
  /** `turn_changed`, as this member may see it. */
  | ({ readonly kind: 'turnChanged' } & TurnChange)
  /** `combatant_moved`. */
  | ({ readonly kind: 'combatantMoved' } & CombatantMove)
  /** `combat_log_changed`: read the combat log again. */
  | { readonly kind: 'combatLogChanged' }
  /** `xp_changed`: an award, an undo or a milestone; read the XP again. */
  | { readonly kind: 'xpChanged' }
  /** `scene_changed`: the open scene changed, read it again. */
  | { readonly kind: 'sceneChanged' }
  /** `scene_check_rolled`: a check was rolled in it, read it again. */
  | { readonly kind: 'sceneCheckRolled' }
  /** `notes_changed`: the master revealed a clue to this player; read the notes again. */
  | { readonly kind: 'notesChanged' }
  /** `stage_changed` (MR-031): an NPC came in or went out, or the speaker
   * changed. It names nobody: the page reads the open scene again. */
  | { readonly kind: 'stageChanged' }
  /** `trap_noticed` (MR-035): this player's character noticed a trap by passing near it. Only they get it. */
  | { readonly kind: 'trapNoticed'; readonly mapId: string; readonly pointId: string }
  /** `creatures_changed` (MR-037): the character's creatures changed outside a combat; read them again. */
  | { readonly kind: 'creaturesChanged' }
  /** `spell_casts_changed` (MR-048): the casts outside a combat changed; read them again. Carries no content. */
  | { readonly kind: 'spellCastsChanged' }
  /** `group_check_changed` (W7-X): a group check was asked, answered or closed; read it again as this member (the hint carries no content). */
  | { readonly kind: 'groupCheckChanged' }
  /** `content_changed` (10.1d): the table's content changed; read the catalog again with this member's role. */
  | { readonly kind: 'contentChanged' }
  /** `character_changes_requested` or `character_resubmitted`: that character changed; read it again. */
  | { readonly kind: 'characterChanged'; readonly characterId: string }
  /** `character_revived`: a dead character lives again; read it again, and the vitals. */
  | { readonly kind: 'characterRevived'; readonly characterId: string }
  /** `revivify_changed`: a Revivify cast outside a combat was asked or answered; the master reads the casts again. */
  | { readonly kind: 'revivifyChanged' }
  /** `calling_you`: the master nudged this player ("Chamar a atenção"). Carries nothing; the page alerts and shows the line. */
  | { readonly kind: 'callingYou' };

/**
 * What a failed call means for the page, from its Connect code and typed
 * detail (never from the message):
 * - `no-access`: `not_found` — not a member (or pending), or no such
 *   campaign; for an adjustment, the character isn't a living player
 *   character any more;
 * - `no-session`: `failed_precondition` with `NO_OPEN_SESSION`;
 * - `signed-out`: `unauthenticated`;
 * - `invalid`: `invalid_argument` (a value outside 0 to its maximum);
 * - `forbidden`: `permission_denied` (a player calling a master's method);
 * - `transient`: anything else (the network, `unavailable`, `aborted`):
 *   trying again may work.
 */
export type LiveErrorKind =
  'no-access' | 'no-session' | 'signed-out' | 'invalid' | 'forbidden' | 'transient';

/** `GetCampaign`, for the lead and to know who is looking. */
export interface CampaignInfoVm {
  readonly name: string;
  readonly isMaster: boolean;
  /** A pending member (RN-15): gets the same "Peça um convite" page. */
  readonly awaitingApproval: boolean;
  /** How the campaign's players roll dice (RN-18) and the caller's own
   * choice: the initiative screen offers the ways they allow. */
  readonly diceMode: DiceMode;
  readonly dicePreference: DicePreference;
}

/** From the player's own sheet (`GetCharacter`), what the vitals block
 * shows next to the live numbers. */
export interface PlayerSheetVm {
  /** `DerivedSheet.armor_class`; `null` for a sheet without it. */
  readonly armorClass: number | null;
  /** The bonuses in Percepção and Investigação and the other skills, for "Procurar armadilhas" and the checks; `undefined` for a sheet without skills. */
  readonly skills?: SearchSkills;
  /** "Mago 3, Gnomo das Rochas". */
  readonly summary: string;
  /** The classes alone, "Clérigo 5" (or "Clérigo 3 / Mago 2"); empty without them. */
  readonly classes?: string;
  /** Revivificar is on the sheet and ready today (prepared, or always prepared by the domain). */
  readonly revivify?: boolean;
  /** The character's senses, as the sheet says them in meters: "Visão no escuro: 18 m" (MR-036). */
  readonly senses: readonly string[];
}

/** From `ListCharacters`, what the master's "Grupo" row shows under a
 * character's name. */
export interface PartyMemberInfoVm {
  /** "Mago 3". */
  readonly classSummary: string;
  /** The player's display name, or `null` when they have none. */
  readonly playerName: string | null;
}

/** The master's correction: every value set replaces the current one. */
export interface VitalsChange {
  readonly hitPointsCurrent?: number;
  readonly hitPointsTemporary?: number;
  readonly spellSlotsUsed?: readonly { readonly level: number; readonly used: number }[];
  readonly pactSlotsUsed?: number;
  /** How many dice of each size are spent (die size to count), for the sizes that change. */
  readonly hitDiceUsedByDie?: Readonly<Record<number, number>>;
  /** The resources whose spent uses change (a master gives uses back). */
  readonly resourcesUsed?: readonly { readonly key: string; readonly used: number }[];
  /** The beast's hit points, for a druid in Wild Shape; 0 ends the form. */
  readonly wildShapeHitPointsCurrent?: number;
}

/**
 * The port the session page depends on, provided at the route level by
 * `LiveSessionSourceLive` (`live-session.routes.ts`), so the generated
 * clients stay in this page's lazy chunk.
 */
export abstract class LiveSessionSource {
  abstract getCampaign(campaignId: string): Promise<CampaignInfoVm>;
  /** `WatchGameSession`. Aborting `signal` closes the stream. */
  abstract watch(campaignId: string, signal: AbortSignal): AsyncIterable<LiveEventVm>;
  abstract getLiveSession(campaignId: string): Promise<LiveSnapshotVm>;
  abstract adjustVitals(
    campaignId: string,
    characterId: string,
    idempotencyKey: string,
    change: VitalsChange,
  ): Promise<VitalsVm>;
  abstract endSession(campaignId: string, sessionId: string): Promise<void>;
  /** `SetCurrentMap`: the map the session shows (`null` clears it). */
  abstract setCurrentMap(campaignId: string, mapId: string | null): Promise<string | null>;
  /** `SetShownImage`: show a gallery image to the players (`null` stops).
   * `keep` is the "Deixar com os jogadores" switch for that image. */
  abstract setShownImage(
    campaignId: string,
    imageId: string | null,
    keep?: boolean,
  ): Promise<ShownImageVm | null>;
  /** `ListLeftImages`: the images the master left with the players. */
  abstract listLeftImages(campaignId: string): Promise<readonly ShownImageVm[]>;
  /** `TakeBackLeftImage`: the master takes an image back ("Tirar"). */
  abstract takeBackLeftImage(campaignId: string, imageId: string): Promise<void>;
  abstract getPlayerSheet(campaignId: string, characterId: string): Promise<PlayerSheetVm>;
  /** The armor class of a creature's book (`GetCreature`), for a druid in a beast form; `null` when it cannot be read. */
  abstract getCreatureArmorClass(campaignId: string, key: string): Promise<number | null>;
  abstract getPartyInfo(campaignId: string): Promise<ReadonlyMap<string, PartyMemberInfoVm>>;
  abstract classifyError(err: unknown): LiveErrorKind;
}
