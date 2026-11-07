import { hookSentence } from '../combat/hook-text';
import {
  type GetRestPreviewResponse,
  RestKind,
  type RestPreview,
} from '../../../gen/meurpg/play/v1/resources_pb';
import { listWords } from './hit-dice-text';

/** The key of the sorcerer's pool (`rules.SorceryPointsKey`): the created slot that vanishes is told next to it. */
const SORCERY_POINTS_KEY = 'sorcery_points';

/** A piece of a confirmation line: the names the board writes in bold are bold. */
export interface RestPart {
  readonly text: string;
  readonly bold: boolean;
}

/** One line of "what comes back": the pieces, read in order. */
export type RestLine = readonly RestPart[];

/** What the master reads before a rest, all of it computed from the preview. */
export interface RestSummary {
  readonly title: string;
  readonly intro: string;
  /** "Já houve um descanso longo nesta sessão…": above the list, for a long rest only. */
  readonly warning: string | null;
  readonly lines: readonly RestLine[];
  /** The characters a long rest does nothing for (0 hit points), one sentence each. */
  readonly noBenefit: readonly string[];
  /** The campaign has no living player character: nothing to list. */
  readonly empty: boolean;
}

/** "Descanso curto", "Descanso longo": the button and the word the screens use for the kind. */
export function restLabel(kind: RestKind): string {
  return kind === RestKind.LONG ? 'Descanso longo' : 'Descanso curto';
}

/** "Descanso longo feito." */
export function restDoneText(kind: RestKind): string {
  return `${restLabel(kind)} feito.`;
}

/** The warning above the list when a long rest was taken in the session already. */
export const LONG_REST_TAKEN_WARNING =
  'Já houve um descanso longo nesta sessão. O SRD permite um a cada 24 horas de jogo: você decide.';

const plain = (text: string): RestPart => ({ text, bold: false });
const bold = (text: string): RestPart => ({ text, bold: true });

/** When a resource comes back, as `ResourceUsageVm.recharge` says it: a resource that also returns on a short rest. */
export type RechargeOf = (
  characterId: string,
  key: string,
) => 'short_rest' | 'long_rest' | undefined;

interface ResourceBack {
  readonly key: string;
  readonly name: string;
  readonly who: { readonly characterId: string; readonly text: string }[];
}

/** Each resource that comes back, by its name from the server, with the characters it comes back to, in the order the
 * preview lists them. The count a character gets is all of its uses: "3 de 3". */
function resourcesBack(characters: readonly RestPreview[]): ResourceBack[] {
  const byKey = new Map<string, ResourceBack>();
  for (const c of characters) {
    for (const r of c.resources) {
      let entry = byKey.get(r.key);
      if (!entry) {
        entry = { key: r.key, name: r.namePt || 'Recurso', who: [] };
        byKey.set(r.key, entry);
      }
      // A resource that gives back one use (restore_one_on_short_rest, restore_one_on_long_rest) says so, not "3 de 3".
      const text =
        r.back > 0
          ? `${c.name} +${r.back} ${r.back === 1 ? 'uso' : 'usos'}`
          : `${c.name} ${r.total} de ${r.total}`;
      entry.who.push({ characterId: c.characterId, text });
    }
  }
  return [...byKey.values()];
}

/** "Fúria: Ragna 3 de 3 · Cura pelas Mãos: Tavo 25 de 25." */
function resourceItems(items: readonly ResourceBack[], lost: ReadonlySet<string>): RestLine {
  const parts: RestPart[] = [];
  items.forEach((item, i) => {
    if (i > 0) {
      parts.push(plain(' · '));
    }
    parts.push(bold(item.name));
    const note = item.key === SORCERY_POINTS_KEY && lost.size > 0 ? ' (o espaço criado some)' : '';
    parts.push(plain(`: ${item.who.map((w) => w.text).join(', ')}${note}`));
  });
  parts.push(plain('.'));
  return parts;
}

/** "Chi (Kai 5 de 5), Canalizar Divindade (Kai 1 de 1): voltam (também voltariam num descanso curto)." */
function alsoShortItems(items: readonly ResourceBack[]): RestLine {
  const parts: RestPart[] = [];
  items.forEach((item, i) => {
    if (i > 0) {
      parts.push(plain(', '));
    }
    parts.push(bold(item.name), plain(` (${item.who.map((w) => w.text).join(', ')})`));
  });
  parts.push(plain(': voltam (também voltariam num descanso curto).'));
  return parts;
}

/** "Ao terminar o descanso longo, Kai: Fôlego da Mesa entrou em ação: ganha 3 PV temporários.": the hooks of the owner (the
 * preview shows the average of their dice; the rest rolls them). */
function hookLines(characters: readonly RestPreview[], long: boolean): RestLine[] {
  const when = `Ao terminar o ${long ? 'descanso longo' : 'descanso curto'}`;
  return characters.flatMap((c) =>
    c.hooks.map((h) => [plain(`${when}, ${c.name}: ${hookSentence(h, true)}.`)]),
  );
}

