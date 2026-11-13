import {
  type CreatureActionResult,
  type Encounter,
  type PreviewSpellAreaResponse,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type CombatClientBase } from './combat-client-base';
import {
  type AttackDie,
  type PairDie,
  type ModeChoice,
  type PoolDie,
  type SlotRef,
  type MetamagicSpec,
  type CastTarget,
  type CastResult,
  type AreaChoice,
  type CastExtras,
  type SummonRequest,
} from './combat-client-types';
import { castExtrasBody, castRollOneof, areaOneof, need } from './combat-client-wire';
import type { Square } from './combat-grid';
import { wallBody, type WallChoice } from './spell-wall';
import { HeldRollAsked, heldOfferOf } from '../held-rolls/held-roll-offer';

/** The calls of spells: casting, previewing an area, the hidden creatures an area reached, and the zones a spell keeps. */
export class CombatCastCalls {
  constructor(private readonly base: CombatClientBase) {}

  /** `CastSpell`. `slot` is `null` for a cantrip; `die` is the d20 of a spell
   * attack (one per target in the app, or a typed face for one target), or the
   * pool of a spell that reads hit points (rolled by the server, or the typed sum). */
  async castSpell(
    campaignId: string,
    encounterId: string,
    casterId: string,
    spellKey: string,
    slot: SlotRef | null,
    targets: readonly CastTarget[],
    die: AttackDie | PoolDie | PairDie | null,
    key: string,
    summon?: SummonRequest,
    damageTypeKey = '',
    mode?: Omit<ModeChoice, 'requestId'>,
    metamagic: readonly MetamagicSpec[] = [],
    extras: CastExtras = {},
  ): Promise<CastResult> {
    const res = await this.base.client.castSpell({
      campaignId,
      encounterId,
      casterId,
      spellKey,
      slot: slot ?? undefined,
      targets: targets.map((t) => ({ combatantId: t.combatantId, darts: t.darts })),
      idempotencyKey: key,
      damageTypeKey,
      roll: castRollOneof(die),
      d20Faces: die && 'faces' in die ? [...die.faces] : [],
      ...(mode ? { rollMode: mode.mode, modeReason: mode.reason } : {}),
      metamagic: metamagic.map((m) => ({
        key: m.key,
        targetIds: [...m.targetIds],
        carefulIds: [...m.carefulIds],
        heightenedId: m.heightenedId,
      })),
      summon: summon
        ? {
            option: summon.option,
            creatureKeys: [...summon.creatureKeys],
            names: [...(summon.names ?? [])],
          }
        : undefined,
      area: areaOneof(extras.area),
      ...castExtrasBody(extras),
    });
    const asked = heldOfferOf(res);
    if (asked) {
      // A spell attack whose d20 waits for a question (a halfling's 1, a Bardic Inspiration die): the d20 is kept and
      // nothing was spent.
      throw new HeldRollAsked(asked, need(res.encounter, 'CastSpell'));
    }
    return {
      encounter: need(res.encounter, 'CastSpell'),
      cast: need(res.cast, 'CastSpell'),
      summoned: res.summonedCombatantIds,
      area: res.area
        ? {
            origin: res.area.origin ? { col: res.area.origin.col, row: res.area.origin.row } : null,
            squares: res.area.squares.map((q) => ({ col: q.col, row: q.row })),
          }
        : null,
      hiddenHits: res.hiddenHits,
      pendingRevealId: res.pendingRevealId,
    };
  }

  /** Revivify in a combat: the dead creature is a `deadTargetId` (never a combatant target), the diamonds are ticked, the key is the caller's. */
  async castRevivify(
    campaignId: string,
    encounterId: string,
    casterId: string,
    slot: SlotRef,
    deadTargetId: string,
    key: string,
  ): Promise<CastResult> {
    const res = await this.base.client.castSpell({
      campaignId,
      encounterId,
      casterId,
      spellKey: 'spell:revivify',
      slot,
      targets: [{ deadTargetId }],
      materialConfirmed: true,
      idempotencyKey: key,
    });
    return {
      encounter: need(res.encounter, 'CastSpell'),
      cast: need(res.cast, 'CastSpell'),
      summoned: res.summonedCombatantIds,
      area: null,
      hiddenHits: [],
      pendingRevealId: '',
    };
  }

