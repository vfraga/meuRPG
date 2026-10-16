import { type Signal, signal, untracked } from '@angular/core';
import type { MatBottomSheet } from '@angular/material/bottom-sheet';
import type { MatDialog } from '@angular/material/dialog';

import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  Combatant,
  Encounter,
  GetTurnOptionsResponse,
  OpportunityOffer,
  PendingDamage,
  ReactionWindow,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from '../../../core/combat/combat-client';
import type { CombatState } from '../../../core/combat/combat-state';
import { currentCombatant } from '../../../core/combat/combat-view';
import { reactorAttacks } from '../../../core/combat/opportunity';
import { openWindows, promptView } from '../../../core/combat/reactions';
import type { SpellCatalog } from '../../../core/combat/spell-catalog';
import type { CreaturesClient } from '../../../core/creatures/creatures-client';
import { wallUnderDoor } from '../../../core/maps/door-paint';
import type { DoorSquare } from '../../../core/maps/layers';
import type { MapState } from '../../../core/maps/map-state';
import type { MapLayers } from '../../../core/maps/layers';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { isRevivify } from '../../../core/revivify/revivify-flow';
import { resourceFlow } from '../../../core/resources/resource-actions';
import type { SearchSkills } from '../../../core/traps/trap-search';
import { openFamiliarEyes } from '../../../shared/familiar-eyes/familiar-eyes-sheet';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { openSpellDetails } from '../../../shared/spell-details/open-spell-details';
import type { SpellDetailsData } from '../../../shared/spell-details/spell-details';
import { spellDetailsFromGen } from '../../../shared/spell-details/spell-details-map';
import { SummonSheet, type SummonSheetData } from '../../../shared/summon-sheet/summon-sheet';
import { openWildShape } from '../../../shared/wild-shape/wild-shape-sheet';
import { type DoorSheetData, openDoorSheet } from '../door-sheet/door-sheet';
import {
  EffectSaveSheet,
  type EffectSaveSheetData,
} from '../effects/effect-save-sheet/effect-save-sheet';
import { openTrapSearch } from '../traps/trap-search-sheet/trap-search-sheet';
import { ActionEconomy } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { AdjustNpc, type AdjustNpcData } from './adjust-npc/adjust-npc';
import { AttackSheet, type AttackSheetData } from './attack-sheet/attack-sheet';
import {
  type HeldRoll,
  type ThrownAttack,
  attackAriaLabel,
  attackSheetData,
  throwBackAttack,
  THROW_BACK_KEY,
} from './attack-sheet/attack-sheet-data';
import type { MasterAreaCastData } from './area-cast/master-area-cast';
import { MasterAreaCast } from './area-cast/master-area-cast';
import { CastSheet, type CastMapData, type CastSheetData } from './cast-sheet/cast-sheet';
import {
  areaSpellOf,
  castAriaLabel,
  castSheetData,
  masterAreaCastData,
  masterAreaFocus,
  opensOnMap,
} from './cast-sheet/cast-sheet-data';
import { ConditionsDialog, type ConditionsData } from './conditions-dialog/conditions-dialog';
import { FeatureSheet, type FeatureSheetData } from './feature-sheet/feature-sheet';
import {
  OpportunitySheet,
  type OpportunityAnswer,
  type OpportunitySheetData,
} from './opportunity/opportunity-sheet';
import {
  ReactionSheet,
  type ReactionSheetData,
  type ReactionSheetResult,
} from './reaction-sheet/reaction-sheet';
import { openRevivifySheet } from './revivify-sheet/revivify-sheet';
import { openBardicInspiration, openFlexibleCasting, openLayOnHands } from './resource-sheets';
import { SpendSheet, type SpendSheetData } from './theatre/spend-sheet';

/** The sheet for a combatant of the Attack action: which attack, and what turns it into a reaction, a resume or a held roll. */
export interface AttackOpen {
  readonly key: string;
  /** A hit whose damage was never rolled: the sheet opens at the damage. */
  readonly resume?: PendingDamage;
  readonly asReaction?: boolean;
  /** One of the player's creatures (or the master's NPC) attacks, with its own `GetTurnOptions`. */
  readonly who?: Combatant;
  readonly creatureOpts?: GetTurnOptionsResponse | null;
  readonly held?: HeldRoll;
  readonly catchWindowId?: string;
  readonly useExtraAction?: boolean;
  readonly thrown?: ThrownAttack;
}

