import {
  type ConsumableTarget,
  ConsumableTargetState,
  type UseConsumableResponse,
} from '../../../gen/meurpg/play/v1/charges_pb';
import { CombatantState, ConsumableUseOutcome } from '../../../gen/meurpg/play/v1/combat_pb';
import { stateWord } from '../combat/state-word';
import { article } from '../format/article';
import { upFirst } from '../format/text';
import { metersText } from '../units';
import type { AttunementEntryVm } from './attunement';
import type { ConsumableKind, ConsumableVm, HealingDice } from './consumable-vm';

/**
 * The consumables of a sheet (potions, oils, scrolls and the like): the rows that group identical ones, the words of the
 * effect line, the receivers of a gift, the cost line of the "Usar" sheet and the notice of a use. The server says what an
 * entry is and costs (`SheetMagicItem.consumable`), words its effect (`effect_pt`) and the use of one
 * (`UseConsumableResponse`); the app only arranges what was sent. Pure, tested without a DOM.
 */

/** The words and the icon of a kind of consumable: how the sheet, the buttons and the refusals speak of it. */
export interface KindWords {
  /** "a poção": the definite article and the noun. */
  readonly the: string;
  /** "A poção", for the start of a sentence. */
  readonly The: string;
  /** "Essa poção já foi usada": the item was used already. */
  readonly goneUsed: string;
  /** "A poção acabou.": the lead of the notice of an item that is gone. */
  readonly goneLead: string;
  readonly icon: string;
  /** The verb of the button that applies it to oneself: a potion is drunk, any other item is used. */
  readonly use: 'Beber' | 'Usar';
}

const KIND_WORDS: Record<ConsumableKind, KindWords> = {
  potion: {
    the: 'a poção',
    The: 'A poção',
    goneUsed: 'Essa poção já foi usada',
    goneLead: 'A poção acabou.',
    icon: 'science',
    use: 'Beber',
  },
  oil: {
    the: 'o óleo',
    The: 'O óleo',
    goneUsed: 'Esse óleo já foi usado',
    goneLead: 'O óleo acabou.',
    icon: 'water_drop',
    use: 'Usar',
  },
  scroll: {
    the: 'o pergaminho',
    The: 'O pergaminho',
    goneUsed: 'Esse pergaminho já foi usado',
    goneLead: 'O pergaminho acabou.',
    icon: 'description',
    use: 'Usar',
  },
  other: {
    the: 'o item',
    The: 'O item',
    goneUsed: 'Esse item já foi usado',
    goneLead: 'O item acabou.',
    icon: 'inventory_2',
    use: 'Usar',
  },
};

/** How the sheet speaks of a kind of consumable. */
export function kindWords(kind: ConsumableKind): KindWords {
  return KIND_WORDS[kind];
}

/** A potion and an oil are given to someone ("Dar a…"); a scroll and any other item are used by whoever holds them. */
export function canGive(kind: ConsumableKind): boolean {
  return kind === 'potion' || kind === 'oil';
}

/** A row of the magic items that holds the identical units of one consumable. */
export interface ConsumableRowVm {
  /** Stable for the row while any unit is on the sheet: `consumable:<catalog key>`. */
  readonly id: string;
  readonly key: string;
  readonly namePt: string;
  readonly kind: ConsumableKind;
  /** What "Usar" is and costs, as the server says it. */
  readonly consumable: ConsumableVm;
  readonly icon: string;
  readonly effectPt: string;
  /** The line under the name: the server's, or "Só mostra o texto". */
  readonly effectLine: string;
  readonly effectIcon: string;
  /** How many units the sheet holds: "×2" from 2. */
  readonly count: number;
  /** The unit "Usar" sends: the first of the group (the server keeps one entry for each unit). */
  readonly first: AttunementEntryVm;
}

/** A row of the magic items: an item as it was, or a group of identical consumables. */
export type MagicRowVm =
  | { readonly type: 'item'; readonly id: string; readonly entry: AttunementEntryVm }
  | { readonly type: 'consumable'; readonly id: string; readonly row: ConsumableRowVm };

/** What an item that is no consumable has to say with no effect of its own. */
const TEXT_ONLY_LINE = 'Só mostra o texto';

/** The line of what "Usar" does, "Cura 2d4 + 2", or that the item only shows its text. */
export function effectLine(effectPt: string): string {
  return effectPt === '' ? TEXT_ONLY_LINE : effectPt;
}

/**
 * The icon of an effect: a heart for a healing (the server sent dice), a book for the text alone (the server sent no line)
 * and a shield for the one effect left, a resistance.
 */