  /** `PreviewSpellArea`: who an area spell placed here reaches and each one's cover, spending nothing. `area` is `null` for a
   * sphere centered on the caster, which has nothing to choose. `wall` is the shape of a wall (Wall of Fire), which `area`'s
   * origin places. */
  async previewSpellArea(
    campaignId: string,
    encounterId: string,
    casterId: string,
    spellKey: string,
    slot: SlotRef | null,
    area: AreaChoice | null,
    grantedBy = '',
    wall: WallChoice | null = null,
  ): Promise<PreviewSpellAreaResponse> {
    return this.base.client.previewSpellArea({
      campaignId,
      encounterId,
      casterId,
      spellKey,
      slot: slot ?? undefined,
      area: areaOneof(area),
      ...(grantedBy ? { grantedBy } : {}),
      ...(wall ? { wall: wallBody(wall) } : {}),
    });
  }

  /** The master's answer to a question of `Encounter.pending_hidden_reveals`: reveal the hidden creatures the area hit, or keep them hidden. */
  async resolveHiddenReveal(
    campaignId: string,
    encounterId: string,
    pendingRevealId: string,
    reveal: boolean,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['resolveHiddenReveal', campaignId, encounterId, pendingRevealId, reveal],
      (sent) =>
        this.base.client.resolveHiddenReveal({
          campaignId,
          encounterId,
          pendingRevealId,
          reveal,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'ResolveHiddenReveal');
  }

  /** `MoveSpellZone`: the caster moves an area that moves by an action (the master moves any zone). One key per move:
   * the same zone and square again is a retry and keeps it; after the move worked the next one is new. */
  async moveSpellZone(
    campaignId: string,
    encounterId: string,
    zoneId: string,
    origin: { col: number; row: number },
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['moveSpellZone', campaignId, encounterId, zoneId, origin.col, origin.row],
      (sent) =>
        this.base.client.moveSpellZone({
          campaignId,
          encounterId,
          zoneId,
          origin: { col: origin.col, row: origin.row },
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'MoveSpellZone');
  }

  /** `EndSpellZone`: the master ends a zone (a concentration that ended outside the app). */
  async endSpellZone(campaignId: string, encounterId: string, zoneId: string): Promise<Encounter> {
    const res = await this.base.keyed(['endSpellZone', campaignId, encounterId, zoneId], (sent) =>
      this.base.client.endSpellZone({ campaignId, encounterId, zoneId, idempotencyKey: sent }),
    );
    return need(res.encounter, 'EndSpellZone');
  }

  /** `UseCreatureAction` (master only): the NPC on turn uses one action of its stat block that is no attack roll. `targetIds`
   * are the creatures it affects (none for an action with no roll, or for an area on a map: the server takes who is inside);
   * `area` places a cone, a line, a cube, a sphere or a cylinder on a map. One key per use: the same action, targets and place
   * again is a retry; after it worked, the next use is new. */
  async useCreatureAction(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    actionKey: string,
    targetIds: readonly string[],
    area?: AreaChoice | null,
  ): Promise<{
    readonly encounter: Encounter;
    readonly result: CreatureActionResult;
    readonly area: { readonly origin: Square | null; readonly squares: readonly Square[] } | null;
  }> {
    const places = area
      ? 'origin' in area
        ? [area.origin.col, area.origin.row]
        : [area.direction.dx, area.direction.dy]
      : [];
    const res = await this.base.keyed(
      [
        'useCreatureAction',
        campaignId,
        encounterId,
        combatantId,
        actionKey,
        targetIds.join(','),
        ...places,
      ],
      (sent) =>
        this.base.client.useCreatureAction({
          campaignId,
          encounterId,
          combatantId,
          actionKey,
          targetIds: [...targetIds],
          area: areaOneof(area),
          idempotencyKey: sent,
        }),
    );
    return {
      encounter: need(res.encounter, 'UseCreatureAction'),
      result: need(res.result, 'UseCreatureAction'),
      area: res.area
        ? {
            origin: res.area.origin ? { col: res.area.origin.col, row: res.area.origin.row } : null,
            squares: res.area.squares.map((q) => ({ col: q.col, row: q.row })),
          }
        : null,
    };
  }
}