/** What `CombatSheets` reads from the screen it serves. */
export interface CombatSheetsContext {
  readonly dialog: MatDialog;
  readonly bottomSheet: MatBottomSheet;
  readonly api: Pick<CombatClient, 'turnOptions' | 'answerReaction'>;
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
  readonly armorClass: () => number | null;
  readonly trapSkills: () => SearchSkills | null;
  readonly mapState: () => MapState;
  /** The battle map: the picture, the grid and the layers an area spell is placed on; `null` for the theatre of the mind. */
  readonly image: () => CastMapData['image'] | null;
  readonly theatre: () => boolean;
  readonly mapName: () => string;
  readonly layers: () => MapLayers;
  readonly fogVision: () => CastMapData['fog'];
  /** The "Gastar movimento" sheet closed: the move note it replaced goes. */
  readonly spendClosed: () => void;
}

/**
 * Every sheet and dialog the combat screen opens for the player or the master (the attack, the cast, the reactions, the
 * opportunity attacks, the conditions, the door...): what each is handed is built by pure functions next to the sheet
 * (`attack-sheet-data.ts`, `cast-sheet-data.ts`), and this class only reads the screen and opens it. It also keeps what
 * the screen must know about the sheets that are open: the attack sheet (a held roll's question waits for it), the reaction
 * sheet (no second one opens over it) and the windows and held rolls whose sheet already opened by itself.
 */
export class CombatSheets {
  /** The attack sheet is on screen: it is the one that gets the answer of its own roll. */
  readonly attackOpen = signal(false);
  /** The reaction sheet is on screen: no second one opens over it. */
  readonly reactionOpen = signal(false);
  /** The reaction windows whose sheet already opened by itself: it opens once for each, never again after it closed. */
  readonly reactionHandled = new Set<string>();
  /** The held rolls the page already opened its question for: a roll's own sheet registers its hold, so the reading
   * of the screen does not open the question a second time. */
  readonly inspirationHandled = new Set<string>();
  /** What the player's combatants say a summoning spell is: the keys `GetSummonOptions` lists, read once for the character. */
  private summonSpells: Promise<ReadonlySet<string>> | null = null;

  constructor(private readonly ctx: CombatSheetsContext) {}

  /** "Atacar": the attack sheet (Alvo, Rolar, Dano) for the character, a creature of theirs or the master's NPC. */
  attack(req: AttackOpen): void {
    const e = this.ctx.encounter();
    const attacker = req.who ?? this.ctx.own();
    if (!e || !attacker) {
      return;
    }
    const data = attackSheetData({
      key: req.key,
      campaignId: this.ctx.campaignId(),
      encounter: e,
      attacker,
      options: req.who ? req.creatureOpts : this.ctx.options(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.state(),
      master: this.ctx.isMaster(),
      asReaction: req.asReaction ?? false,
      useExtraAction: req.useExtraAction ?? false,
      catchWindowId: req.catchWindowId ?? '',
      resume: req.resume,
      held: req.held,
      thrown: req.thrown,
      onHeld: (holdId) => this.inspirationHandled.add(holdId),
    });
    if (!data) {
      return;
    }
    this.attackOpen.set(true);
    openSheet<AttackSheet, AttackSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      AttackSheet,
      {
        data,
        ariaLabel: attackAriaLabel(data.attack, req.asReaction ?? false, req.who),
        labelledBy: 'sheet-t',
      },
    ).subscribe({
      complete: () => {
        this.attackOpen.set(false);
        if (req.catchWindowId) {
          this.keepMissile(req.catchWindowId);
        }
      },
      error: () => this.attackOpen.set(false),
    });
  }

  /** The master taps a door of the map: its sheet opens (open, close, lock, or reveal a secret door). The map reads itself again on the stream. */
  door(door: DoorSquare): void {
    const e = this.ctx.encounter();
    if (!e) {
      return;
    }
    const layers = this.ctx.layers();
    const data: DoorSheetData = {
      campaignId: this.ctx.campaignId(),
      mapId: e.mapId,
      door,
      wallSquares: wallUnderDoor(
        layers.walls,
        layers.columns,
        layers.rows,
        this.ctx.mapState().map()?.squareFactor ?? 1,
        door,
      ),
    };
    openDoorSheet(this.ctx.dialog, this.ctx.bottomSheet, data).subscribe();
  }

