import {
  type Encounter,
  type PreviewSpellAreaResponse,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { InspirationQuestion } from '../resources/bardic-inspiration';
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
import { LuckyQuestion } from './lucky-question';

/** The calls of spells: casting, previewing an area and the hidden creatures an area reached. */
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
    if (res.luckyOffer) {
      // A spell attack of a halfling whose d20 shows a 1: the d20 is kept, nothing was spent, and Lucky asks first.
      throw new LuckyQuestion(res.luckyOffer, need(res.encounter, 'CastSpell'));
    }
    if (res.inspirationOffer) {
      // A spell attack of a character that holds a die: the d20 is kept, nothing was spent.
      throw new InspirationQuestion(res.inspirationOffer, need(res.encounter, 'CastSpell'));
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
   * sphere centered on the caster, which has nothing to choose. */
  async previewSpellArea(
    campaignId: string,
    encounterId: string,
    casterId: string,
    spellKey: string,
    slot: SlotRef | null,
    area: AreaChoice | null,
  ): Promise<PreviewSpellAreaResponse> {
    return this.base.client.previewSpellArea({
      campaignId,
      encounterId,
      casterId,
      spellKey,
      slot: slot ?? undefined,
      area: areaOneof(area),
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
}
