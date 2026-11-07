import { signal } from '@angular/core';
import type { PendingDamage } from '../../../../gen/meurpg/play/v1/combat_pb';
import { currentCombatant } from '../../../core/combat/combat-view';
import { isRevivify } from '../../../core/revivify/revivify-flow';
import { openSheet, trackOpen } from '../../../shared/sheet/sheet-host';
import { SummonSheet, type SummonSheetData } from '../../../shared/summon-sheet/summon-sheet';
import { AttackSheet, type AttackSheetData } from './attack-sheet/attack-sheet';
import { attackAriaLabel, attackSheetData } from './attack-sheet/attack-sheet-data';
import type { MasterAreaCastData } from './area-cast/master-area-cast';
import {
  type CreatureActionOutcome,
  MasterCreatureArea,
  type MasterCreatureAreaData,
} from './area-cast/master-creature-area';
import type { CreatureAreaRequest } from './npc-card/creature-actions';
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
import { ZoneMoveSheet, zoneMoveTitle, type ZoneMoveSheetData } from './zone-move/zone-move-sheet';
import { spellKind } from '../../../core/combat/cast-flow';
import { castRefOfKey, optionOfRef, splitCastRef } from '../../../core/combat/cast-ref';
import type { CastPreset } from './target-mode/target-flow';
import { openRevivifySheet } from './revivify-sheet/revivify-sheet';
import { ActionSheets } from './action-sheets';
import type { AttackOpen, CombatSheetsContext } from './combat-sheets-context';
import { ReactionSheets } from './reaction-sheets';

/** The player's own attack of a turn, which has a target to tap on the map: not a creature's, a reaction, a damage or a held roll's. */
function aimsOnMap(req: AttackOpen): boolean {
  return !req.who && !req.asReaction && !req.resume && !req.held && !req.thrown;
}

/**
 * Every sheet and dialog the combat screen opens for the player or the master. What each is handed is built by pure
 * functions next to the sheet (`attack-sheet-data.ts`, `cast-sheet-data.ts`); this class opens the attack and the cast
 * sheets, and the other sheets live in the classes it builds: `ReactionSheets` (the reactions and the opportunity
 * attacks) and `ActionSheets` (the dialogs of one purpose). It keeps what the screen must know about the attack sheet
 * (a held roll's question waits for it) and the held rolls whose sheet already opened by itself.
 */
export class CombatSheets {
  /** The attack sheet is on screen: it is the one that gets the answer of its own roll. */
  readonly attackOpen = signal(false);
  /** The held rolls the page already opened its question for: a roll's own sheet registers its hold, so the reading
   * of the screen does not open the question a second time. */
  readonly inspirationHandled = new Set<string>();
  /** The summon options `GetSummonOptions` lists, as cast refs (the spell and the feature that grants it), read once for the character. */
  private summonSpells: Promise<ReadonlySet<string>> | null = null;
  /** The sheets that answer a question of the combat: the reactions and the opportunity attacks. */
  readonly reactions: ReactionSheets;
  /** The dialogs of one purpose (the door, the conditions, the resources...). */
  readonly actions: ActionSheets;

