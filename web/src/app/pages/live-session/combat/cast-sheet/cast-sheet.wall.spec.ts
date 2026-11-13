import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { By } from '@angular/platform-browser';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  CombatantKind,
  PreviewSpellAreaResponseSchema,
  SpellTargetsSchema,
  WallSide,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy, SpellAreaShape } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { CastSheet, type CastSheetData } from './cast-sheet';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const pensantus = combatant({
  id: 'p',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  mine: true,
  col: 2,
  row: 4,
});
const kobold = combatant({ id: 'k1', label: 'Kobold 1', col: 6, row: 3 });

const wallTargets = create(SpellTargetsSchema, {
  spellKey: 'spell:wall-of-fire',
  placement: AreaPlacement.POINT,
  areaShape: SpellAreaShape.LINE,
  areaSizeFt: 60,
  areaWidthFt: 5,
  rangeFt: 120,
  targets: [{ combatantId: 'k1', label: 'Kobold 1', distanceFt: 20 }],
});

const wallOfFire = {
  spell: { level: 4 },
  damage: [{ damageTypeKey: 'fire', bySlotLevel: { 4: '5d8', 5: '6d8' }, byCharacterLevel: {} }],
  healBySlotLevel: {},
  higherLevel: [],
};

const laid = create(PreviewSpellAreaResponseSchema, {
  origin: { col: 4, row: 2 },
  squares: [
    { col: 4, row: 2 },
    { col: 5, row: 2 },
  ],
  burningSquares: [{ col: 4, row: 3 }],
  targets: [{ combatantId: 'k1', label: 'Kobold 1', distanceFt: 5 }],
  coverCounts: true,
  lengthFt: 60,
});

async function open(over: { castSpell?: ReturnType<typeof vi.fn> } = {}) {
  TestBed.resetTestingModule();
  const previewSpellArea = vi.fn().mockResolvedValue(laid);
  const castSpell =
    over.castSpell ??
    vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [pensantus, kobold] }),
      cast: { targets: [], pendingDamages: [], metamagicKeys: [], slot: { level: 4, pact: false } },
      summoned: [],
    });
  const data = {
    campaignId: 'c',
    encounterId: 'enc',
    casterId: 'p',
    round: 2,
    spellKey: 'spell:wall-of-fire',
    name: 'Muralha de Fogo',
    level: 4,
    concentration: true,
    cantripDice: '',
    economy: ActionEconomy.ACTION,
    slots: [{ level: 4, free: 1, pact: false }],
    usage: [{ level: 4, total: 1, used: 0 }],
    pact: null,
    targets: wallTargets,
    shieldFree: null,
    shieldName: '',
    attackBonus: 0,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state: {
      encounter: signal(encounter({ combatants: [pensantus, kobold] })),
      apply: vi.fn(),
    },
    map: {
      image: { url: 'cave.png', width: 1200, height: 800 },
      mapName: 'A caverna',
      columns: 12,
      rows: 8,
      layers: null,
      fog: null,
    },
  } as unknown as CastSheetData;
  TestBed.configureTestingModule({
    providers: [
      { provide: CombatClient, useValue: fakeCombatClient({ castSpell, previewSpellArea }) },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(wallOfFire) } },
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: vi.fn() } },
    ],
  });
  const fixture = TestBed.createComponent(CastSheet);
  const settle = async () => {
    for (let i = 0; i < 3; i++) {
      fixture.detectChanges();
      await fixture.whenStable();
    }
    fixture.detectChanges();
  };
  await settle();
  const el = fixture.nativeElement as HTMLElement;
  const button = (label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
      (b) => b.getAttribute('aria-label') === label || plain(b.textContent).includes(label),
    );
  const radio = (group: string, word: string) =>
    Array.from(
      el.querySelectorAll<HTMLInputElement>(`[role="radiogroup"][aria-label="${group}"] input`),
    ).find((r) => plain(r.closest('label')?.textContent).includes(word));
  const placeAt = async (col: number, row: number) => {
    const map = fixture.debugElement.query(By.directive(AreaMap)).componentInstance as AreaMap;
    map.place.emit({ kind: 'point', square: { col, row } });
    await settle();
  };
  return { fixture, el, button, radio, placeAt, settle, castSpell, previewSpellArea };
}