export function effectIcon(effectPt: string, consumable: ConsumableVm | undefined): string {
  if (consumable?.heal) {
    return 'favorite';
  }
  return effectPt === '' ? 'menu_book' : 'shield';
}

/**
 * The magic items as the sheet draws them: every consumable of the same key is one row, at the place of the first, and
 * anything else stays a row of its own. An entry is a consumable when the server sent its `consumable`.
 */
export function magicRows(entries: readonly AttunementEntryVm[]): MagicRowVm[] {
  const groups = new Map<string, AttunementEntryVm[]>();
  for (const e of entries) {
    if (e.consumable && !e.charges) {
      groups.set(e.key, [...(groups.get(e.key) ?? []), e]);
    }
  }
  const rows: MagicRowVm[] = [];
  const placed = new Set<string>();
  for (const e of entries) {
    const consumable = e.charges ? undefined : e.consumable;
    if (!consumable) {
      rows.push({ type: 'item', id: e.id, entry: e });
    } else if (!placed.has(e.key)) {
      placed.add(e.key);
      const units = groups.get(e.key) ?? [e];
      const effectPt = e.effectPt ?? '';
      rows.push({
        type: 'consumable',
        id: `consumable:${e.key}`,
        row: {
          id: `consumable:${e.key}`,
          key: e.key,
          namePt: e.namePt,
          kind: consumable.kind,
          consumable,
          icon: kindWords(consumable.kind).icon,
          effectPt,
          effectLine: effectLine(effectPt),
          effectIcon: effectIcon(effectPt, consumable),
          count: units.length,
          first: units[0],
        },
      });
    }
  }
  return rows;
}

/** Why a row that costs the Action is off once the turn's Action is spent: it sits folded, and the row says it too. */
export const ACTION_SPENT_WHY = 'A ação desta vez já foi usada.';

/** A row of the combat's "Itens": one potion or scroll, or the group of identical ones, with the mark of what using it costs. */
export interface CombatItemRowVm {
  readonly id: string;
  readonly key: string;
  readonly namePt: string;
  readonly kind: ConsumableKind;
  readonly icon: string;
  /** The line under the name: what the item does, in lower case (the group's "×N" is the row's `count`). */
  readonly detail: string;
  readonly count: number;
  /** The Ação, for what costs it in a combat (`costsAction`); none for what costs no action. */
  readonly economy: 'action' | null;
  /** The unit "Usar" sends: the first of the group. */
  readonly first: AttunementEntryVm;
}

/** The consumables of a character as the combat's "Itens" lists them: the same rows as the sheet, with their cost. */
export function combatItemRows(entries: readonly AttunementEntryVm[]): CombatItemRowVm[] {
  return magicRows(entries).flatMap((r) => {
    if (r.type !== 'consumable') {
      return [];
    }
    const c = r.row;
    return [
      {
        id: c.id,
        key: c.key,
        namePt: c.namePt,
        kind: c.kind,
        icon: c.icon,
        detail: lowerFirst(c.effectLine),
        count: c.count,
        economy: c.consumable.costsAction ? ('action' as const) : null,
        first: c.first,
      },
    ];
  });
}

/** "resta 1" / "restam 2": how many units are left, in lower case for a line that goes on. */
export function leftWords(left: number): string {
  return left === 1 ? 'resta 1' : `restam ${left}`;
}

/**
 * The line under the title of the "Usar" sheet: the count and, where the use is known to be in a combat, that it spends the
 * action. The master's title names the character, so his line names the item first. `inCombat` is what the page or the
 * targets read said; `null` while it is not known, and then the cost is left to the line under the buttons, which says it
 * once the answer is in (a sheet never says both costs).
 */
export function subtitleOf(input: {
  readonly itemName: string;
  readonly left: number;
  readonly actingFor: string | null;
  readonly inCombat: boolean | null;
  /** The use costs the action (`consumable.costsAction`): an item that does not says nothing of the cost. */
  readonly costsAction: boolean;
}): string {
  const parts = input.actingFor
    ? [input.itemName, leftWords(input.left)]
    : [upFirst(leftWords(input.left))];
  if (input.costsAction && input.inCombat === true) {
    parts.push('em combate, gasta a ação');
  }
  return parts.join(' · ');
}

/** What the line under the buttons of the "Usar" sheet says: an info icon, a lead where there is one (bold with the mark of the Ação where the item spends it) and the rest. */
export interface UseCost {
  readonly mark: boolean;
  readonly lead: string;
  readonly rest: string;
}

