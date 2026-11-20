import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { By } from '@angular/platform-browser';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  CombatantKind,
  PreviewSpellAreaResponseSchema,
  ReactionKind,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  Ability,
  ActionEconomy,
  SpellAreaShape,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
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
    targets?: typeof fireballTargets;
    answer?: ReturnType<typeof preview>;
    castSpell?: ReturnType<typeof vi.fn>;
    markReactionRolling?: ReturnType<typeof vi.fn>;
    master?: boolean;
    windowGone?: boolean;
    close?: ReturnType<typeof vi.fn>;
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
  const open = encounter({
    combatants: [pensantus, toren, goblin],
    reactionWindows: over.windowGone
      ? []
      : [
          reactionWindow({
            id: 'w1',
            kind: ReactionKind.READIED,
            reactorId: 'p',
            rolling: false,
          } as never),
        ],
  } as never);
  const enc = signal(open);
  const state = { encounter: enc, apply: vi.fn((e) => enc.set(e)) };
  const markReactionRolling = over.markReactionRolling ?? vi.fn().mockResolvedValue(open);
  const close = over.close ?? vi.fn();
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
    slots: [],
    usage: [],
    pact: null,
    targets: over.targets ?? fireballTargets,
    shieldFree: null,
    shieldName: '',
    attackBonus: 0,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    master: over.master ?? false,
    state,
    map: {
      image: { url: 'cave.png', width: 1200, height: 800 },
      mapName: 'A caverna',
      columns: 12,
      rows: 8,
      layers: null,
      fog: null,
    },
    release: {
      windowId: 'w1',
      readiedId: 'r1',
      reactorLabel: 'Pensantus',
      trigger: 'Quando o ogro passar pela porta',
      slotLevel: 3,
    },
  } as unknown as CastSheetData;
  TestBed.configureTestingModule({
    providers: [
      {
        provide: CombatClient,
        useValue: fakeCombatClient({ castSpell, previewSpellArea, markReactionRolling }),
      },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(fireball) } },
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close } },
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
  return {
    fixture,
    el,
    button,
    placeAt,
    settle,
    castSpell,
    previewSpellArea,
    markReactionRolling,
    close,
    state,
    enc,
  };
}

describe('CastSheet: a readied spell released on the map (#857, boards 857-02 and 857-03)', () => {
  it('opens with the release title, the alarm icon, the band and no slot to pick, marking the window at once', async () => {
    const { el, markReactionRolling } = await open();
    expect(plain(el.querySelector('.frame__title')?.textContent)).toBe('Soltar Bola de Fogo');
    expect(plain(el.querySelector('.frame__sub')?.textContent)).toContain(
      'Reação · a ação preparada de Pensantus',
    );
    expect(plain(el.querySelector('[data-testid=release-band]')?.textContent)).toContain(
      'Preparada: “Quando o ogro passar pela porta”Espaço de 3º nível · gasto ao preparar',
    );
    expect(el.querySelector('app-slot-picker')).toBeNull();
    expect(markReactionRolling).toHaveBeenCalledWith('c', 'enc', 'w1', false);
  });

  it('foots step 1 with "Confirmar local" and "Voltar", and says the table waits', async () => {
    const { el, button } = await open();
    expect(plain(el.querySelector('[data-testid=release-wait]')?.textContent)).toBe(
      'A mesa espera você: até 2 min.',
    );
    expect(button('Confirmar local')).toBeDefined();
    expect(button('Voltar')).toBeDefined();
    expect(button('Cancelar')).toBeUndefined();
  });

  it('casts with no slot once the place is confirmed, with "Conjurar Bola de Fogo" and "Mudar o local", and ends on "Voltar à sessão"', async () => {
    const { el, button, placeAt, settle, castSpell } = await open({
      answer: preview([{ id: 'g1', label: 'Goblin 1' }]),
    });
    await placeAt(6, 4);
    button('Confirmar local')!.click();
    await settle();
    expect(button('Mudar o local')).toBeDefined();
    button('Conjurar Bola de Fogo')!.click();
    await settle();
    expect((castSpell.mock.calls[0] as unknown[])[4]).toBeNull();
    expect(button('Voltar à sessão')).toBeDefined();
    expect(button('Voltar à sua vez')).toBeUndefined();
    expect(plain(el.querySelector('.frame__title')?.textContent)).toBe('Bola de Fogo conjurada');
  });

  it('on "Voltar" takes the mark off with the same call and clear, then closes without a cast', async () => {
    const { button, settle, markReactionRolling, close } = await open();
    button('Voltar')!.click();
    await settle();
    expect(markReactionRolling).toHaveBeenLastCalledWith('c', 'enc', 'w1', true);
    expect(close).toHaveBeenCalledWith(false);
  });

  it('takes the mark off when the sheet goes away without "Voltar" (Esc, the backdrop, a swipe)', async () => {
    const { fixture, markReactionRolling } = await open();
    expect(markReactionRolling).toHaveBeenCalledTimes(1);
    fixture.destroy();
    await Promise.resolve();
    expect(markReactionRolling).toHaveBeenLastCalledWith('c', 'enc', 'w1', true);
  });

  it('does not clear a mark when the sheet goes away after "Voltar" took it off, or once the window is gone', async () => {
    const back = await open();
    back.button('Voltar')!.click();
    await back.settle();
    back.fixture.destroy();
    await Promise.resolve();
    expect(back.markReactionRolling).toHaveBeenCalledTimes(2);
    const gone = await open();
    gone.enc.set(encounter({ revision: 9, combatants: [], reactionWindows: [] } as never));
    await gone.settle();
    gone.fixture.destroy();
    await Promise.resolve();
    expect(gone.markReactionRolling).toHaveBeenCalledTimes(1);
  });

  it('says in the confirmation of nobody that only the reaction is spent', async () => {
    const { el, button, placeAt, settle } = await open({ answer: preview([]) });
    await placeAt(6, 4);
    button('Confirmar local')!.click();
    await settle();
    expect(plain(el.querySelector('app-area-notice')?.textContent)).toContain(
      'A magia gasta a sua reação mesmo assim.',
    );
  });

  it('keeps the master off the mark, and reads the footer that the player may still answer', async () => {
    const { el, markReactionRolling } = await open({ master: true });
    expect(markReactionRolling).not.toHaveBeenCalled();
    expect(plain(el.querySelector('[data-testid=release-wait]')?.textContent)).toBe(
      'Pensantus ainda pode responder no próprio aparelho: vale a primeira resposta.',
    );
  });

  it('drops the map when the window goes away: the notice and "Voltar à sessão" only (857-09 card 1)', async () => {
    const { el, button, settle, state } = await open();
    state.apply(
      encounter({
        revision: 9,
        combatants: [pensantus, toren, goblin],
        reactionWindows: [],
      } as never),
    );
    await settle();
    expect(el.querySelector('[role="application"]')).toBeNull();
    expect(plain(el.querySelector('[data-testid=release-gone]')?.textContent)).toContain(
      'A janela fechou.',
    );
    expect(button('Confirmar local')).toBeUndefined();
    expect(button('Voltar à sessão')).toBeDefined();
  });
});
