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
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  Ability,
  ActionEconomy,
  SpellAreaShape,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

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
const toren = combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, col: 5, row: 4 });
const goblin = combatant({ id: 'g1', label: 'Goblin 1', col: 6, row: 3 });

const fireballTargets = create(SpellTargetsSchema, {
  spellKey: 'spell:fireball',
  placement: AreaPlacement.POINT,
  areaShape: SpellAreaShape.SPHERE,
  areaSizeFt: 20,
  rangeFt: 150,
  targets: [
    { combatantId: 't', label: 'Toren', distanceFt: 15 },
    { combatantId: 'g1', label: 'Goblin 1', distanceFt: 20 },
  ],
});

const fireball = {
  spell: { level: 3 },
  save: { ability: Ability.DEXTERITY },
  damage: [],
  healBySlotLevel: {},
  higherLevel: [],
};

function preview(targets: { id: string; label: string; ally?: boolean }[]) {
  return create(PreviewSpellAreaResponseSchema, {
    origin: { col: 6, row: 4 },
    squares: [{ col: 6, row: 4 }],
    coverCounts: true,
    targets: targets.map((t) => ({
      combatantId: t.id,
      label: t.label,
      ally: t.ally ?? false,
      distanceFt: 5,
    })),
  });
}

async function open(
  over: {
    map?: boolean;
    targets?: typeof fireballTargets;
    answer?: ReturnType<typeof preview>;
    castSpell?: ReturnType<typeof vi.fn>;
  } = {},
) {
  TestBed.resetTestingModule();
  const previewSpellArea = vi.fn().mockResolvedValue(over.answer ?? preview([]));
  const castSpell =
    over.castSpell ??
    vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [pensantus, toren, goblin] }),
      cast: { targets: [], pendingDamages: [], metamagicKeys: [], slot: { level: 3, pact: false } },
      summoned: [],
    });
  const data = {
    campaignId: 'c',
    encounterId: 'enc',
    casterId: 'p',
    round: 2,
    spellKey: 'spell:fireball',
    name: 'Bola de Fogo',
    level: 3,
    concentration: false,
    cantripDice: '',
    economy: ActionEconomy.ACTION,
    slots: [{ level: 3, free: 2, pact: false }],
    usage: [{ level: 3, total: 2, used: 0 }],
    pact: null,
    targets: over.targets ?? fireballTargets,
    shieldFree: null,
    shieldName: '',
    attackBonus: 0,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state: {
      encounter: signal(encounter({ combatants: [pensantus, toren, goblin] })),
      apply: vi.fn(),
    },
    map:
      over.map === false
        ? null
        : {
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
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(fireball) } },
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
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    );
  const placeAt = async (col: number, row: number) => {
    const map = fixture.debugElement.query(By.directive(AreaMap)).componentInstance as AreaMap;
    map.place.emit({ kind: 'point', square: { col, row } });
    await settle();
  };
  return { fixture, el, button, placeAt, settle, castSpell, previewSpellArea };
}