  constructor(private readonly ctx: CombatSheetsContext) {
    this.reactions = new ReactionSheets(ctx, (req) => this.attack(req));
    this.actions = new ActionSheets(ctx);
  }

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
      legendary: req.legendary,
      onHeld: (holdId) => this.inspirationHandled.add(holdId),
    });
    if (!data) {
      return;
    }
    // The player's own attack on a turn: the target is tapped on the map first (C7-04); any other opens the sheet at once.
    if (
      aimsOnMap(req) &&
      this.ctx.target?.()?.attack(data, (target) => this.openAttack(req, data, target))
    ) {
      return;
    }
    this.openAttack(req, data, null);
  }

  /** The attack sheet, at the roll when the target is already named. */
  private openAttack(req: AttackOpen, data: AttackSheetData, target: string | null): void {
    trackOpen(
      this.attackOpen,
      openSheet<AttackSheet, AttackSheetData, boolean>(
        this.ctx.dialog,
        this.ctx.bottomSheet,
        AttackSheet,
        {
          data: target ? { ...data, target } : data,
          ariaLabel: attackAriaLabel(data.attack, req.asReaction ?? false, req.who),
          labelledBy: 'sheet-t',
        },
      ),
      {
        complete: () => {
          if (req.catchWindowId) {
            this.reactions.keepMissile(req.catchWindowId);
          }
        },
      },
    );
  }

  /** "Conjurar": the cast sheet (the slot, the targets, the roll, the damage); a summoning spell asks for the creatures instead. */
  async cast(ref: string): Promise<void> {
    const { key, grantedBy } = splitCastRef(ref);
    const spell = optionOfRef(this.ctx.options()?.options?.spells ?? [], ref);
    // A summoning spell (the classes' or a feature's) and Revivificar have their own sheets.
    if (spell?.spell && (await this.isSummon(key, grantedBy))) {
      this.summon(key, spell.spell.namePt || spell.spell.name);
      return;
    }
    if (grantedBy === '' && isRevivify(key)) {
      this.revivify();
      return;
    }
    if (!(await this.castOnMap(ref))) {
      this.castSheet(ref);
    }
  }

  /** The player's spell with creatures to aim at: they are tapped on the map first (C7-04). `false` when the sheet opens as it
   * always did (the master, no map, an area, a spell on the caster). */
  private async castOnMap(ref: string): Promise<boolean> {
    const { key } = splitCastRef(ref);
    const flow = this.ctx.target?.();
    // The master picks on the map only for the character he acts for (`own` is that one).
    const data = flow && (!this.ctx.isMaster() || this.ctx.own()) ? this.castData(ref) : null;
    if (!flow || !data) {
      return false;
    }
    const details = await this.ctx.catalog.details(this.ctx.campaignId(), key);
    return flow.cast(data, spellKind(key, details ?? null), (preset) =>
      this.castSheet(ref, undefined, preset ?? undefined),
    );
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
  castSheet(ref: string, resume?: readonly PendingDamage[], preset?: CastPreset): void {
    const built = this.castData(ref, resume);
    if (!built) {
      return;
    }
    const data: CastSheetData = preset ? { ...built, preset } : built;
    openSheet<CastSheet, CastSheetData, boolean>(this.ctx.dialog, this.ctx.bottomSheet, CastSheet, {
      data,
      ariaLabel: castAriaLabel(data.name, !!resume),
      labelledBy: 'sheet-t',
      ...(opensOnMap(data) ? { focus: '[role="application"]' } : {}),
    }).subscribe();
  }

  /** What the cast sheet is handed for a spell of the options, or `null` when the combat or the options are not there. */
  private castData(ref: string, resume?: readonly PendingDamage[]): CastSheetData | null {
    const e = this.ctx.encounter();
    const caster = this.ctx.own();
    const turn = this.ctx.options();
    if (!e || !caster || !turn?.options) {
      return null;
    }
    return castSheetData({
      key: ref,
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

  /** "Usar" on a creature's action with a shape, on a map: the master's picker of the area, the same map as the spells'. The
   * action is sent from the dialog; `done` gets what it did, or `null` when the master gave up or there is no map. */
  creatureArea(req: CreatureAreaRequest): void {
    const e = this.ctx.encounter();
    const who = e ? currentCombatant(e) : null;
    const map = this.castMap();
    if (!e || !who || !map) {
      req.done(null);
      return;
    }
    const data: MasterCreatureAreaData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      actor: who,
      action: req.action,
      placement: req.placement,
      targetIds: [],
      map,
      state: this.ctx.state(),
    };
    openSheet<MasterCreatureArea, MasterCreatureAreaData, CreatureActionOutcome | null>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      MasterCreatureArea,
      {
        data,
        ariaLabel: `${who.label} usa ${req.action.namePt || req.action.name}`,
        labelledBy: 'sheet-t',
        width: '760px',
        tall: true,
        focus: '[role="application"]',
      },
    ).subscribe((outcome) => req.done(outcome ?? null));
  }

  /** "Mover" on a zone: the picker of its new place (a sheet on a phone, a dialog from a tablet up). `false` without a battle map. */
  moveZone(zoneId: string, onClosed: () => void): boolean {
    const e = this.ctx.encounter();
    const map = this.castMap();
    const zone = e?.zones.find((z) => z.id === zoneId);
    if (!e || !map || !zone) {
      return false;
    }
    const data: ZoneMoveSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      zoneId,
      master: this.ctx.isMaster(),
      map,
      state: this.ctx.state(),
    };
    openSheet<ZoneMoveSheet, ZoneMoveSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      ZoneMoveSheet,
      {
        data,
        // The phone's bottom sheet is named by this label, a dialog by its title ("Mover o Raio Lunar"): one name for both.
        ariaLabel: zoneMoveTitle(zone.spellNamePt),
        labelledBy: 'sheet-t',
        width: '760px',
        panelClass: 'mr-sheet-zone',
        focus: '.cm__surface[tabindex]',
      },
    ).subscribe({ complete: onClosed });
    return true;
  }

  private isSummon(key: string, grantedBy = ''): Promise<boolean> {
    const own = this.ctx.own();
    if (!own) {
      return Promise.resolve(false);
    }
    // A failed read is not kept: the next cast asks again, so one error never hides Conjurar Animais for the whole session.
    this.summonSpells ??= this.ctx.creatures
      .summonOptions(this.ctx.campaignId(), own.characterId)
      .then(
        (options) =>
          new Set(options.spells.map((s) => castRefOfKey(s.spellKey, s.grantedBy ?? ''))),
        (err: unknown) => {
          this.summonSpells = null;
          throw err;
        },
      );
    return this.summonSpells.then(
      (keys) => keys.has(castRefOfKey(key, grantedBy)),
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
}
