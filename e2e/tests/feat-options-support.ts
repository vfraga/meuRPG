import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type Locator, type Page } from '@playwright/test';

import { createEntryRPC } from './content-support';
import { setTableRulesRPC } from './table-rules-support';

// Setup for the a11y rows of the feat editor's options and named choices (MR-025, RN-23): feats with a list of options, a whole
// SRD set offered, a named ability choice and a named damage type choice, and the formula that reads a choice. Everything comes
// through the API, so a row proves its screen and not the editor's writes; invented names, as the other content specs have.

/** "Estilo do Veterano": a choice of one of four options the feat lists itself. */
export function veteranFeatBody(): Record<string, unknown> {
  return {
    namePt: 'Estilo do Veterano',
    descPt: ['Anos de campanha deixaram um jeito próprio de lutar. Escolha um estilo ao pegar o talento.'],
    effects: [{ type: 'choice', choice: 'feature', count: 1 }],
    options: [
      { namePt: 'Guarda Alta', descPt: ['Você mantém a lâmina erguida.'], effects: [{ type: 'modifier', target: 'ac', mode: 'add', value: '1' }] },
      {
        namePt: 'Mão Firme',
        descPt: ['Seu golpe cai sempre no mesmo ponto: +1 no dano com armas corpo a corpo.'],
        effects: [{ type: 'modifier', target: 'damage.weapon.melee', mode: 'add', value: '1' }],
      },
      {
        namePt: 'Olho de Falcão',
        descPt: ['Você mira antes de soltar: +2 no ataque com armas à distância.'],
        effects: [{ type: 'modifier', target: 'attack.weapon.ranged', mode: 'add', value: '2' }],
      },
      { namePt: 'Vigia do Acampamento', descPt: ['Nada se move sem que você note.'], effects: [{ type: 'proficiency', proficiency: 'skill:perception' }] },
    ],
  };
}

/** "Livro de Estilos": a choice of one of 60 options, the most a feat lists, so "Adicionar uma opção" waits. */
export function limitFeatBody(): Record<string, unknown> {
  return {
    namePt: 'Livro de Estilos',
    descPt: ['Um livro de estilos de luta, um para cada gosto. Escolha um ao pegar o talento.'],
    effects: [{ type: 'choice', choice: 'feature', count: 1 }],
    options: Array.from({ length: 60 }, (_, i) => ({
      namePt: `Estilo ${i + 1}`,
      descPt: [`O estilo ${i + 1} do livro.`],
      effects: [{ type: 'modifier', target: 'ac', mode: 'add', value: '1' }],
    })),
  };
}

/** "Escola de Armas": the choice offers a whole SRD set, the fighting styles. */
export function schoolFeatBody(): Record<string, unknown> {
  return {
    namePt: 'Escola de Armas',
    descPt: ['Você treinou numa escola de armas. Escolha um estilo de luta ao pegar o talento.'],
    effects: [{ type: 'choice', choice: 'feature', count: 1, set: 'fighting_style' }],
  };
}

/** "Resiliente": one named ability choice, then +1 in that ability (up to 20) and proficiency in its saving throw. */
export function resilientFeatBody(): Record<string, unknown> {
  return {
    namePt: 'Resiliente',
    descPt: ['Escolha uma habilidade: ela sobe 1 ponto, até 20, e você ganha proficiência nos testes de resistência dela.'],
    effects: [
      { type: 'choice', choice: 'ability', count: 1, name: 'habilidade' },
      { type: 'modifier', target: 'score.@habilidade', mode: 'add', value: '1', cap: 20 },
      { type: 'proficiency', proficiency: 'save.@habilidade' },
    ],
  };
}

/** "Afinidade Elemental": a named damage type choice among five, then a minimum of 2 on the damage dice of that type. */
export function affinityFeatBody(): Record<string, unknown> {
  return {
    namePt: 'Afinidade Elemental',
    descPt: ['Escolha um elemento. Nos dados de dano desse tipo, um 1 conta como 2.'],
    prerequisite: { spellcasting: true },
    effects: [
      { type: 'choice', choice: 'damage_type', count: 1, name: 'tipo_de_dano', from: ['damage-type:acid', 'damage-type:lightning', 'damage-type:fire', 'damage-type:cold', 'damage-type:thunder'] },
      { type: 'roll_minimum', rolls: ['damage'], value: '2', damageType: 'damage-type:@tipo_de_dano' },
    ],
  };
}

