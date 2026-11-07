import type { RollDie } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { type Signal } from '@angular/core';
import type { MatBottomSheet } from '@angular/material/bottom-sheet';
import type { MatDialog } from '@angular/material/dialog';
import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  Combatant,
  Encounter,
  GetTurnOptionsResponse,
  HeldRollOffer,
  PendingDamage,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from '../../../core/combat/combat-client';
import type { CombatState } from '../../../core/combat/combat-state';
import type { SpellCatalog } from '../../../core/combat/spell-catalog';
import type { CreaturesClient } from '../../../core/creatures/creatures-client';
import type { MapState } from '../../../core/maps/map-state';
import type { MapLayers } from '../../../core/maps/layers';
import type { VitalsVm } from '../../../core/play/live-session.types';
import type { SearchSkills } from '../../../core/traps/trap-search';
import { type ThrownAttack } from './attack-sheet/attack-sheet-data';
import type { AttackSheetData } from './attack-sheet/attack-sheet-types';
import { type CastMapData } from './cast-sheet/cast-sheet';
import type { TargetFlow } from './target-mode/target-flow';

/** The sheet for a combatant of the Attack action: which attack, and what turns it into a reaction, a resume or a held roll. */
export interface AttackOpen {
  readonly key: string;
  /** A hit whose damage was never rolled: the sheet opens at the damage. */
  readonly resume?: PendingDamage;
  readonly asReaction?: boolean;
  /** One of the player's creatures (or the master's NPC) attacks, with its own `GetTurnOptions`. */
  readonly who?: Combatant;
  readonly creatureOpts?: GetTurnOptionsResponse | null;
  readonly held?: HeldRollOffer;
  readonly catchWindowId?: string;
  readonly useExtraAction?: boolean;
  readonly thrown?: ThrownAttack;
  /** The master's legendary attack of a creature off its turn: see `AttackSheetData.legendary`. */
  readonly legendary?: AttackSheetData['legendary'];
}

/** What `CombatSheets` reads from the screen it serves. */
export interface CombatSheetsContext {
  readonly dialog: MatDialog;
  readonly bottomSheet: MatBottomSheet;
  readonly api: Pick<CombatClient, 'attacks' | 'moves'>;
  readonly creatures: Pick<CreaturesClient, 'summonOptions'>;
  readonly catalog: Pick<SpellCatalog, 'details'>;
  readonly campaignId: () => string;
  readonly encounter: () => Encounter | null;
  readonly own: () => Combatant | null;
  readonly isMaster: () => boolean;
  readonly state: () => CombatState;
  readonly diceMode: () => DiceMode;
  readonly preference: () => DicePreference;
  /** The player's `GetTurnOptions`. */
  readonly options: () => GetTurnOptionsResponse | null;
  readonly vitals: () => readonly VitalsVm[];
  readonly ownVitals: Signal<VitalsVm | null>;
  readonly casterClasses: () => string;
  /** The table dice the character may add to a save (`DerivedSheet.roll_dice`). */
  readonly rollDice?: () => readonly RollDie[];
  readonly armorClass: () => number | null;
  readonly trapSkills: () => SearchSkills | null;
  readonly mapState: () => MapState;
  /** The battle map: the picture, the grid and the layers an area spell is placed on; `null` for the theatre of the mind. */
  readonly image: () => CastMapData['image'] | null;
  readonly theatre: () => boolean;
  readonly mapName: () => string;
  readonly layers: () => MapLayers;
  readonly fogVision: () => CastMapData['fog'];
  /** The player's choice of a target on the map (C7-04); absent where the sheets keep their own list. */
  readonly target?: () => TargetFlow;
  /** The "Gastar movimento" sheet closed: the move note it replaced goes. */
  readonly spendClosed: () => void;
}
