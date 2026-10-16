import {
  type Combatant,
  type CombatLogEntry,
  CombatLogKind,
  WildShapeEndReason,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode, type ActionOption } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { ResourceUsageVm } from '../play/resource-usage.types';
import { joinDots, tight } from '../format/text';
import { article } from './combat-log';
import { isUnlimited } from './unlimited-uses';
import { reasonText } from './combat-options';
import { groupFeminine, groupName } from './creature-names';

/**
 * What a player is told when something ended without their asking, and the Wild Shape line of the turn. Words and
 * bookkeeping only: the numbers and the reasons are the server's (the combat log's Wild Shape line and the combat's own
 * data); nothing here works out a rule.
 */

/** A Wild Shape form that ended and was not the druid's own doing. */
export interface FormEnded {
  readonly beast: string;
  readonly reason: WildShapeEndReason;
  /** The damage that passed from the beast to the druid, as the server's log line says it; 0 when it says none. */
  readonly carried: number;
}

/** "O Lobo caiu a 0 PV e você voltou à forma normal. 6 de dano passaram para você.", and the other reasons in their own words. */
export function formNoticeText(ended: FormEnded): string {
  const a = article(ended.beast);
  const beast = `${a === 'a' ? 'A' : 'O'} ${ended.beast}`;
  switch (ended.reason) {
    case WildShapeEndReason.DAMAGE:
      return tight(
        `${beast} caiu a 0 PV e você voltou à forma normal.${ended.carried > 0 ? ` ${ended.carried} de dano passaram para você.` : ''}`,
      );
    case WildShapeEndReason.MASTER:
      return `O mestre levou ${a} ${ended.beast} a 0 PV e você voltou à forma normal.`;
    case WildShapeEndReason.ZERO_HP:
      return 'Você caiu a 0 PV e voltou à forma normal.';
    case WildShapeEndReason.UNCONSCIOUS:
      return 'Você ficou inconsciente e voltou à forma normal.';
    default:
      return 'Você voltou à forma normal.';
  }
}

/**
 * Reads the combat log for the end of the player's own Wild Shape. What the log already held when the combat was first read
 * is not news (a reconnect or a re-read never repeats a notice), a form the druid left by choice says nothing, and another
 * combat starts clean.
 */
export class FormNotices {
  private seen: Set<string> | null = null;
  private combat = '';

  /** `reset` says the combat changed (clear what was on screen); `notice` is the new end to tell, if any. */
  read(
    encounterId: string,
    ownId: string,
    loaded: boolean,
    entries: readonly CombatLogEntry[],
  ): { reset: boolean; notice: FormEnded | null } {
    const reset = encounterId !== this.combat;
    if (reset) {
      this.combat = encounterId;
      this.seen = null;
    }
    if (!loaded || !ownId) {
      return { reset, notice: null };
    }
    const lines = entries.filter(
      (en) =>
        en.kind === CombatLogKind.WILD_SHAPE &&
        en.actorId === ownId &&
        !!en.wildShape &&
        !en.wildShape.started,
    );
    if (this.seen === null) {
      this.seen = new Set(lines.map((l) => l.id));
      return { reset, notice: null };
    }
    let notice: FormEnded | null = null;
    for (const line of lines) {
      if (this.seen.has(line.id) || !line.wildShape) {
        continue;
      }
      this.seen.add(line.id);
      if (line.wildShape.endReason !== WildShapeEndReason.LEFT) {
        notice = {
          beast: line.wildShape.beastNamePt,
          reason: line.wildShape.endReason,
          carried: line.wildShape.carriedDamage,
        };
      }
    }
    return { reset, notice };
  }
}

/** What the player's concentration ending took with it. */
export interface LostConcentration {
  readonly spell: string;
  readonly gone: readonly Combatant[];
}

/** "Você perdeu a concentração em Conjurar Animais. Os 2 Lobos atrozes sumiram." */
export function lostNoticeText(lost: LostConcentration): string {
  const base = `Você perdeu a concentração em ${lost.spell}.`;
  if (lost.gone.length === 0) {
    return base;
  }
  const fem = lost.gone.length > 1 ? groupFeminine(lost.gone) : false;
  const who =
    lost.gone.length > 1
      ? `${fem ? 'As' : 'Os'} ${lost.gone.length} ${groupName(lost.gone)} sumiram.`
      : `${lost.gone[0].label} sumiu.`;
  return tight(`${base} ${who}`);
}

/**
 * Watches the player's concentration and the creatures it keeps, from the combat's own data, for one combat at a time. A
 * spell that goes (with its creatures, or none) is a loss to tell; the player's own ending says nothing; another combat starts
 * without it; and an undo that brings the creatures (or the spell) back takes the notice away.
 */
export class ConcentrationWatch {
  private held: { spell: string; creatures: readonly Combatant[] } | null = null;
  private combat = '';

  /** Marks that the player ended it themselves: the next loss is not news. */
  endedByMe = false;

  /** `notice` is `undefined` when nothing changes, `null` to clear what is shown, a loss to show. */
  read(
    encounterId: string,
    spell: string,
    creatures: readonly Combatant[],
    shown: LostConcentration | null,
  ): LostConcentration | null | undefined {
    let out: LostConcentration | null | undefined;
    if (encounterId !== this.combat) {
      this.combat = encounterId;
      this.held = null;
      this.endedByMe = false;
      out = shown ? null : undefined;
    }
    const current = out === null ? null : shown;
    if (
      current &&
      (current.gone.some((g) => creatures.some((c) => c.id === g.id)) ||
        (spell !== '' && spell === current.spell))
    ) {
      out = null;
    }
    if (this.held && spell !== this.held.spell) {
      const gone = this.held.creatures.filter((c) => !creatures.some((x) => x.id === c.id));
      if (this.endedByMe) {
        this.endedByMe = false;
      } else if (gone.length > 0 || spell === '') {
        out = { spell: this.held.spell, gone };
      }
    }
    this.held = spell ? { spell, creatures } : null;
    return out;
  }
}

/**
 * The druid's Wild Shape as a line of the Ação (E9-11 1): its uses ("restam 2 de 2 usos · volta no descanso curto ou longo")
 * and why it cannot be used now. `null` without the feature, and while in a form.
 */
export function wildActionLine(
  feature: ActionOption | undefined,
  inForm: boolean,
  resource: ResourceUsageVm | undefined,
): { readonly key: string; readonly detail: string; readonly reason: string } | null {
  if (!feature?.action || inForm) {
    return null;
  }
  const n = feature.usesLeft;
  const uses = resource
    ? isUnlimited(resource.total)
      ? 'usos ilimitados'
      : `restam ${n} de ${resource.total} ${resource.total === 1 ? 'uso' : 'usos'}`
    : `restam ${n} ${n === 1 ? 'uso' : 'usos'}`;
  const back =
    resource?.recharge === 'long_rest'
      ? 'volta no descanso longo'
      : resource?.recharge === 'dawn'
        ? 'volta ao amanhecer'
        : 'volta no descanso curto ou longo';
  const reason = feature.enabled
    ? ''
    : feature.reason?.code === DisabledReasonCode.NO_USES
      ? 'Sem usos'
      : feature.reason?.code === DisabledReasonCode.ACTION_USED
        ? 'Sem ação disponível'
        : reasonText(feature.reason);
  return {
    key: feature.action.key,
    detail: tight(joinDots(['Vire uma fera', uses, back])),
    reason,
  };
}