/** "Voz de Comando": a named ability choice and a note whose number is a formula that reads it. */
export function commandFeatBody(): Record<string, unknown> {
  return {
    namePt: 'Voz de Comando',
    descPt: ['Sua voz faz o grupo se firmar. A CD é a do talento.'],
    effects: [
      { type: 'choice', choice: 'ability', count: 1, name: 'habilidade' },
      { type: 'note', textPt: 'CD da Voz de Comando: {value}', value: '8 + prof() + mod("@habilidade")' },
    ],
  };
}

export interface FeatOptionsTable {
  veteran: string;
  limit: string;
  school: string;
  resilient: string;
  affinity: string;
  command: string;
}

/** The six feats in a campaign with "Talentos" on. */
export async function createFeatOptionsRPC(master: Page, campaignId: string): Promise<FeatOptionsTable> {
  await setTableRulesRPC(master, campaignId, { featsAllowed: true });
  return {
    veteran: await createEntryRPC(master, campaignId, 'tableFeat', veteranFeatBody()),
    limit: await createEntryRPC(master, campaignId, 'tableFeat', limitFeatBody()),
    school: await createEntryRPC(master, campaignId, 'tableFeat', schoolFeatBody()),
    resilient: await createEntryRPC(master, campaignId, 'tableFeat', resilientFeatBody()),
    affinity: await createEntryRPC(master, campaignId, 'tableFeat', affinityFeatBody()),
    command: await createEntryRPC(master, campaignId, 'tableFeat', commandFeatBody()),
  };
}

/** The viewport of each width, a phone, a tablet and a laptop: a screen is judged at the fold a person sees. */
const HEIGHTS: Record<number, number> = { 390: 844, 768: 1024, 1280: 800 };

export function featViewport(width: number): { width: number; height: number } {
  return { width, height: HEIGHTS[width] ?? 800 };
}

/** The folder the screenshots go to when `E2E_SHOTS_DIR` names one; otherwise nothing is written. */
const shotsDir = process.env.E2E_SHOTS_DIR;

/** The device scale of a context that takes screenshots: 2 when they are asked for, else the browser's own. */
export const featShotScale = shotsDir ? { deviceScaleFactor: 2 } : {};

/**
 * Saves the viewport as `<screen>-<width>-<light|dark>.png` when `E2E_SHOTS_DIR` is set; a normal run writes nothing.
 * With `fullPage`, a screen that scrolls on purpose is saved whole as `<screen>-full-…`.
 * On the phone it also writes the height of the whole page to `heights-<screen>.json`, so a read screen's length is on record.
 */
export async function shootFeat(page: Page, screen: string, width: number, scheme: 'light' | 'dark', fullPage = false): Promise<void> {
  if (!shotsDir) {
    return;
  }
  mkdirSync(shotsDir, { recursive: true });
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: join(shotsDir, `${screen}${fullPage ? '-full' : ''}-${width}-${scheme}.png`), fullPage });
  if (width === 390 && scheme === 'light') {
    // One file for each screen, so the parallel rows never write the same one; the run's caller merges them into heights.json.
    const height = await page.evaluate(() => document.documentElement.scrollHeight);
    writeFileSync(join(shotsDir, `heights-${screen}.json`), `${JSON.stringify({ [screen]: height })}\n`);
  }
}

/** Opens a rich select (the browser's customizable `<select>`) with room below it for the list, and waits until it is drawn. */
export async function openSelect(select: Locator): Promise<void> {
  // The list opens below the field: the field goes near the top of the viewport, under the page's bar, so the whole list fits.
  await select.evaluate((el) => {
    el.scrollIntoView({ block: 'start' });
    window.scrollBy(0, -120);
  });
  await select.click();
  await expect.poll(() => select.evaluate((el) => el.matches(':open'))).toBe(true);
}

/** Asserts that a line of the open list is drawn whole inside the viewport (a list cut by the fold fails, and says where the line is). */
export async function expectInViewport(page: Page, line: Locator): Promise<void> {
  await expect(line).toHaveCount(1);
  const viewport = page.viewportSize()!;
  await expect
    .poll(async () => {
      const box = await line.boundingBox();
      const inside = box !== null && box.y >= 0 && box.y + box.height <= viewport.height;
      return `${inside ? 'inside' : 'outside'} ${JSON.stringify({ box, viewport })}`;
    })
    .toMatch(/^inside /);
}
