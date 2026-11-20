import { expect, type Page } from '@playwright/test';

import { createSubclassRPC, listEntries } from './classes-support';
import { createEntryRPC } from './content-support';
import { callRPC } from './support';
import { campaignWithEmptyPlayer } from './table-rules-support';

// The option rows of a subclass feature (#580, board 580-03): "Truques de lâmina" with five options, four that ask something. The
// second asks for the first (a sibling), so the saved siblings, the summary lines and the "Pede" lines have something to say.

export const OPTION_NAMES = ['Golpe do Corvo', 'Asa Partida', 'Bico de Ferro', 'Pena Negra', 'Grito do Bando'];
const TEXTS = ['Soma o dado de manobra ao dano.', 'Soma o dado ao ataque.', 'Soma o dado ao dano de novo.', 'Reduz um golpe que recebe.', 'Um aliado ganha o dado.'];

export interface OptionRowsTable {
  campaignId: string;
  subclass: string;
  /** The keys the server made for the options, in order. */
  keys: string[];
  /** The key of the table option "Olhar de Brasa". */
  brasa: string;
}

export async function optionRowsTable(master: Page, player: Page, name: string): Promise<OptionRowsTable> {
  const campaignId = await campaignWithEmptyPlayer(master, player, name);
  const brasa = await createEntryRPC(master, campaignId, 'tableOption', { namePt: 'Olhar de Brasa', descPt: ['Você enxerga no escuro.'], set: 'invocation' });
  const subclass = await createSubclassRPC(master, campaignId, {
    namePt: 'Guarda do Corvo',
    classKey: 'class:fighter',
    descPt: ['Quem aprendeu a lâmina com os corvos.'],
    levels: [{ level: 3, features: [{
      namePt: 'Truques de lâmina',
      descPt: ['Escolha entre as opções da lista.'],
      effects: [{ type: 'choice', choice: 'feature', count: 3 }],
      options: OPTION_NAMES.map((n, i) => ({ namePt: n, descPt: [TEXTS[i]] })),
    }] }],
  });
  const entry = (await listEntries(master, campaignId)).find((e) => e.key === subclass)!;
  const feature = entry.tableSubclass.levels[0].features[0];
  const keys = feature.options.map((o: { key: string }) => o.key);
  const asking = [
    undefined,
    { classLevel: { classKey: 'class:fighter', level: 7 }, requiresOptionKeys: [keys[0]] },
    { classLevel: { classKey: 'class:fighter', level: 10 } },
    { requiresOptionKeys: [keys[0]] },
    { classLevel: { classKey: 'class:fighter', level: 15 } },
  ];
  const options = feature.options.map((o: Record<string, unknown>, i: number) => (asking[i] ? { ...o, prerequisite: asking[i] } : o));
  const res = await callRPC(master, 'meurpg.rules.v1.TableContentService/UpdateTableEntry', {
    campaignId,
    key: subclass,
    expectedRevision: entry.revision,
    tableSubclass: { ...entry.tableSubclass, levels: [{ ...entry.tableSubclass.levels[0], features: [{ ...feature, options }] }] },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return { campaignId, subclass, keys, brasa };
}