  /** The "?" of a spell: its description (the SRD's, read once through the catalog), a sheet on a phone. */
  describeSpell(key: string, name: string): void {
    openSpellDetails(this.ctx.dialog, this.ctx.bottomSheet, this.spellDetailsData(key, name));
  }

  /** What the details sheet needs: the name now, and the text when it loads. */
  private spellDetailsData(key: string, name: string): SpellDetailsData {
    const campaignId = this.ctx.campaignId();
    return {
      namePt: name,
      load: async () => {
        const details = await this.ctx.catalog.details(campaignId, key);
        if (!details) {
          throw new Error('spell details unavailable');
        }
        return spellDetailsFromGen(details);
      },
    };
  }

  /** "Conjurar": the cast sheet (the slot, the targets, the roll, the damage); a summoning spell asks for the creatures instead. */
  async cast(key: string): Promise<void> {
    const spell = this.ctx.options()?.options?.spells.find((s) => s.spell?.key === key);
    if (spell?.spell && (await this.isSummon(key))) {
      this.summon(key, spell.spell.namePt || spell.spell.name);
      return;
    }
    if (isRevivify(key)) {
      this.revivify();
      return;
    }
    this.castSheet(key);
  }

  /** Revivificar has its own sheet (who died nearby, the diamonds, the result), not the target picker. */
  private revivify(): void {
    const e = this.ctx.encounter();
    const own = this.ctx.own();
    if (!e || !own) {
      return;
    }
    openRevivifySheet(this.ctx.dialog, this.ctx.bottomSheet, {
      campaignId: this.ctx.campaignId(),
      casterName: own.label,
      classes: this.ctx.casterClasses(),
      combat: { encounterId: e.id, casterId: own.id, round: e.round, state: this.ctx.state() },
      casterCharacterId: own.characterId,
      reload: signal(0),
    }).subscribe();
  }

  /** The cast sheet for a spell of the options, or a cantrip that asks for a saving throw; with `resume` it opens at the
   * damage the player never rolled. */
  castSheet(key: string, resume?: readonly PendingDamage[]): void {
    const e = this.ctx.encounter();
    const caster = this.ctx.own();
    const turn = this.ctx.options();
    if (!e || !caster || !turn?.options) {
      return;
    }
    const data: CastSheetData = castSheetData({
      key,
      campaignId: this.ctx.campaignId(),
      encounter: e,
      caster,
      turn: { ...turn, options: turn.options },
      vitals: this.ctx.vitals().find((v) => v.characterId === caster.characterId),
      master: this.ctx.isMaster(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.state(),
      resume,
      map: this.castMap(),
    });
    openSheet<CastSheet, CastSheetData, boolean>(this.ctx.dialog, this.ctx.bottomSheet, CastSheet, {
      data,
      ariaLabel: castAriaLabel(data.name, !!resume),
      labelledBy: 'sheet-t',
      ...(opensOnMap(data) ? { focus: '[role="application"]' } : {}),
    }).subscribe();
  }

  /** The battle map an area spell is placed on: the picture, the grid, the layers and, for a player, what the fog lets them
   * see. `null` without a map (theatre of the mind), where every area spell keeps its list. */
  private castMap(): CastMapData | null {
    const e = this.ctx.encounter();
    const img = this.ctx.image();
    if (!e || !img || this.ctx.theatre()) {
      return null;
    }
    return {
      image: img,
      mapName: this.ctx.mapName(),
      columns: e.gridColumns,
      rows: e.gridRows,
      layers: this.ctx.layers(),
      fog: this.ctx.fogVision(),
    };
  }

  /** "Conjurar" on an NPC's area spell: the master's picker in a dialog (PM-02d state 11), the same steps as the player's. */
  masterArea(key: string): void {
    const e = this.ctx.encounter();
    const who = e ? currentCombatant(e) : null;
    const found = areaSpellOf(this.ctx.options(), key);
    const map = this.castMap();
    if (!e || !who || !found || !map) {
      return;
    }
    const data: MasterAreaCastData = masterAreaCastData({
      campaignId: this.ctx.campaignId(),
      encounter: e,
      caster: who,
      key,
      found,
      map,
      state: this.ctx.state(),
    });
    openSheet<MasterAreaCast, MasterAreaCastData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      MasterAreaCast,
      {
        data,
        ariaLabel: `${who.label} conjura ${found.name}`,
        labelledBy: 'sheet-t',
        width: '1100px',
        tall: true,
        ...masterAreaFocus(found.targets),
      },
    ).subscribe();
  }