/** The NPCs of the campaign that kept what they spent (the master's preview alone): a long rest gives back their uses a day and
 * Legendary Resistance, a short rest their rest actions. */
function npcLines(preview: GetRestPreviewResponse): RestLine[] {
  if (preview.npcsBack.length === 0) {
    return [];
  }
  return [
    [
      bold('NPCs da campanha'),
      plain(` recuperam os usos: ${listWords(preview.npcsBack.map((n) => n.name))}.`),
    ],
  ];
}

/** What the confirmation lists: the board's lines, from what the server says each character gets back. `rechargeOf`
 * says, for a long rest, which resources also come back on a short one (the preview itself does not). */
export function restSummary(
  kind: RestKind,
  preview: GetRestPreviewResponse,
  rechargeOf: RechargeOf,
): RestSummary {
  const long = kind === RestKind.LONG;
  const characters = preview.characters.filter((c) => !c.noBenefit);
  const lines: RestLine[] = [];

  if (long) {
    lines.push([plain('Todos os '), bold('PV'), plain(' voltam ao máximo (e a Ajuda acaba).')]);
    lines.push([
      plain('Metade dos '),
      bold('dados de vida'),
      plain(' gastos (no mínimo 1) voltam.'),
    ]);
  } else {
    lines.push([plain('Os jogadores podem gastar '), bold('dados de vida'), plain(' para curar.')]);
  }

  const lostSlots = new Set(characters.filter((c) => c.createdSlotsLost > 0).map((c) => c.name));
  const resources = resourcesBack(characters);
  if (long) {
    const alsoShort = resources.filter((r) =>
      r.who.every((w) => rechargeOf(w.characterId, r.key) === 'short_rest'),
    );
    const longOnly = resources.filter((r) => !alsoShort.includes(r));
    if (longOnly.length > 0) {
      lines.push(resourceItems(longOnly, lostSlots));
    }
    if (lostSlots.size > 0 && !longOnly.some((r) => r.key === SORCERY_POINTS_KEY)) {
      lines.push([bold('espaços criados'), plain(`: ${listWords([...lostSlots])} (somem).`)]);
    }
    if (alsoShort.length > 0) {
      lines.push(alsoShortItems(alsoShort));
    }
    if (characters.some((c) => c.spellSlotsBack.length > 0 || c.pactSlotsBack > 0)) {
      lines.push([bold('espaços de magia'), plain(': todos.')]);
    }
  } else {
    if (resources.length > 0) {
      lines.push(resourceItems(resources, new Set()));
    }
    const pact = characters.filter((c) => c.pactSlotsBack > 0).map((c) => c.name);
    if (pact.length > 0) {
      lines.push([bold('espaços de pacto'), plain(`: ${listWords(pact)}.`)]);
    }
  }

  lines.push(...hookLines(characters, long));

  lines.push(...npcLines(preview));

  return {
    title: `Começar o ${long ? 'descanso longo' : 'descanso curto'}?`,
    intro: `${long ? 'Pelo menos 8 horas' : 'Pelo menos 1 hora'}. Volta, para quem estiver na campanha:`,
    warning: long && preview.longRestAlreadyTaken ? LONG_REST_TAKEN_WARNING : null,
    lines,
    noBenefit: long
      ? preview.characters
          .filter((c) => c.noBenefit)
          .map((c) => `${c.name} está a 0 PV: o descanso longo não faz efeito.`)
      : [],
    empty: preview.characters.length === 0,
  };
}

/** The die sizes of a long rest the master's table chooses for one character: the sizes with dice spent, and how many may
 * come back in all. Only a character with more than one size spent, and a limit below what is spent, has a choice;
 * for the others the server's default (the largest dice first) is the only sensible answer. */
export interface HitDiceChoice {
  readonly characterId: string;
  readonly name: string;
  readonly limit: number;
  readonly sizes: readonly {
    readonly faces: number;
    readonly spent: number;
    readonly start: number;
  }[];
}

/** The characters of a long rest that choose which hit dice come back, with the preview's default as the start. */
export function hitDiceChoices(preview: GetRestPreviewResponse): HitDiceChoice[] {
  return preview.characters
    .filter((c) => !c.noBenefit && c.hitDiceSpent.length > 1)
    .filter((c) => c.hitDiceBackLimit < c.hitDiceSpent.reduce((sum, d) => sum + d.count, 0))
    .map((c) => ({
      characterId: c.characterId,
      name: c.name,
      limit: c.hitDiceBackLimit,
      sizes: c.hitDiceSpent.map((d) => ({
        faces: d.faces,
        spent: d.count,
        start: c.hitDiceBack.find((b) => b.faces === d.faces)?.count ?? 0,
      })),
    }));
}