/**
 * The cost line under the buttons of the "Usar" sheet. One line for one case.
 * In a combat the subtitle says the action is spent, so the line says only who the item acts on, and only in the give phase.
 * Out of a combat it says the turn is not spent, in words that differ by phase.
 * A use that does not cost the action says so whatever the case. `inCombat` is `null` while the page does
 * not know (the targets are not read yet, or could not be): the line then says nothing of the action. `acts` is
 * who the item acts on once a receiver is chosen ("A poção age em Brisa.").
 */
export function useCost(input: {
  readonly kind: ConsumableKind;
  readonly costsAction: boolean;
  readonly inCombat: boolean | null;
  readonly blocked: boolean;
  readonly phase: 'choose' | 'give';
  /** The receiver's name once one is chosen in the list. */
  readonly receiverName: string;
  /** Someone may receive it: "Dar a…" is offered. */
  readonly canGive: boolean;
}): UseCost | null {
  if (input.blocked) {
    return null;
  }
  const words = kindWords(input.kind);
  const acts = input.receiverName ? ` ${words.The} age em ${input.receiverName}.` : '';
  if (!input.costsAction) {
    return {
      mark: false,
      lead: 'Não gasta a ação: o mestre decide o tempo que isso leva.',
      rest: acts,
    };
  }
  if (input.inCombat === null) {
    return null;
  }
  if (input.inCombat) {
    // The cost is said once, in the subtitle ("em combate, gasta a ação"): the line under the list says only what the item does to the
    // receiver, and the first choice adds only the reason a button is dashed.
    return input.phase === 'give' && acts ? { mark: false, lead: '', rest: acts.trim() } : null;
  }
  return input.phase === 'give'
    ? { mark: false, lead: 'Fora de combate, não gasta a vez.', rest: acts }
    : {
        mark: false,
        lead: 'Fora de combate: não gasta a vez.',
        rest: input.canGive ? ' “Dar a…” lista o grupo todo.' : '',
      };
}

/** What the page knows keeps a use from the Action: it is not the user's turn, or the Action of the turn is spent. */
export type TurnBlock = 'turn' | 'action';

/**
 * What the combat knows that keeps a potion from the Action: it is not the character's turn (the turn comes before the Action),
 * or the Action of the turn is spent. `null` while both are there. Only an item that costs the action asks (`costsAction`).
 */
export function turnBlockOf(input: {
  readonly myTurn: boolean;
  readonly actionUsed: boolean;
}): TurnBlock | null {
  if (!input.myTurn) {
    return 'turn';
  }
  return input.actionUsed ? 'action' : null;
}

/** A character the consumable may be given to, as the radio list reads. */
export interface ConsumableTargetVm {
  readonly id: string;
  readonly name: string;
  readonly down: boolean;
  /** "A 1,5 m · Caída" in a combat, empty out of one (the server tells how a character stands only in a running combat). */
  readonly line: string;
}

/** The receivers the server sent that may receive it now, with the words of their line. */
export function targetRows(targets: readonly ConsumableTarget[]): ConsumableTargetVm[] {
  return targets.map((t) => {
    const down = t.state === ConsumableTargetState.DOWN;
    const parts = [
      t.distanceFt > 0 ? `A ${metersText(t.distanceFt)}` : '',
      down
        ? stateWord(CombatantState.DOWN, t.name)
        : t.state === ConsumableTargetState.OK
          ? 'De pé'
          : '',
    ].filter((p) => p !== '');
    return {
      id: t.characterId,
      name: t.name,
      down,
      line: parts.join(' · '),
    };
  });
}

/** What a use or a refusal says on the page, in the live region of the magic items or of the combat's items. */
export interface ConsumableNotice {
  readonly tone: 'good' | 'neutral';
  readonly icon: string;
  /** The bold lead: who got what, or what happened ("Brisa recebeu 7 PV e acorda."). */
  readonly headline: string;
  readonly detail: string;
  /** The item's text, for a consumable the app carries out no effect for. */
  readonly text: readonly string[];
  /** The line that heads the item's text and says what the item is ("Poção, muito rara"): the caption of the text's box, not a box of its own. */
  readonly caption?: string;
}

export interface UseNoticeInput {
  readonly itemName: string;
  readonly kind: ConsumableKind;
  readonly effectPt: string;
  /** The healing dice the server sent for the item: the notice writes them with their spaces and the sign of the number added. */
  readonly heal: HealingDice | null;
  readonly userName: string;
  readonly receiverName: string;
  /** The item went to another character. */
  readonly gift: boolean;
  /** The receiver was down: a heal gets them up. */
  readonly woke: boolean;
  /** The units of the item the page holds once it was read again after the use; `null` when that read is not on screen. */
  readonly left: number | null;
}

