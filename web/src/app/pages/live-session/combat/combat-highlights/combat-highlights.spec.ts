import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import {
  CharacterHighlightsSchema,
  GetCombatHighlightsResponseSchema,
  HighlightCategorySchema,
  HighlightKind,
  HighlightWinnerSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import type { CombatantInfo } from '../combat-info';
import { CombatHighlights } from './combat-highlights';
import { HighlightsCard } from './highlights-card';
import { nextTurn } from '../../../../../test-settle';

const nbsp = ' ';
/** The text of an element as read, without the icons' ligature names. */
const flat = (e: Element | null | undefined) => {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return copy.textContent
    ?.replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
};

function category(kind: HighlightKind, value: number, ...winners: [string, string][]) {
  return create(HighlightCategorySchema, {
    kind,
    value,
    winners: winners.map(([characterId, name]) =>
      create(HighlightWinnerSchema, { characterId, name }),
    ),
  });
}

/** The artboards' combat. */
const fight = {
  categories: [
    category(HighlightKind.MOST_DAMAGE, 23, ['toren', 'Toren']),
    category(HighlightKind.TANK, 24, ['brisa', 'Brisa']),
    category(HighlightKind.FINAL_BLOW, 2, ['pens', 'Pensantus'], ['toren', 'Toren']),
  ],
  characters: [
    create(CharacterHighlightsSchema, {
      characterId: 'pens',
      name: 'Pensantus',
      damageDealt: 17,
      finalBlows: 2,
      criticalHits: 3,
    }),
    create(CharacterHighlightsSchema, {
      characterId: 'toren',
      name: 'Toren',
      damageDealt: 23,
      damageTaken: 10,
      finalBlows: 2,
    }),
    create(CharacterHighlightsSchema, {
      characterId: 'brisa',
      name: 'Brisa',
      damageDealt: 8,
      damageTaken: 24,
    }),
  ],
};
const masterAnswer = create(GetCombatHighlightsResponseSchema, fight);
/** What a player gets: the same categories and one row of the table, their own (Pensantus's). */
const playerAnswer = create(GetCombatHighlightsResponseSchema, {
  categories: fight.categories,
  characters: [fight.characters[0]],
});

const info = new Map<string, CombatantInfo>([
  ['toren', { classSummary: '', playerName: 'Caio', kindLabel: '', raceName: '' }],
  ['brisa', { classSummary: '', playerName: 'Lia', kindLabel: '', raceName: '' }],
  ['pens', { classSummary: '', playerName: 'Vinicius', kindLabel: '', raceName: '' }],
]);

async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
  for (let i = 0; i < 3; i++) {
    await fixture.whenStable();
    await nextTurn();
    fixture.detectChanges();
  }
}