  private isSummon(key: string): Promise<boolean> {
    const own = this.ctx.own();
    if (!own) {
      return Promise.resolve(false);
    }
    // A failed read is not kept: the next cast asks again, so one error never hides Conjurar Animais for the whole session.
    this.summonSpells ??= this.ctx.creatures
      .summonOptions(this.ctx.campaignId(), own.characterId)
      .then(
        (options) => new Set(options.spells.map((s) => s.spellKey)),
        (err: unknown) => {
          this.summonSpells = null;
          throw err;
        },
      );
    return this.summonSpells.then(
      (keys) => keys.has(key),
      () => false,
    );
  }

  /** Conjurar Animais in a combat: the choices of the creatures and the group's one initiative roll (E9-12). */
  private summon(key: string, name: string): void {
    const e = this.ctx.encounter();
    const own = this.ctx.own();
    if (!e || !own) {
      return;
    }
    const data: SummonSheetData = {
      campaignId: this.ctx.campaignId(),
      characterId: own.characterId,
      spellKey: key,
      combat: {
        encounterId: e.id,
        casterId: own.id,
        diceMode: this.ctx.diceMode(),
        preference: this.ctx.preference(),
        state: this.ctx.state(),
        concentrating: own.concentrationSpellNamePt,
      },
    };
    openSheet<SummonSheet, SummonSheetData, unknown>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      SummonSheet,
      {
        data,
        ariaLabel: `Conjurar ${name}`,
        labelledBy: 'summon-t',
        width: '560px',
      },
    ).subscribe();
  }

  /** "Condições…" in a combatant's menu (the master). */
  conditions(combatantId: string): void {
    const e = this.ctx.encounter();
    const combatant = e?.combatants.find((c) => c.id === combatantId);
    if (!e || !combatant) {
      return;
    }
    const data: ConditionsData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatant,
      state: this.ctx.state(),
    };
    openSheet<ConditionsDialog, ConditionsData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      ConditionsDialog,
      {
        data,
        ariaLabel: `Condições de ${combatant.label}`,
        labelledBy: 'sheet-t',
        width: '600px',
      },
    ).subscribe();
  }

  /** "Você foi atingido: usar Escudo?" and the other reactions: opens by itself, and has to be answered. */
  reaction(window: ReactionWindow): void {
    const e = this.ctx.encounter();
    const own = this.ctx.own();
    const vitals = this.ctx.vitals().find((v) => v.characterId === own?.characterId);
    if (!e) {
      return;
    }
    if (window.prompt.case === 'effectSave') {
      this.effectSave(window);
      return;
    }
    const data: ReactionSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      window,
      round: e.round,
      usage: vitals?.spellSlots ?? [],
      pact: vitals?.pactSlots ?? null,
      state: this.ctx.state(),
      armorClass: this.ctx.armorClass(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
    };
    this.reactionOpen.set(true);
    openSheet<ReactionSheet, ReactionSheetData, ReactionSheetResult>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      ReactionSheet,
      {
        data,
        ariaLabel: promptView(window, e.round)?.ariaLabel ?? 'Reação',
        alert: true,
      },
    ).subscribe({
      next: (result) => {
        if (result?.throwWindowId) {
          this.reactionHandled.add(result.throwWindowId);
          this.throwBack(result.throwWindowId);
        }
      },
      complete: () => this.reactionOpen.set(false),
    });
  }

  /** The saving throw an effect asks at the end or the start of this player's turn (RN-22): opens by itself, and again from "Rolar o teste". */
  effectSave(window: ReactionWindow): void {
    const e = this.ctx.encounter();
    if (!e || this.reactionOpen()) {
      return;
    }
    const data: EffectSaveSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      window,
      round: e.round,
      state: this.ctx.state(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
    };
    this.reactionOpen.set(true);
    openSheet<EffectSaveSheet, EffectSaveSheetData, void>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      EffectSaveSheet,
      { data, ariaLabel: 'Teste de resistência do fim do turno', labelledBy: 'sheet-t' },
    ).subscribe({ complete: () => this.reactionOpen.set(false) });
  }

  /**
   * "Devolver (1 de chi)": the ranged attack with the missile the monk caught, part of the same reaction (see
   * `throwBackAttack`). Closing the sheet without the roll keeps the missile (the window is passed), so the combat never
   * waits on it.
   */
  private throwBack(windowId: string): void {
    const window = this.ctx.encounter()?.reactionWindows.find((x) => x.id === windowId);
    const thrown = throwBackAttack(window, this.ctx.options());
    if (thrown) {
      this.attack({
        key: THROW_BACK_KEY,
        asReaction: true,
        catchWindowId: windowId,
        thrown,
      });
    }
  }

  /** The throw-back sheet closed without a roll: the missile is kept, so the window does not hang. */
  private keepMissile(windowId: string): void {
    const e = this.ctx.encounter();
    if (!e || !openWindows(e).some((w) => w.id === windowId)) {
      return;
    }
    const answer = { use: false };
    void this.ctx.api
      .answerReaction(this.ctx.campaignId(), e.id, windowId, answer, `keep-${windowId}`)
      .then((r) => this.ctx.state().apply(r.encounter))
      .catch(() => undefined);
  }

  /** "Dano/Cura" on an NPC (`AdjustCombatantHitPoints`). */
  adjustNpc(combatantId: string): void {
    const e = this.ctx.encounter();
    const combatant = e?.combatants.find((c) => c.id === combatantId);
    if (!e || !combatant) {
      return;
    }
    openSheet<AdjustNpc, AdjustNpcData, boolean>(this.ctx.dialog, this.ctx.bottomSheet, AdjustNpc, {
      data: {
        campaignId: this.ctx.campaignId(),
        encounterId: e.id,
        combatant,
        state: this.ctx.state(),
      },
      ariaLabel: `Dano ou cura em ${combatant.label}`,
      labelledBy: 'adjust-npc-title',
      width: '440px',
    }).subscribe();
  }

  /** The player's prompt: "O Goblin 2 está saindo do seu alcance. Ataque de oportunidade?" It reads the
   * reactor's attacks itself (its buttons are off until they are in), so an offer is never stuck on a failed read. */
  opportunity(offer: OpportunityOffer): void {
    const e = this.ctx.encounter();
    if (!e) {
      return;
    }
    const data: OpportunitySheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      offer,
      round: e.round,
      load: async () => {
        const options = await this.ctx.api.turnOptions(
          this.ctx.campaignId(),
          e.id,
          offer.reactorId,
        );
        return {
          options,
          attacks: reactorAttacks(
            offer,
            (options.options?.attacks ?? []).flatMap((a) => (a.attack ? [a.attack] : [])),
          ),
        };
      },
      state: this.ctx.state(),
    };
    openSheet<OpportunitySheet, OpportunitySheetData, OpportunityAnswer>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      OpportunitySheet,
      {
        data,
        ariaLabel: 'Ataque de oportunidade',
        alert: true,
      },
    ).subscribe((answer) => {
      if (answer) {
        this.opportunityAttack(offer, answer.attackKey, answer.options, false);
      }
    });
  }

  /** The offer is still pending after the attack sheet closed (nobody rolled): ask again, it must be answered. */
  private rearm(offer: OpportunityOffer): void {
    const still =
      this.ctx.encounter()?.opportunityOffers.some((o) => o.id === offer.id && o.forYou) ?? false;
    if (still && !this.ctx.isMaster()) {
      untracked(() => this.opportunity(offer));
    }
  }

  /** "Atacar com <arma>": the attack sheet, with the mover as its target. */
  opportunityAttack(
    offer: OpportunityOffer,
    key: string,
    options: GetTurnOptionsResponse | null | undefined,
    byMaster: boolean,
  ): void {
    const e = this.ctx.encounter();
    const attack = options?.options?.attacks.find((a) => a.attack?.key === key)?.attack;
    if (!e || !attack) {
      return;
    }
    const data: AttackSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      attackerId: offer.reactorId,
      round: e.round,
      attack,
      targets: [],
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.state(),
      asReaction: true,
      opportunity: {
        offerId: offer.id,
        targetId: offer.moverId,
        targetLabel: offer.moverLabel,
        byMaster,
      },
    };
    openSheet<AttackSheet, AttackSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      AttackSheet,
      {
        data,
        ariaLabel: `Ataque de oportunidade com ${attack.namePt || attack.name}`,
        labelledBy: 'sheet-t',
      },
    ).subscribe(() => this.rearm(offer));
  }

  /** "Gastar movimento" (a combat without a map): the sheet that asks how far. For the player's character, or for one of their creatures. */
  spend(id?: string): void {
    const e = this.ctx.encounter();
    const who = id ?? this.ctx.own()?.id;
    if (!e || !who) {
      return;
    }
    const data: SpendSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatantId: who,
      state: this.ctx.state(),
    };
    openSheet<SpendSheet, SpendSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      SpendSheet,
      {
        data,
        ariaLabel: 'Gastar movimento',
        labelledBy: 'sheet-t',
      },
    ).subscribe(() => this.ctx.spendClosed());
  }

  /** The search for traps (E9-08): it spends the action through `SearchForTraps`. */
  searchTraps(): void {
    openTrapSearch(this.ctx.dialog, this.ctx.bottomSheet, {
      campaignId: this.ctx.campaignId(),
      skills: this.ctx.trapSkills(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.mapState(),
      inCombat: true,
    }).subscribe();
  }

  /** "Retomar o Fôlego": the feature that rolls on its own sheet. */
  secondWind(own: Combatant, e: Encounter, name: string, economy: ActionEconomy): void {
    const data: FeatureSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      combatantId: own.id,
      actionKey: 'feature:second-wind',
      name,
      cost: economy === ActionEconomy.BONUS_ACTION ? 'Ação bônus' : 'Ação',
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.state(),
    };
    openSheet<FeatureSheet, FeatureSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      FeatureSheet,
      { data, ariaLabel: name, labelledBy: 'sheet-t' },
    ).subscribe();
  }

  /** The dialog of a class resource (Cura pelas Mãos, Conjuração Flexível, Inspiração de Bardo): `TakeAction` refuses
   * these actions, `ResourceService` runs them. */
  resource(
    flow: NonNullable<ReturnType<typeof resourceFlow>>,
    key: string,
    actorId: string,
    encounterId: string,
  ): void {
    const base = {
      campaignId: this.ctx.campaignId(),
      encounterId,
      actorId,
      vitals: this.ctx.ownVitals,
      state: this.ctx.state(),
    };
    const targets =
      this.ctx.options()?.resourceTargets.find((t) => t.actionKey === key)?.targets ?? [];
    switch (flow) {
      case 'lay-on-hands':
        openLayOnHands(this.ctx.dialog, this.ctx.bottomSheet, { ...base, targets }).subscribe();
        break;
      case 'bardic-give':
        openBardicInspiration(this.ctx.dialog, this.ctx.bottomSheet, {
          ...base,
          targets,
        }).subscribe();
        break;
      default:
        openFlexibleCasting(this.ctx.dialog, this.ctx.bottomSheet, {
          ...base,
          direction: flow === 'flexible-create' ? 'create' : 'convert',
        }).subscribe();
    }
  }

  /** "Ver pelos olhos do Nanquim": the question first; the stream brings the band, the action spent and the blind line. */
  familiarEyes(familiarName: string): void {
    const mine = this.ctx.own();
    if (!mine) {
      return;
    }
    openFamiliarEyes(this.ctx.dialog, this.ctx.bottomSheet, {
      campaignId: this.ctx.campaignId(),
      characterId: mine.characterId,
      characterName: mine.label,
      familiarName,
      inCombat: true,
    }).subscribe();
  }

  /** "Transformar": the list of beasts the level allows. */
  wildShape(): void {
    const own = this.ctx.own();
    if (!own) {
      return;
    }
    const resource = this.ctx.ownVitals()?.resources?.find((r) => r.key === 'wild_shape');
    openWildShape(this.ctx.dialog, this.ctx.bottomSheet, {
      campaignId: this.ctx.campaignId(),
      characterId: own.characterId,
      inCombat: true,
      uses: resource ? { left: resource.total - resource.used, total: resource.total } : null,
    }).subscribe((result) => {
      if (result?.encounter) {
        this.ctx.state().apply(result.encounter);
      }
    });
  }
}
