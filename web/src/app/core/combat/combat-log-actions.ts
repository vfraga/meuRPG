import { hookStandUpSentence } from './hook-text';
import {
  type CombatLogEntry,
  type CombatLogModeChange,
  type CombatLogStateChange,
  DeathSaveOutcome,
  JumpKind,
  LayOnHandsCureKind,
  WildShapeEndReason,
  HitRiderChoice,
  HitRiderLogResult,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { CombatantStateKind } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { d20TotalText, rollText } from './combat-dice';
import { damageText } from './combat-log-attack';
import { luckyText } from './lucky';
import { modeWord } from './roll-mode';
import { conditionName, listNames } from './conditions';
import { metersFixed, metersText } from '../units';
import { circleLabel } from './combat-options';
import { pointsText } from '../resources/pools';
import { countsSentence } from './death-saves';
import { article } from '../format/article';
import { dcLabel } from '../format/save-dc';
import { pushText, the } from './combat-log-words';
import { durationPt } from './creature-action';

/** What a class resource did (Cura pelas Mãos, Conjuração Flexível, Inspiração de Bardo). The hit points a touch gave back
 * are only in the entry for the master and the target's player; why a touch did nothing is the master's alone. */
export function resourceText(e: CombatLogEntry): { icon: string; text: string } {
  const r = e.resource;
  const target = e.targetLabel || 'alguém';
  switch (r?.key) {
    case 'feature:lay-on-hands':
      if (r.nothingHappened) {
        const why = r.nothingReason ? `; o toque não agiu: ${r.nothingReason}` : '';
        return {
          icon: 'favorite',
          text: ` tocou ${the(target)} com a Cura pelas Mãos (${pointsText(r.spent)}): sem efeito${why}`,
        };
      }
      if (r.cure === LayOnHandsCureKind.POISON) {
        return {
          icon: 'favorite',
          text: ` neutralizou o veneno de ${target} com a Cura pelas Mãos (${pointsText(r.spent)})`,
        };
      }
      if (r.cure === LayOnHandsCureKind.DISEASE) {
        return {
          icon: 'favorite',
          text: ` curou a doença de ${target} com a Cura pelas Mãos (${pointsText(r.spent)})`,
        };
      }
      return {
        icon: 'favorite',
        text:
          r.healed === undefined
            ? ` usou a Cura pelas Mãos em ${target} (${pointsText(r.spent)})`
            : ` curou ${r.healed} PV de ${target} com a Cura pelas Mãos (${pointsText(r.spent)})`,
      };
    case 'feature:flexible-casting-creating-spell-slots':
      return {
        icon: 'auto_awesome',
        text: ` criou um espaço de ${circleLabel(r.slotLevel)} com a Conjuração Flexível (${pointsText(r.spent)} de feitiçaria)`,
      };
    case 'feature:flexible-casting-converting-spell-slot':
      return {
        icon: 'auto_awesome',
        text: ` converteu um espaço de ${circleLabel(r.slotLevel)} em ${pointsText(r.gained)} de feitiçaria`,
      };
    case 'feature:bardic-inspiration':
      return {
        icon: 'music_note',
        text:
          r.dieSides > 0
            ? ` deu um d${r.dieSides} da Inspiração de Bardo a ${target}`
            : ` deu a Inspiração de Bardo a ${target}`,
      };
    default:
      return { icon: 'bolt', text: ' usou um recurso da classe' };
  }
}

export function deathSaveText(e: CombatLogEntry): string {
  const s = e.deathSave;
  if (!s) {
    return ' faz um teste contra a morte';
  }
  // A table that hides the death saves (RN-24) tells everyone else only the result: no roll, no outcome, no counts.
  if (s.stable && !s.roll && s.outcome === DeathSaveOutcome.UNSPECIFIED) {
    return ' estabilizou';
  }
  const word =
    s.outcome === DeathSaveOutcome.REVIVED
      ? 'volta com 1 PV'
      : s.outcome === DeathSaveOutcome.CRITICAL_FAILURE
        ? 'falha crítica, conta duas falhas'
        : s.outcome === DeathSaveOutcome.FAILURE
          ? 'falha'
          : 'sucesso';
  // The d20 is the master's and the character's own player's; the others read the outcome.
  const roll = s.roll
    ? `rola o teste contra a morte: ${rollText(s.roll)}, `
    : 'faz um teste contra a morte: ';
  let out = ` ${roll}${word}`;
  if (s.outcome !== DeathSaveOutcome.REVIVED) {
    out += ` (${countsSentence(s.successes, s.failures)})`;
  }
  const rerolled = luckyText(s.roll); // "Sortudo: 1 → 14" or "Indomitável: 4 → 15"
  if (rerolled) {
    out += `. ${rerolled}`;
  }
  if (s.hook) {
    out += `. ${hookStandUpSentence(s.hook, e.actorLabel || 'o personagem')}`;
  }
  if (s.stable) {
    out += '. Estável: não rola mais';
  } else if (s.dying) {
    out += '. Morrendo: o mestre confirma a morte';
  }
  return out;
}

/** "está Envenenado e Derrubado", "ficou sem condições", and the concentration that ended. */
export function conditionsText(e: CombatLogEntry): string {
  const parts: string[] = [];
  const names = e.conditions.map(conditionName).filter(Boolean);
  // An entry that only ended a concentration carries no conditions: it must
  // not read as "ficou sem condições".
  if (names.length > 0 || !e.concentrationEndedKey) {
    parts.push(names.length > 0 ? `ficou ${listNames(names)}` : 'ficou sem condições');
  }
  if (e.concentrationEndedKey) {
    parts.push('deixou de se concentrar');
  }
  return ` ${parts.join('; ')}`;
}

/** What a standard action reads as in the log, by its key. */
export function actionText(e: CombatLogEntry): string {
  switch (e.key) {
    case 'standard:hide':
      return ' se esconde';
    case 'standard:search':
      return ' procura ao redor';
    case 'standard:ready':
      return ' se prepara';
    case 'standard:use-an-object':
      return ' usa um objeto';
    default:
      return ` usa ${e.keyNamePt || 'uma ação'}`;
  }
}

/** What Canalizar Divindade did, one note for each creature: the hit points Preservar a Vida gave (for the master and
 * the cleric's player only; the server sends nothing to the others), or the save of each undead against Expulsar
 * Mortos-Vivos. */
export function channelNotes(e: CombatLogEntry): string[] {
  const c = e.channelDivinity;
  if (!c) {
    return [];
  }
  const heals = c.heals.map(
    (h) =>
      `${h.label}: recuperou ${h.healed} PV (pedido ${h.asked}), agora com ${h.hitPointsAfter} PV`,
  );
  const turns = c.turns.map(
    (t) =>
      `${t.label}: Sabedoria ${t.total} contra CD ${c.saveDc}, ${t.turned ? 'foi expulso' : 'resistiu'}`,
  );
  return [...heals, ...turns];
}

export function hitPointsText(e: CombatLogEntry): string {
  const n = Math.abs(e.hitPointsDelta);
  const after = e.hitPointsAfter === undefined ? '' : `, agora com ${e.hitPointsAfter} PV`;
  if (e.hitPointsDelta < 0) {
    return ` perdeu ${n} PV, por ajuste do mestre${after}`;
  }
  if (e.hitPointsDelta > 0) {
    return ` recuperou ${n} PV, por ajuste do mestre${after}`;
  }
  return ` teve os PV ajustados pelo mestre${after}`;
}

/** Movement spent by number, in a combat without a map: "gastou 6,0 m de movimento". */
export function spentText(e: CombatLogEntry): string {
  const length = e.distanceDft > 0 ? metersFixed(e.distanceDft / 10) : metersFixed(e.distanceFt);
  return ` gastou ${length} de movimento`;
}

/** A move: "anda 3,0 m", "saltou 4,5 m" or "saltou 1,8 m para cima". A long jump that landed in
 * difficult terrain reminds the master (only he gets the flag) of the SRD's Acrobacia check: the app rolls nothing. */
export function movedText(e: CombatLogEntry): string {
  if (e.jump === JumpKind.HIGH) {
    return ` saltou ${metersFixed((e.jumpHeightDft || e.distanceDft) / 10)} para cima`;
  }
  const length = e.distanceDft > 0 ? metersFixed(e.distanceDft / 10) : metersText(e.distanceFt);
  if (e.jump === JumpKind.LONG) {
    return ` saltou ${length}${e.jumpRunningStart ? ', com corrida' : ''}${e.landingDifficult ? ' e caiu em terreno difícil. Acrobacia CD 10 ou cai Derrubado' : ''}`;
  }
  return ` anda ${length}${spentNote(e)}`;
}

/** What a walk spent when it is not the distance: ", gasta 6,2 m de movimento (terreno difícil)". The
 * reasons come from the SRD: another creature's space and rubble cost double, a drag halves the speed and a
 * prone creature crawls at double the cost. Nothing when the walk spent what it went (or the event is old). */
function spentNote(e: CombatLogEntry): string {
  const distance = e.distanceDft > 0 ? e.distanceDft : e.distanceFt * 10;
  if (e.spentDft <= 0 || e.spentDft === distance) {
    return '';
  }
  const factor = (e.moveDragging ? 2 : 1) * (e.moveCrawling ? 2 : 1);
  const reasons: string[] = [];
  if (e.spentDft > distance * factor) {
    reasons.push('terreno difícil');
  }
  if (e.moveDragging) {
    reasons.push('arrastando');
  }
  if (e.moveCrawling) {
    reasons.push('rastejando');
  }
  return `, gasta ${metersFixed(e.spentDft / 10)} de movimento${reasons.length > 0 ? ` (${reasons.join(', ')})` : ''}`;
}

/** "Fúria de Toren começou", "Fúria de Toren acabou (não atacou nem sofreu dano)". */
export function stateText(e: CombatLogEntry, st: CombatLogStateChange): string {
  const noun =
    st.kind === CombatantStateKind.RAGE
      ? 'Fúria'
      : st.kind === CombatantStateKind.HUNTERS_MARK_TARGET
        ? 'Marca'
        : st.kind === CombatantStateKind.DODGING
          ? 'Esquiva'
          : st.kind === CombatantStateKind.RECKLESS
            ? 'Ataque descuidado'
            : 'Estado';
  const who = e.actorLabel || e.targetLabel;
  // A state a table feature defines is called by the feature's name.
  const called = st.kind === CombatantStateKind.TABLE && st.namePt ? st.namePt : noun;
  const name = who ? `${called} de ${who}` : called;
  if (st.started) {
    return `${name} começou`;
  }
  const reason = stateReason(st.reason);
  return `${name} acabou${reason ? ` (${reason})` : ''}`;
}

function stateReason(key: string): string {
  switch (key) {
    case 'no_attack':
      return 'não atacou nem sofreu dano';
    case 'unconscious':
      return 'ficou inconsciente';
    case 'by_choice':
      return 'por escolha';
    case 'duration':
      return 'a duração acabou';
    case 'incapacitated':
      return 'ficou incapacitado';
    case 'master':
      return 'o mestre encerrou';
    case 'left':
      return 'a concentração ou o combate acabou';
    default:
      return key;
  }
}

/** "Brisa pediu Vantagem; o mestre aprovou". */
export function modeAnsweredText(m: CombatLogModeChange): string {
  const asked = `pediu ${modeWord(m.mode)}`;
  return ` ${asked}; o mestre ${m.approved ? 'aprovou' : 'recusou'}`;
}

/** "Bandido 1: 9 PV (2d8 + 2: 3, 4)", or "Bandido 1: 11 PV (média)": what the master reads of the monsters that came in. */
export function monstersAddedText(e: CombatLogEntry): string {
  const n = e.monsters.length;
  const each = e.monsters.map((m) => {
    if (!m.rolled) {
      return `${m.label}: ${m.hitPoints} PV (média)`;
    }
    const dice = m.dice.replace(/([+-])/, ' $1 ');
    return `${m.label}: ${m.hitPoints} PV (${dice}: ${m.faces.join(', ')})`;
  });
  return `Você pôs ${n === 1 ? 'um monstro' : `${n} monstros`} no combate. ${each.join('; ')}`;
}

/** "virou o Lobo", "voltou à forma normal": a druid's change of form. The damage that passed on to the druid is only in the
 * entry the master and the druid's player get (RN-20); everyone else reads that the druid is back and why, with no number. */
export function wildShapeText(e: CombatLogEntry): string {
  const w = e.wildShape;
  if (!w) {
    return ' mudou de forma';
  }
  if (w.started) {
    return ` virou ${the(w.beastNamePt)}`;
  }
  const she = article(e.actorLabel) === 'a' ? 'ela' : 'ele';
  switch (w.endReason) {
    case WildShapeEndReason.DAMAGE:
      return ` voltou à forma normal: ${the(w.beastNamePt)} caiu a 0 PV${w.carriedDamage > 0 ? ` e ${w.carriedDamage} de dano passaram para ${she}` : ''}`;
    case WildShapeEndReason.MASTER:
      return ` voltou à forma normal, porque o mestre levou ${the(w.beastNamePt)} a 0 PV`;
    case WildShapeEndReason.ZERO_HP:
      return ' voltou à forma normal, ao cair a 0 PV';
    case WildShapeEndReason.UNCONSCIOUS:
      return ' voltou à forma normal, ao ficar inconsciente';
    default:
      return ' voltou à forma normal';
  }
}

/** The line of an answered monk rider: "Toren usou a Técnica da Mão Aberta (Derrubar) em Goblin: ...". The roll comes for the master and the monk's player, the DC for the master and, when the table shows it, the target's player. */
export function hitRiderText(e: CombatLogEntry): string {
  const r = e.hitRider;
  const target = e.targetLabel || 'alguém';
  if (r && (r.choice === HitRiderChoice.APPLY || r.actionNamePt)) {
    return creatureRiderText(e, r);
  }
  if (!r || r.choice === HitRiderChoice.DECLINE) {
    return ` dispensou a técnica em ${target}`;
  }
  const names: Partial<Record<HitRiderChoice, string>> = {
    [HitRiderChoice.PRONE]: 'a Técnica da Mão Aberta (Derrubar)',
    [HitRiderChoice.PUSH]: 'a Técnica da Mão Aberta (Empurrar até 4,5 m)',
    [HitRiderChoice.NO_REACTIONS]:
      'a Técnica da Mão Aberta (sem reações até o fim do próximo turno)',
    [HitRiderChoice.STUN]: 'o Golpe Atordoante',
  };
  const roll = r.numbers
    ? ` (${d20TotalText(r.d20, r.saveTotal)}${r.dc > 0 ? ` contra ${dcLabel(r.dc)}` : ''})`
    : '';
  let result = '';
  if (r.result === HitRiderLogResult.SAVED) {
    result = `: ${target} passou no teste${roll}`;
  } else if (r.result === HitRiderLogResult.FAILED) {
    result = `: ${target} falhou no teste${roll}`;
    if (r.choice === HitRiderChoice.PUSH) {
      result += `; ${pushText(r) || 'o mestre posiciona a criatura até 4,5 m mais longe'}`;
    }
  }
  return ` usou ${names[r.choice] ?? 'a técnica'} em ${target}${result}`;
}

/**
 * The line of a creature's rider the master answered (the extra save or the grapple of an attack): "usou Garras em Brisa: Brisa
 * falhou no teste (1d20 (8) + 2 = 10 contra CD 10); ficou Paralisado por 1 minuto". Only what the entry carries is written: a player
 * reads no DC and no key, and the condition only for a target they own or one with a sign everyone sees.
 */
function creatureRiderText(e: CombatLogEntry, r: NonNullable<CombatLogEntry['hitRider']>): string {
  const target = e.targetLabel || 'alguém';
  const name = r.actionNamePt || 'o ataque';
  if (r.choice === HitRiderChoice.DECLINE) {
    return ` dispensou o efeito de ${name} em ${target}`;
  }
  const roll = r.numbers
    ? ` (${d20TotalText(r.d20, r.saveTotal)}${r.dc > 0 ? ` contra ${dcLabel(r.dc)}` : ''})`
    : '';
  // The damage the save governs (a spider's poison), rolled and applied after the rider: its words follow the save's.
  const damage = r.damage ? [r.damage, ...r.moreDamages].map(damageText).join('') : '';
  const duration = r.durationText ? ` por ${durationPt(r.durationText)}` : '';
  const repeat = r.repeatSave ? '; repete o teste no fim de cada turno dele' : '';
  const condition = r.conditionNamePt ? `; ficou ${r.conditionNamePt}${duration}${repeat}` : '';
  const escape = r.escapeDc > 0 ? ` (${dcLabel(r.escapeDc).replace('CD', 'CD de fuga')})` : '';
  let result = '';
  if (r.result === HitRiderLogResult.SAVED) {
    result = `: ${target} passou no teste${roll}${damage}`;
  } else if (r.result === HitRiderLogResult.FAILED) {
    result = `: ${target} falhou no teste${roll}${damage}${condition}`;
  } else if (r.conditionNamePt) {
    result = `: ${target} ficou ${r.conditionNamePt}${escape}${duration}${repeat}`;
  }
  return ` usou ${name} em ${target}${result}`;
}