describe("CombatHighlights, the master's panel (E8-11)", () => {
  const highlights = vi.fn();

  async function setup(answer = masterAnswer) {
    highlights.mockReset().mockResolvedValue(answer);
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: { highlights } }],
    });
    const fixture = TestBed.createComponent(CombatHighlights);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('encounterId', 'enc-1');
    fixture.componentRef.setInput('info', info);
    fixture.detectChanges();
    await settle(fixture);
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const tiles = (el: HTMLElement) => Array.from(el.querySelectorAll('.tile'));

  it('asks for the combat\'s highlights and titles the panel "Destaques do combate"', async () => {
    const { el } = await setup();
    expect(highlights).toHaveBeenCalledWith('c1', 'enc-1');
    expect(flat(el.querySelector('h2'))).toBe('Destaques do combate');
    expect(flat(el.querySelector('.hl__lead'))).toContain('Os jogadores veem o mesmo cartão.');
  });

  it('shows one tile per category with a winner, the number as the largest text, and leaves the others out', async () => {
    const { el } = await setup();
    expect(tiles(el).map((t) => flat(t.querySelector('.tile__label')))).toEqual([
      'Mais dano causado',
      'Tanque',
      'Golpe final',
    ]);
    expect(tiles(el).map((t) => t.querySelector('.tile__value')?.textContent)).toEqual([
      `23${nbsp}de${nbsp}dano`,
      `24${nbsp}de${nbsp}dano`,
      `2${nbsp}inimigos`,
    ]);
    expect(flat(tiles(el)[1].querySelector('.tile__sub'))).toBe('mais dano recebido');
    expect(el.textContent).not.toContain('Mais cura');
    expect(el.textContent).not.toContain('Acertos críticos Acertos'); // no tile; only the table's header names it
    expect(
      tiles(el).some((t) => flat(t.querySelector('.tile__label')) === 'Acertos críticos'),
    ).toBe(false);
  });

  it('names the player behind a winner, and everyone in a tie with "cada um"', async () => {
    const { el } = await setup();
    expect(tiles(el).map((t) => flat(t.querySelector('.tile__names')))).toEqual([
      'Toren',
      'Brisa',
      'Pensantus e Toren',
    ]);
    expect(tiles(el).map((t) => flat(t.querySelector('.tile__who')))).toEqual([
      'de Caio',
      'de Lia',
      'cada um · de Vinicius e Caio',
    ]);
  });

  it('shows "Números de cada jogador", every number of every character, zeros included', async () => {
    const { el } = await setup();
    expect(flat(el.querySelector('h3'))).toBe('Números de cada jogador');
    const rows = Array.from(el.querySelectorAll('.tbl__row:not(.tbl__row--head)'));
    expect(
      rows.map((r) => Array.from(r.querySelectorAll('.tbl__num')).map((n) => n.textContent)),
    ).toEqual([
      ['17', '0', '0', '2', '3'],
      ['23', '0', '10', '2', '0'],
      ['8', '0', '24', '0', '0'],
    ]);
    expect(rows.map((r) => flat(r.querySelector('b')))).toEqual(['Pensantus', 'Toren', 'Brisa']);
    expect(flat(el.querySelector('.tbl__row--head'))).toBe(
      'PersonagemDano causadoCuraDano recebidoGolpes finaisAcertos críticos',
    );
  });

  it('says so when nobody hurt or healed anyone, and keeps the table', async () => {
    const { el } = await setup(
      create(GetCombatHighlightsResponseSchema, { characters: fight.characters }),
    );
    expect(tiles(el)).toHaveLength(0);
    expect(flat(el.querySelector('.hl__note'))).toBe(
      'Ninguém causou, curou ou sofreu dano neste combate.',
    );
    expect(el.querySelectorAll('.tbl__row')).toHaveLength(4);
  });

  it('shows a tie in three, and a heal and a critical when there are some', async () => {
    const { el } = await setup(
      create(GetCombatHighlightsResponseSchema, {
        categories: [
          category(HighlightKind.MOST_HEALING, 9, ['brisa', 'Brisa'], ['toren', 'Toren']),
          category(HighlightKind.CRITICAL_HITS, 1, ['toren', 'Toren']),
        ],
      }),
    );
    expect(tiles(el).map((t) => t.querySelector('.tile__value')?.textContent)).toEqual([
      `9${nbsp}de${nbsp}cura`,
      `1${nbsp}crítico`,
    ]);
    expect(flat(tiles(el)[0].querySelector('.tile__names'))).toBe('Brisa e Toren');
    expect(flat(tiles(el)[0].querySelector('.tile__who'))).toBe('cada um · de Lia e Caio');
    expect(el.querySelector('.tbl')).toBeNull();
  });

  it('says why it could not load, and tries again', async () => {
    highlights
      .mockReset()
      .mockRejectedValueOnce(new ConnectError('x', Code.Unavailable))
      .mockResolvedValue(masterAnswer);
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: { highlights } }],
    });
    const fixture = TestBed.createComponent(CombatHighlights);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('encounterId', 'enc-1');
    fixture.detectChanges();
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(flat(el.querySelector('[role="alert"]'))).toContain(
      'Não deu para carregar os destaques',
    );
    el.querySelector<HTMLButtonElement>('.hl__retry')!.click();
    await settle(fixture);
    expect(tiles(el)).toHaveLength(3);
    expect(el.querySelector('[role="alert"]')).toBeNull();
  });
});