function lowerFirst(text: string): string {
  return `${text.charAt(0).toLowerCase()}${text.slice(1)}`;
}

/**
 * How the sheet and the log speak of an item by its name: "a Poção de cura", "uma Poção de cura", "bebeu". The catalog carries no
 * gender, so the article follows the name ("Poção…" is feminine). The verb follows the kind the server sent (a potion is drunk,
 * any other item is used); with no kind it is "usou", never a guess from the name.
 */
export function itemWords(
  name: string,
  kind?: ConsumableKind,
): {
  readonly definite: string;
  readonly indefinite: string;
  readonly verb: 'bebeu' | 'usou';
} {
  const potion = /^poç(ão|ões)/i.test(name);
  const feminine = potion || article(name) === 'a';
  const drunk = kind === 'potion';
  return {
    definite: `${feminine ? 'a' : 'o'} ${name}`,
    indefinite: `${feminine ? 'uma' : 'um'} ${name}`,
    verb: drunk ? 'bebeu' : 'usou',
  };
}

/** "Resta 1 Poção de cura.", "Restam 3 unidades de Poção de cura.", "Era a última unidade." or nothing when the count is not known. */
function leftSentence(itemName: string, left: number | null): string {
  if (left === null) {
    return '';
  }
  if (left <= 0) {
    return 'Era a última unidade.';
  }
  return left === 1 ? `Resta 1 ${itemName}.` : `Restam ${left} unidades de ${itemName}.`;
}

function joined(...parts: readonly string[]): string {
  return parts.filter((p) => p !== '').join(' ');
}

/** The roll written out: "3 + 2 + 2" for the faces and the number added (the total less the faces). */
export function rollSum(faces: readonly number[], total: number): string {
  const modifier = total - faces.reduce((a, n) => a + n, 0);
  const sum = faces.join(' + ');
  return modifier === 0 ? sum : `${sum} ${modifier < 0 ? '−' : '+'} ${Math.abs(modifier)}`;
}

/** The notice of an item that was already used: the sheet is read again, and no unit of it is left. */
export function goneNotice(kind: ConsumableKind): ConsumableNotice {
  const words = kindWords(kind);
  return {
    tone: 'neutral',
    icon: 'info',
    headline: words.goneLead,
    detail: `${words.goneUsed}. A ficha foi atualizada.`,
    text: [],
  };
}

/**
 * The item's text as the notice shows it: the server sends it as lines, the first of which says what the item is ("Poção, muito
 * rara") when more follow. That one is the caption of the box the others sit in.
 */
export function textOf(lines: readonly string[]): {
  readonly text: readonly string[];
  readonly caption?: string;
} {
  return lines.length > 1 ? { caption: lines[0], text: lines.slice(1) } : { text: lines };
}

/**
 * What the server said of a use, as the page words it: the heal with its dice, a potion at full hit points (nothing healed:
 * a neutral notice, no dice), the resistance, or the item's own text. `healed` zero is how the answer says the receiver was
 * already at full hit points.
 */
export function useNotice(res: UseConsumableResponse, input: UseNoticeInput): ConsumableNotice {
  const { definite: the, verb } = itemWords(input.itemName, input.kind);
  const left = leftSentence(input.itemName, input.left);
  switch (res.outcome) {
    case ConsumableUseOutcome.HEALED: {
      if (res.healed <= 0) {
        return {
          tone: 'neutral',
          icon: 'info',
          headline: `${input.receiverName} já estava com os PV cheios.`,
          detail: joined('Nada foi curado, mas a unidade foi gasta.', left),
          text: [],
        };
      }
      return {
        tone: 'good',
        icon: 'favorite',
        headline: `${input.receiverName} recebeu ${res.healed} PV${input.woke ? ' e acorda' : ''}.`,
        detail: joined(
          `${input.heal?.text ?? res.diceText}: ${rollSum(res.faces, res.rollTotal)} = ${res.rollTotal}.`,
          left,
        ),
        text: [],
      };
    }
    case ConsumableUseOutcome.RESISTANCE:
      return {
        tone: 'good',
        icon: 'check_circle',
        headline: `${input.receiverName} ${input.gift ? 'recebeu' : verb} ${the}: ${lowerFirst(effectLine(input.effectPt))}.`,
        detail: '',
        text: [],
      };
    default:
      return {
        tone: 'neutral',
        icon: 'menu_book',
        headline: input.gift
          ? `${input.userName} deu ${the} a ${input.receiverName}.`
          : `${input.userName} ${verb} ${the}.`,
        detail: 'O app não aplica este efeito: o mestre e você seguem o texto.',
        ...textOf(res.text),
      };
  }
}
