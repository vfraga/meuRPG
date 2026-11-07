import {
  CombatEffect,
  type CombatLogEntry,
  CombatLogKind,
  SpellEffectKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { circleLabel } from './combat-options';
import { creatureActionText } from './combat-log-creature';
import { trapLogText } from '../traps/trap-log';
import { article } from '../format/article';
import { luckyText } from './lucky';
import { castCountedNotes, castTreatedNotes, rerollNote } from './combat-log-dice';
import { legendaryOptionName } from './legendary-card';
import {
  actionText,
  channelNotes,
  conditionsText,
  deathSaveText,
  hitPointsText,
  hitRiderText,
  modeAnsweredText,
  monstersAddedText,
  movedText,
  resourceText,
  spentText,
  stateText,
  wildShapeText,
} from './combat-log-actions';
import {
  attackText,
  castText,
  damageNotes,
  modeNote,
  pairOf,
  reviveCastText,
} from './combat-log-attack';
import { contestText } from './combat-log-contest';
import { effectEndedText, effectLine, exhaustionLogText } from './combat-log-effects';
import { LogContext, LogLine, NO_CONTEXT } from './combat-log-types';
import { the } from './combat-log-words';

/** What a line builder is given: the entry, the fields every line shares, the combat's name and who is reading. */
interface LineArgs {
  readonly e: CombatLogEntry;
  readonly base: { id: string; actor: string; hidden: boolean; undoable: boolean };
  readonly encounterName: string;
  readonly ctx: LogContext;
}

/** One builder per kind of entry, so a new kind is one more row and no function grows with it. A builder answers `null` for an
 * entry it cannot write (an effect with no effect in it). */
const LINE_BUILDERS: ReadonlyMap<CombatLogKind, (args: LineArgs) => LogLine | null> = new Map([
  [
    CombatLogKind.COMBAT_BEGUN,
    ({ base, encounterName, ctx }) => {
      return {
        ...base,
        icon: 'flag',
        actor: '',
        text:
          (encounterName ? `Combate iniciado: ${encounterName}` : 'Combate iniciado') +
          (ctx.theatre ? ', sem mapa (teatro da mente)' : ''),
      };
    },
  ],
  // A spell attack (Raio de Fogo) has the sparkles E6-15 draws for spells.
  [
    CombatLogKind.ATTACK,
    ({ e, base }) => {
      return {
        ...base,
        icon: e.key.startsWith('spell:') ? 'auto_awesome' : 'swords',
        text: attackText(e),
        roll: pairOf(e.attackRoll, 'd20 do ataque'),
        notes: [modeNote(e.modeChange), rerollNote(e.attackRoll), ...damageNotes(e.damage)].filter(
          Boolean,
        ),
      };
    },
  ],
  [
    CombatLogKind.ACTION,
    ({ e, base }) => {
      return {
        ...base,
        icon: e.key === 'standard:hide' ? 'visibility_off' : 'bolt',
        text: actionText(e),
        ...(e.channelDivinity ? { notes: channelNotes(e) } : {}),
      };
    },
  ],
  [
    CombatLogKind.MOVED,
    ({ e, base, ctx }) => {
      return {
        ...base,
        icon: ctx.theatre ? 'directions_run' : 'arrow_forward',
        text: ctx.theatre ? spentText(e) : movedText(e),
      };
    },
  ],
  // The server puts the NPC whose hit points changed in the target.
  [
    CombatLogKind.HIT_POINTS_ADJUSTED,
    ({ e, base }) => {
      return {
        ...base,
        actor: e.actorLabel || e.targetLabel,
        icon: 'healing',
        text: hitPointsText(e),
      };
    },
  ],
  [
    CombatLogKind.REVEAL_CHANGED,
    ({ e, base }) => {
      return {
        ...base,
        actor: e.actorLabel || e.targetLabel,
        icon: e.nowHidden ? 'visibility_off' : 'visibility',
        text: e.nowHidden ? ' foi escondido dos jogadores' : ' foi revelado aos jogadores',
      };
    },
  ],
  [
    CombatLogKind.COMBAT_ENDED,
    ({ base }) => {
      return { ...base, icon: 'flag', actor: '', text: 'Combate encerrado' };
    },
  ],
  [
    CombatLogKind.SPELL_CAST,
    ({ e, base, ctx }) => {
      if (e.creatureAction) {
        // The master also reads the SRD's text of the action, as a line under the sentence.
        return {
          ...base,
          icon: 'bolt',
          text: creatureActionText(e, ctx),
          ...(e.creatureAction.text ? { notes: [`Texto do SRD: ${e.creatureAction.text}`] } : {}),
        };
      }
      if (e.spell?.effectKind === SpellEffectKind.REVIVE) {
        return { ...base, icon: 'favorite', ...reviveCastText(e, ctx) };
      }
      const cast = castText(e, ctx);
      // "Sortudo: 1 → 14": a spell attack's d20 the halfling's Lucky rolled again, or a halfling player's save the app
      // rolled again by itself (the server sends the roll to whoever may read it), kept by the event so a reload still
      // finds it.
      const targets = e.spell?.targets ?? [];
      const lucky = [
        ...targets.flatMap((t) => [rerollNote(t.attackRoll), luckyText(t.save?.roll)]),
        ...castTreatedNotes(targets),
        ...castCountedNotes(targets),
      ].filter(Boolean);
      return {
        ...base,
        icon: 'auto_awesome',
        text: cast.text,
        card: cast.card,
        ...(lucky.length ? { notes: lucky } : {}),
      };
    },
  ],
  // The master brought a dead character back: the server puts the combatant in the target and sends no actor.
  [
    CombatLogKind.CHARACTER_REVIVED,
    ({ e, base }) => {
      return {
        ...base,
        actor: '',
        icon: 'favorite',
        text: `O mestre reviveu ${e.targetLabel || 'alguém'}`,
      };
    },
  ],
  [
    CombatLogKind.REACTION,
    ({ e, base }) => {
      const slot = e.spell?.slot;
      return {
        ...base,
        icon: 'shield',
        text: ` conjura ${e.keyNamePt || 'uma magia'}${slot ? ` (${slot.resource ? 'conjuração inata' : circleLabel(slot.level)})` : ''}, com a reação`,
      };
    },
  ],
  [
    CombatLogKind.DEATH_SAVE,
    ({ e, base }) => {
      return { ...base, icon: 'heart_broken', text: deathSaveText(e) };
    },
  ],
  // The server puts the character in the target.
  [
    CombatLogKind.DEATH_CONFIRMED,
    ({ e, base }) => {
      return { ...base, actor: e.targetLabel || e.actorLabel, icon: 'close', text: ' morreu' };
    },
  ],
  [
    CombatLogKind.CONDITIONS_CHANGED,
    ({ e, base }) => {
      return {
        ...base,
        actor: e.targetLabel || e.actorLabel,
        icon: 'label',
        text: conditionsText(e),
      };
    },
  ],
  // A trap that fired is public: everyone reads the line (the dice stay with the master and the creature's player).
  [
    CombatLogKind.TRAP_TRIGGERED,
    ({ e, base, ctx }) => {
      return { ...base, icon: 'warning', actor: '', text: trapLogText(e.trap, ctx.master) };
    },
  ],
  // A member of a joint turn ended their part and the turn goes on.
  [
    CombatLogKind.TURN_PART_ENDED,
    ({ e, base }) => {
      return {
        ...base,
        icon: 'flag',
        text: ` encerrou a parte ${article(e.actorLabel) === 'a' ? 'dela' : 'dele'}`,
      };
    },
  ],
  [
    CombatLogKind.WILD_SHAPE,
    ({ e, base }) => {
      return { ...base, icon: 'pets', text: wildShapeText(e) };
    },
  ],
  // The master offered it, in a combat without a map: the mover is the actor and the reactor the target.
  [
    CombatLogKind.OPPORTUNITY_OFFERED,
    ({ e, base }) => {
      return { ...base, icon: 'swords', text: ` saiu do alcance de ${e.targetLabel || 'alguém'}` };
    },
  ],
  [
    CombatLogKind.HIT_RIDER,
    ({ e, base }) => {
      return { ...base, icon: 'sports_martial_arts', text: hitRiderText(e) };
    },
  ],
  // A move opened a closed door (RN-26): "Toren abriu a porta." The server sends the line only to who saw or remembers the door.
  [
    CombatLogKind.DOOR_OPENED,
    ({ base }) => {
      return { ...base, icon: 'door_open', text: ' abriu a porta' };
    },
  ],
  [
    CombatLogKind.EFFECT_ENDED,
    ({ e, base, ctx }) => {
      return {
        ...base,
        icon: e.effectEnd?.effect === CombatEffect.SHIELD ? 'shield' : 'favorite_border',
        actor: '',
        text: effectEndedText(e, ctx),
      };
    },
  ],
  [
    CombatLogKind.EFFECT,
    ({ e, base, ctx }) => {
      return e.effect ? effectLine(base, e.effect, ctx) : null;
    },
  ],
  [
    CombatLogKind.EXHAUSTION,
    ({ e, base, ctx }) => {
      return e.effect
        ? { ...base, icon: 'battery_alert', actor: '', text: exhaustionLogText(e.effect, ctx) }
        : null;
    },
  ],
  [
    CombatLogKind.RESOURCE,
    ({ e, base }) => {
      const r = resourceText(e);
      return { ...base, icon: r.icon, text: r.text };
    },
  ],
  // A reaction was answered or closed (PM-04): one line, written on the server for who reads it (the master's has the
  // numbers of the NPCs, the players' never names a reactor they do not see).
  [
    CombatLogKind.REACTION_WINDOW,
    ({ e, base }) => {
      return { ...base, icon: 'bolt', actor: '', text: e.reactionTextPt };
    },
  ],
  // The master put monsters in the combat (RN-29): the line is his alone, with the hit points and the dice they were rolled with.
  [
    CombatLogKind.MONSTERS_ADDED,
    ({ e, base }) => {
      return { ...base, hidden: true, icon: 'pets', actor: '', text: monstersAddedText(e) };
    },
  ],
  // The master spent legendary actions on an option of a stat block that the engine does not resolve: the line is
  // his alone, with the creature, the option as the stat block names it (SRD, English) and the cost.
  [
    CombatLogKind.LEGENDARY_SPENT,
    ({ e, base, ctx }) => {
      const spent = e.legendarySpent;
      if (!spent) {
        return null;
      }
      const actions = spent.cost === 1 ? 'ação lendária' : 'ações lendárias';
      return {
        ...base,
        hidden: true,
        icon: 'bolt',
        text: `gastou ${spent.cost} ${actions}: ${legendaryOptionName(ctx.legendaryNames, e.actorId, spent.optionName) ?? spent.optionName}`,
      };
    },
  ],
  // The server rolled a Recharge action's d6 at the start of its creature's turn: the line is the master's and the creature's controller's.
  [
    CombatLogKind.RECHARGE_ROLLED,
    ({ e, base }) => {
      const recharge = e.recharge;
      if (!recharge) {
        return null;
      }
      return {
        ...base,
        icon: 'casino',
        text: `rolou ${recharge.face} para ${recharge.actionName}: ${recharge.recharged ? 'recarregou' : 'não recarregou'}`,
      };
    },
  ],
  // The master took an extra out of a damage; the reason is only for him and the attacker's player.
  [
    CombatLogKind.DAMAGE_PART_REMOVED,
    ({ e, base }) => {
      return {
        ...base,
        icon: 'remove_circle_outline',
        actor: '',
        text: `O mestre tirou ${e.keyNamePt ? the(e.keyNamePt) : 'um extra'}${e.reason ? `: “${e.reason}”` : ''}`,
      };
    },
  ],
  [
    CombatLogKind.STATE_CHANGED,
    ({ e, base }) => {
      return e.state
        ? { ...base, icon: 'local_fire_department', actor: '', text: stateText(e, e.state) }
        : null;
    },
  ],
  [
    CombatLogKind.CONTEST,
    ({ e, base, ctx }) => {
      const line = contestText(e, ctx);
      return line ? { ...base, icon: line.icon, text: line.text } : null;
    },
  ],
  [
    CombatLogKind.ROLL_MODE_ANSWERED,
    ({ e, base }) => {
      return e.modeChange
        ? {
            ...base,
            icon: 'casino',
            text: modeAnsweredText(e.modeChange),
            notes: e.modeChange.reason ? [`Motivo: “${e.modeChange.reason}”`] : undefined,
          }
        : null;
    },
  ],
]);

/** The line of one entry, or `null` for a kind this app doesn't know. */
export function logLine(
  e: CombatLogEntry,
  encounterName = '',
  ctx: LogContext = NO_CONTEXT,
): LogLine | null {
  const base = { id: e.id, actor: e.actorLabel, hidden: e.hidden, undoable: e.undoable };
  return LINE_BUILDERS.get(e.kind)?.({ e, base, encounterName, ctx }) ?? null;
}