const slot = { level: 4, pact: false };
const lineWall = {
  ring: false,
  direction: { dx: 1, dy: 0 },
  lengthFt: 60,
  side: WallSide.RIGHT,
};
/** What the first preview asks with: no side is picked, so the server is asked with a provisional one. */
const unpicked = { ...lineWall, side: WallSide.LEFT };

describe('CastSheet: a spell that lays a wall (Muralha de Fogo)', () => {
  it('opens on "Onde fica" with the shape, the direction, the length and the side, each labelled, no side picked', async () => {
    const { el, button, radio, previewSpellArea } = await open();
    expect(plain(el.textContent)).toContain('Onde fica');
    expect(plain(el.textContent)).not.toContain('Onde ela explode');
    expect(el.querySelector('[role="radiogroup"][aria-label="Forma da muralha"]')).not.toBeNull();
    expect(el.querySelectorAll('[aria-label^="Direção da muralha"] button')).toHaveLength(8);
    expect(plain(el.querySelector('output')?.textContent)).toContain('18 m');
    expect(radio('Lado que queima', 'Acima')?.checked).toBe(false);
    expect(radio('Lado que queima', 'Abaixo')?.checked).toBe(false);
    expect(plain(el.textContent)).toContain('Escolha o lado');
    expect(plain(el.textContent)).toContain(
      'A seta no mapa mostra o lado: 5d8 de fogo para quem termina o turno ali, até 3 m.',
    );
    expect(button('Próximo: quem está nela')?.getAttribute('aria-disabled')).toBe('true');
    expect(plain(el.querySelector('.missing')?.textContent)).toContain(
      'Toque no mapa para marcar onde a muralha fica.',
    );
    expect(previewSpellArea).not.toHaveBeenCalled();
  });

  it('shows the two steps side by side, "1 Onde fica" now and "2 Quem está nela" next, and the heading is the current one', async () => {
    const { el, button, radio, placeAt, settle } = await open();
    const steps = () => plain(el.querySelector('app-area-step')?.textContent);
    expect(steps()).toContain('Onde fica');
    expect(steps()).toContain('Quem está nela');
    expect(plain(el.querySelector('app-area-step h3')?.textContent)).toMatch(/^1\s*Onde fica$/);
    await placeAt(4, 2);
    radio('Lado que queima', 'Abaixo')!.click();
    await settle();
    button('Próximo: quem está nela')!.click();
    await settle();
    expect(plain(el.querySelector('app-area-step h3')?.textContent)).toMatch(
      /^2\s*Quem está nela$/,
    );
    expect(steps()).toContain('Onde fica');
    expect(document.activeElement?.closest('app-area-step')).not.toBeNull();
  });

  it('pins "Cancelar" and "Próximo: quem está nela" in the footer, side by side and of one size, outside the scrolling body', async () => {
    const { el } = await open();
    const foot = el.querySelector('.frame__foot')!;
    expect(foot.closest('.frame')).not.toBeNull();
    expect(el.querySelector('.frame__body')?.contains(foot)).toBe(false);
    const row = foot.querySelector('.actions')!;
    expect(row.classList.contains('actions--pair')).toBe(true);
    expect(row.classList.contains('actions--stack')).toBe(false);
    const labels = Array.from(row.querySelectorAll('button')).map((b) => plain(b.textContent));
    expect(labels.sort()).toEqual(['Cancelar', 'Próximo: quem está nela']);
    expect(row.querySelector('button.cancel')?.className).toContain('outlined');
    expect(row.querySelector('button.cast')?.className).toContain('unelevated');
  });

  it('sums the ring up from the server: its diameter, the squares laid and who it names in the wall', async () => {
    const { el, button, radio, settle, placeAt, previewSpellArea } = await open();
    previewSpellArea.mockResolvedValue(
      create(PreviewSpellAreaResponseSchema, {
        origin: { col: 4, row: 2 },
        squares: Array.from({ length: 16 }, (_, i) => ({ col: i % 4, row: Math.floor(i / 4) })),
        burningSquares: [{ col: 5, row: 5 }],
        targets: [{ combatantId: 'k1', label: 'Kobold 1', distanceFt: 5 }],
        coverCounts: true,
        lengthFt: 20,
      }),
    );
    expect(el.querySelector('.wall-summary')).toBeNull();
    radio('Forma da muralha', 'Anel')!.click();
    await settle();
    await placeAt(4, 2);
    const card = el.querySelector('.wall-summary')!;
    expect(plain(card.querySelector('strong')?.textContent)).toBe('Anel de 6 m · 16 quadrados');
    expect(plain(card.textContent)).toContain('Na muralha agora: Kobold 1');
    expect(button('Próximo: quem está nela')).toBeDefined();
  });

  it('draws no summary for a straight wall', async () => {
    const { el, placeAt } = await open();
    await placeAt(4, 2);
    expect(el.querySelector('.wall-summary')).toBeNull();
  });

  it('previews the wall with a provisional side, draws no burning side, and keeps "Próximo" off until a side is picked', async () => {
    const { el, button, radio, placeAt, settle, previewSpellArea } = await open();
    await placeAt(4, 2);
    expect(previewSpellArea).toHaveBeenLastCalledWith(
      'c',
      'enc',
      'p',
      'spell:wall-of-fire',
      slot,
      { origin: { col: 4, row: 2 } },
      '',
      unpicked,
    );
    expect(el.querySelector('app-zone-layer .zl__burn')).toBeNull();
    expect(button('Próximo: quem está nela')?.getAttribute('aria-disabled')).toBe('true');
    expect(plain(el.querySelector('.missing')?.textContent)).toContain(
      'Escolha o lado que queima.',
    );
    button('Próximo: quem está nela')!.click();
    await settle();
    expect(plain(el.textContent)).toContain('Onde fica');
    radio('Lado que queima', 'Abaixo')!.click();
    await settle();
    expect(button('Próximo: quem está nela')?.getAttribute('aria-disabled')).not.toBe('true');
    expect(el.querySelector('app-zone-layer .zl__burn')).not.toBeNull();
  });

  it('asks again at every change of the wall, and a turn takes the picked side back', async () => {
    const { button, radio, placeAt, settle, previewSpellArea } = await open();
    await placeAt(4, 2);
    radio('Lado que queima', 'Abaixo')!.click();
    await settle();
    button('Diminuir o comprimento em 1,5 m')!.click();
    await settle();
    expect(previewSpellArea).toHaveBeenCalledTimes(3);
    expect((previewSpellArea.mock.calls.at(-1) as unknown[])[7]).toEqual({
      ...lineWall,
      lengthFt: 55,
    });
    button('Para o sul')!.click();
    await settle();
    expect(previewSpellArea).toHaveBeenCalledTimes(4);
    expect((previewSpellArea.mock.calls.at(-1) as unknown[])[7]).toEqual({
      ring: false,
      direction: { dx: 0, dy: 1 },
      lengthFt: 55,
      side: WallSide.LEFT,
    });
    expect(radio('Lado que queima', 'À direita')?.checked).toBe(false);
    expect(radio('Lado que queima', 'À esquerda')?.checked).toBe(false);
  });

  it('says it is working out the wall while the preview is on its way', async () => {
    let answer!: (r: typeof laid) => void;
    const { el, placeAt, settle, previewSpellArea } = await open();
    previewSpellArea.mockReturnValueOnce(new Promise<typeof laid>((r) => (answer = r)));
    await placeAt(4, 2);
    expect(plain(el.textContent)).toContain('Calculando onde a muralha fica…');
    answer(laid);
    await settle();
    expect(plain(el.textContent)).not.toContain('Calculando onde a muralha fica…');
  });

  it('warns when the wall stopped at a wall of the map', async () => {
    const { el, placeAt } = await open();
    // The answer holds 2 squares; the 18 m wall holds 12 on open floor.
    await placeAt(4, 2);
    expect(plain(el.textContent)).toContain(
      'A muralha para na parede: ficou com 3 m (2 quadrados) em vez de 18 m.',
    );
  });

  it('a ring has no direction pad and no side of a line, only Dentro and Fora, none picked', async () => {
    const { el, radio, placeAt, settle, previewSpellArea } = await open();
    radio('Forma da muralha', 'Anel')!.click();
    await settle();
    expect(el.querySelector('[aria-label^="Direção da muralha"]')).toBeNull();
    expect(radio('Lado que queima', 'Acima')).toBeUndefined();
    expect(radio('Lado que queima', 'Dentro')?.checked).toBe(false);
    expect(plain(el.querySelector('output')?.textContent)).toContain('6 m');
    await placeAt(5, 5);
    expect((previewSpellArea.mock.calls.at(-1) as unknown[])[7]).toEqual({
      ring: true,
      direction: null,
      lengthFt: 20,
      side: WallSide.INNER,
    });
  });

  it('goes to who is inside with "Próximo", and casts with the same wall and the start', async () => {
    const { el, button, radio, placeAt, settle, castSpell } = await open();
    await placeAt(4, 2);
    radio('Lado que queima', 'Abaixo')!.click();
    await settle();
    button('Próximo: quem está nela')!.click();
    await settle();
    expect(plain(el.textContent)).toContain('Quem está nela');
    expect(el.querySelector('app-wall-controls')).toBeNull();
    button('Conjurar Muralha de Fogo')!.click();
    await settle();
    expect(castSpell).toHaveBeenCalledTimes(1);
    const call = castSpell.mock.calls[0] as unknown[];
    expect(call[12]).toEqual({ area: { origin: { col: 4, row: 2 } }, wall: lineWall });
  });

  it('"Mudar o local" brings the controls back with the wall as it was', async () => {
    const { el, button, radio, placeAt, settle } = await open();
    await placeAt(4, 2);
    radio('Lado que queima', 'Abaixo')!.click();
    await settle();
    button('Próximo: quem está nela')!.click();
    await settle();
    button('Mudar o local')!.click();
    await settle();
    expect(plain(el.textContent)).toContain('Onde fica');
    expect(radio('Lado que queima', 'Abaixo')?.checked).toBe(true);
  });

  it('retries a lost cast with the same key, and takes a new key when the wall changed', async () => {
    const castSpell = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('lost', Code.Unavailable))
      .mockRejectedValueOnce(new ConnectError('lost', Code.Unavailable))
      .mockResolvedValue({
        encounter: encounter(),
        cast: { targets: [], pendingDamages: [], metamagicKeys: [] },
        summoned: [],
      });
    const { button, radio, placeAt, settle } = await open({ castSpell });
    await placeAt(4, 2);
    radio('Lado que queima', 'Abaixo')!.click();
    await settle();
    button('Próximo: quem está nela')!.click();
    await settle();
    button('Conjurar Muralha de Fogo')!.click();
    await settle();
    button('Conjurar Muralha de Fogo')!.click();
    await settle();
    expect(castSpell).toHaveBeenCalledTimes(2);
    const calls = castSpell.mock.calls as unknown[][];
    expect(calls[1][7]).toBe(calls[0][7]);
    expect((calls[1][12] as { wall: unknown }).wall).toEqual(lineWall);
    button('Mudar o local')!.click();
    await settle();
    radio('Lado que queima', 'Acima')!.click();
    await settle();
    button('Próximo: quem está nela')!.click();
    await settle();
    button('Conjurar Muralha de Fogo')!.click();
    await settle();
    const third = castSpell.mock.calls[2] as unknown[];
    expect(third[7]).not.toBe(calls[0][7]);
    expect((third[12] as { wall: { side: WallSide } }).wall.side).toBe(WallSide.LEFT);
  });
});