describe("HighlightsCard, the players' card (E8-11)", () => {
  const highlights = vi.fn();

  async function setup(answer = playerAnswer, characterId = 'pens', name = 'Pensantus') {
    highlights.mockReset().mockResolvedValue(answer);
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: { highlights } }],
    });
    const fixture = TestBed.createComponent(HighlightsCard);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('encounterId', 'enc-1');
    fixture.componentRef.setInput('subtitle', 'Emboscada na estrada · 4 rodadas');
    fixture.componentRef.setInput('characterId', characterId);
    fixture.componentRef.setInput('characterName', name);
    fixture.detectChanges();
    await settle(fixture);
    const closed = vi.fn();
    fixture.componentInstance.closed.subscribe(closed);
    return { fixture, el: fixture.nativeElement as HTMLElement, closed };
  }

  const rows = (el: HTMLElement) => Array.from(el.querySelectorAll('.row'));

  it('says the combat ended, in a polite live region, and names the combat and its rounds', async () => {
    const { el } = await setup();
    expect(flat(el.querySelector('.card__tag'))).toBe('Combate encerrado');
    expect(el.querySelector('[role="status"] h2')?.textContent).toBe('O combate acabou');
    expect(flat(el.querySelector('.card__sub'))).toBe('Emboscada na estrada · 4 rodadas');
  });

  it('lists the categories with their number and names, a tie naming everyone', async () => {
    const { el } = await setup();
    expect(rows(el).map((r) => flat(r.querySelector('.row__label')))).toEqual([
      'Mais dano causado',
      'Tanque',
      'Golpe final Você',
    ]);
    expect(rows(el).map((r) => r.querySelector('.row__value')?.textContent)).toEqual([
      `23${nbsp}de${nbsp}dano`,
      `24${nbsp}de${nbsp}dano`,
      `2${nbsp}inimigos`,
    ]);
    expect(rows(el).map((r) => flat(r.querySelector('.row__names')))).toEqual([
      'Toren',
      'Brisa',
      'Pensantus e Toren',
    ]);
    expect(flat(rows(el)[2])).toContain('cada um');
    expect(flat(rows(el)[1].querySelector('.row__small'))).toBe('mais dano recebido');
  });

  it('marks the reader with the word "Você", only on the tiles their character won', async () => {
    const { el } = await setup();
    expect(Array.from(el.querySelectorAll('.row__you')).map((y) => y.textContent)).toEqual([
      'Você',
    ]);
    expect(el.querySelector('.row__you')?.closest('.row')).toBe(rows(el)[2]);
  });

  it('gives "Seu resultado, Pensantus": the five numbers of their own row, zeros included', async () => {
    const { el } = await setup();
    expect(flat(el.querySelectorAll('.card__h')[1])).toBe('Seu resultado, Pensantus');
    expect(Array.from(el.querySelectorAll('.own__tile')).map((t) => flat(t))).toEqual([
      'Dano causado17',
      'Dano recebido0',
      'Golpes finais2',
      'Cura0',
      'Acertos críticos3',
    ]);
  });

  it('shows "Seu resultado" even for someone who won nothing: the zeros are theirs', async () => {
    const own = create(GetCombatHighlightsResponseSchema, {
      categories: fight.categories,
      characters: [create(CharacterHighlightsSchema, { characterId: 'outra', name: 'Outra' })],
    });
    const { el } = await setup(own, 'outra', 'Outra');
    expect(flat(el.querySelectorAll('.card__h')[1])).toBe('Seu resultado, Outra');
    expect(Array.from(el.querySelectorAll('.own__value')).map((v) => v.textContent)).toEqual([
      '0',
      '0',
      '0',
      '0',
      '0',
    ]);
    expect(el.querySelector('.row__you')).toBeNull();
  });

  it("never shows the master's table", async () => {
    const { el } = await setup();
    expect(el.textContent).not.toContain('Números de cada jogador');
    expect(el.querySelector('table, [role="table"]')).toBeNull();
  });

  it('has a ✕ of 44px and "Fechar", both closing it, and no filled button', async () => {
    const { el, closed } = await setup();
    const x = el.querySelector<HTMLButtonElement>('.card__x')!;
    expect(x.getAttribute('aria-label')).toBe('Fechar');
    x.click();
    el.querySelector<HTMLButtonElement>('.card__close')!.click();
    expect(closed).toHaveBeenCalledTimes(2);
    expect(el.querySelector('.mat-mdc-unelevated-button')).toBeNull();
  });

  it('says so when nobody hurt or healed anyone', async () => {
    const { el } = await setup(create(GetCombatHighlightsResponseSchema, {}));
    expect(flat(el.querySelector('.card__none'))).toBe(
      'Ninguém causou, curou ou sofreu dano neste combate.',
    );
  });

  it('shows nothing at all when the highlights cannot be read: it is a bonus, not a screen', async () => {
    highlights.mockReset().mockRejectedValue(new ConnectError('x', Code.Unavailable));
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: { highlights } }],
    });
    const fixture = TestBed.createComponent(HighlightsCard);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('encounterId', 'enc-1');
    fixture.detectChanges();
    await settle(fixture);
    expect((fixture.nativeElement as HTMLElement).querySelector('.card')).toBeNull();
  });
});