describe('CastSheet: an area spell placed on the map', () => {
  it('opens on the map, "Onde ela explode", with "Confirmar local" off and its reason, asking nothing yet', async () => {
    const { el, button, previewSpellArea } = await open();
    expect(plain(el.textContent)).toContain('Onde ela explode');
    expect(el.querySelector('[role="application"]')?.getAttribute('aria-label')).toBe(
      'Mapa: escolha o ponto da Bola de Fogo',
    );
    expect(button('Confirmar local')?.getAttribute('aria-disabled')).toBe('true');
    expect(plain(el.textContent)).toContain('Toque no mapa para escolher o ponto.');
    expect(plain(el.querySelector('.frame__sub')?.textContent)).toContain('esfera de 6 m de raio');
    expect(el.querySelector('app-cast-targets')).toBeNull();
    expect(previewSpellArea).not.toHaveBeenCalled();
  });

  it('asks the server once placed, then lists who is inside with the ally warning, and casts with the area', async () => {
    const { el, button, placeAt, settle, castSpell, previewSpellArea } = await open({
      answer: preview([
        { id: 't', label: 'Toren', ally: true },
        { id: 'g1', label: 'Goblin 1' },
      ]),
    });
    await placeAt(6, 4);
    expect(previewSpellArea).toHaveBeenCalledWith(
      'c',
      'enc',
      'p',
      'spell:fireball',
      { level: 3, pact: false },
      { origin: { col: 6, row: 4 } },
    );
    button('Confirmar local')!.click();
    await settle();
    expect(plain(el.textContent)).toContain('Quem está na área');
    expect(plain(el.querySelector('app-area-list [role="alert"]')?.textContent)).toContain(
      'Toren está na área.',
    );
    expect(el.querySelectorAll('app-area-list li.row')).toHaveLength(2);
    expect(document.activeElement?.closest('app-area-step')).not.toBeNull();
    button('Conjurar Bola de Fogo')!.click();
    await settle();
    expect(castSpell).toHaveBeenCalledTimes(1);
    const call = castSpell.mock.calls[0] as unknown[];
    expect(call[5]).toEqual([]);
    expect(call[12]).toEqual({ area: { origin: { col: 6, row: 4 } } });
    expect(plain(el.querySelector('.frame__title')?.textContent)).toBe('Bola de Fogo conjurada');
    expect(button('Voltar à sua vez')).toBeDefined();
  });

  it('sends a retry of the same cast with the same key', async () => {
    const castSpell = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('lost', Code.Unavailable))
      .mockResolvedValue({
        encounter: encounter(),
        cast: { targets: [], pendingDamages: [], metamagicKeys: [] },
        summoned: [],
      });
    const { button, placeAt, settle } = await open({
      castSpell,
      answer: preview([{ id: 'g1', label: 'Goblin 1' }]),
    });
    await placeAt(6, 4);
    button('Confirmar local')!.click();
    await settle();
    button('Conjurar Bola de Fogo')!.click();
    await settle();
    button('Conjurar Bola de Fogo')!.click();
    await settle();
    expect(castSpell).toHaveBeenCalledTimes(2);
    expect(castSpell.mock.calls[1][7]).toBe(castSpell.mock.calls[0][7]);
  });

  it('asks in place when nobody the caster sees is inside, with the focus on "Mudar o local"', async () => {
    const { el, button, placeAt, settle, castSpell } = await open({ answer: preview([]) });
    await placeAt(6, 4);
    button('Confirmar local')!.click();
    await settle();
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Ninguém que você vê está na área. A magia gasta o espaço de 3º nível e a sua ação mesmo assim.',
    );
    expect(button('Conjurar Bola de Fogo')).toBeUndefined();
    expect(plain(document.activeElement?.textContent)).toContain('Mudar o local');
    expect(castSpell).not.toHaveBeenCalled();
    button('Conjurar mesmo assim')!.click();
    await settle();
    expect(castSpell).toHaveBeenCalledTimes(1);
  });

  it('"Mudar o local" goes back to the map with the point where it was, nothing spent', async () => {
    const { el, button, placeAt, settle, castSpell } = await open({
      answer: preview([{ id: 'g1', label: 'Goblin 1' }]),
    });
    await placeAt(6, 4);
    button('Confirmar local')!.click();
    await settle();
    button('Mudar o local')!.click();
    await settle();
    expect(plain(el.textContent)).toContain('Onde ela explode');
    expect(button('Confirmar local')?.getAttribute('aria-disabled')).not.toBe('true');
    expect(castSpell).not.toHaveBeenCalled();
  });
});

describe('CastSheet: an area spell without a map', () => {
  it('keeps the list, and asks in place when nobody is ticked ("Ninguém está marcado.")', async () => {
    const { el, button, settle, castSpell } = await open({ map: false });
    expect(el.querySelector('[role="application"]')).toBeNull();
    expect(plain(el.textContent)).toContain('Quem a magia atinge (pode ser ninguém)');
    button('Conjurar Bola de Fogo')!.click();
    await settle();
    expect(castSpell).not.toHaveBeenCalled();
    expect(plain(el.querySelector('[role="alert"] p')?.textContent)).toBe(
      'Ninguém está marcado. A magia gasta o espaço de 3º nível e a sua ação mesmo assim.',
    );
    expect(plain(document.activeElement?.textContent)).toBe('Voltar à lista');
    button('Voltar à lista')!.click();
    await settle();
    expect(el.querySelector('[role="alert"]')).toBeNull();
    button('Conjurar Bola de Fogo')!.click();
    await settle();
    button('Conjurar mesmo assim')!.click();
    await settle();
    expect(castSpell).toHaveBeenCalledTimes(1);
    expect((castSpell.mock.calls[0] as unknown[])[5]).toEqual([]);
  });
});
