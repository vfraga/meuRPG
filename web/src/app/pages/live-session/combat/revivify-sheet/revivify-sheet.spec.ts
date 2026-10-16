import { signal } from '@angular/core';
import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  SpellSlotSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  PreviewRevivifyResponseSchema,
  RevivifyRequestSchema,
  RevivifyRequestStatus,
  RevivifyTargetSchema,
} from '../../../../../gen/meurpg/play/v1/revivify_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { RevivifyClient } from '../../../../core/revivify/revivify-client';
import { RevivifySheet, type RevivifySheetData } from './revivify-sheet';
import { nextTurn } from '../../../../../test-settle';

const text = (el: Element | null | undefined) =>
  (el?.textContent ?? '').replace(/\s+/g, ' ').trim();

const combatPreview = create(PreviewRevivifyResponseSchema, {
  targets: [
    create(RevivifyTargetSchema, {
      targetId: 'toren',
      name: 'Toren',
      deathRound: 3,
      roundsSinceDeath: 5,
    }),
    create(RevivifyTargetSchema, {
      targetId: 'goblin',
      name: 'Goblin 2',
      deathRound: 6,
      roundsSinceDeath: 2,
    }),
  ],
  slot: create(SpellSlotSchema, { level: 3 }),
  slotsFree: 2,
  countsTime: true,
});
const outsidePreview = create(PreviewRevivifyResponseSchema, {
  targets: [
    create(RevivifyTargetSchema, {
      targetId: 'toren',
      name: 'Toren',
      needsMasterConfirmation: true,
    }),
  ],
  slot: create(SpellSlotSchema, { level: 3 }),
  slotsFree: 2,
});

interface Harness {
  readonly fixture: ComponentFixture<RevivifySheet>;
  readonly el: HTMLElement;
  readonly reload: ReturnType<typeof signal<number>>;
  readonly closed: ReturnType<typeof vi.fn>;
  readonly cast: ReturnType<typeof vi.fn>;
  readonly endTurn: ReturnType<typeof vi.fn>;
  readonly preview: ReturnType<typeof vi.fn>;
  readonly request: ReturnType<typeof vi.fn>;
  readonly list: ReturnType<typeof vi.fn>;
}

async function settle(h: Pick<Harness, 'fixture'>): Promise<void> {
  await h.fixture.whenStable();
  await nextTurn();
  await h.fixture.whenStable();
  h.fixture.detectChanges();
}

async function mount(
  inCombat: boolean,
  over: {
    cast?: () => Promise<unknown>;
    list?: () => Promise<unknown>;
  } = {},
): Promise<Harness> {
  const reload = signal(0);
  const state = new CombatState();
  state.apply(
    encounter({
      id: 'enc',
      currentCombatantId: 'i',
      combatants: [combatant({ id: 'i', label: 'Ilaria', mine: true })],
    }),
  );
  const data: RevivifySheetData = {
    campaignId: 'c',
    casterName: 'Ilaria',
    classes: 'Clérigo 5',
    combat: inCombat ? { encounterId: 'enc', casterId: 'i', round: 8, state } : null,
    casterCharacterId: 'ilaria',
    reload,
  };
  const closed = vi.fn();
  const cast = vi.fn(
    over.cast ??
      (() =>
        Promise.resolve({
          encounter: encounter({
            id: 'enc',
            currentCombatantId: 'i',
            combatants: [combatant({ id: 'i', label: 'Ilaria', mine: true })],
          }),
          cast: {},
          summoned: [],
        })),
  );
  const endTurn = vi.fn(() =>
    Promise.resolve(
      encounter({
        id: 'enc',
        currentCombatantId: 'b',
        combatants: [combatant({ id: 'b', label: 'Brisa' })],
      }),
    ),
  );
  const preview = vi.fn(() => Promise.resolve(inCombat ? combatPreview : outsidePreview));
  const request = vi.fn(() =>
    Promise.resolve(
      create(RevivifyRequestSchema, { id: 'r1', status: RevivifyRequestStatus.PENDING }),
    ),
  );
  const list = vi.fn(over.list ?? (() => Promise.resolve([])));
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: closed, disableClose: false } },
      { provide: CombatClient, useValue: { castRevivify: cast, endTurn } },
      { provide: RevivifyClient, useValue: { preview, request, list } },
    ],
  });
  const fixture = TestBed.createComponent(RevivifySheet);
  const h = {
    fixture,
    el: fixture.nativeElement as HTMLElement,
    reload,
    closed,
    cast,
    endTurn,
    preview,
    request,
    list,
  };
  fixture.detectChanges();
  await settle(h);
  return h;
}

const button = (h: Harness, label: string) =>
  Array.from(h.el.querySelectorAll<HTMLButtonElement>('button')).find((b) => text(b) === label);
const click = async (h: Harness, label: string) => {
  button(h, label)?.click();
  await settle(h);
};
const tick = async (h: Harness) => {
  const box = h.el.querySelector<HTMLInputElement>('input[type=checkbox]');
  box?.click();
  await settle(h);
};

describe('RevivifySheet in a combat', () => {
  it('lists only who the server says the spell reaches, with the header and the steps', async () => {
    const h = await mount(true);
    expect(h.preview).toHaveBeenCalledWith('c', { encounterId: 'enc', casterId: 'i' });
    expect(text(h.el.querySelector('.frame__title'))).toBe('Revivificar');
    expect(text(h.el.querySelector('.frame__sub'))).toBe(
      '3º nível · 1 ação · toque · Ilaria, Clérigo 5',
    );
    expect(text(h.el.querySelector('.steps'))).toBe('1 Alvo 2 Confirmar 3 Resultado');
    const rows = Array.from(h.el.querySelectorAll('.row'));
    expect(rows).toHaveLength(2);
    expect(text(rows[0].querySelector('.row__title'))).toBe('Toren');
    expect(text(rows[0].querySelector('.row__detail'))).toBe(
      'Ao lado · morreu na rodada 3, há 5 rodadas',
    );
    expect(text(rows[0].querySelector('.tag'))).toContain('Pode ser revivido');
    expect(text(rows[1].querySelector('.row__title'))).toBe('Goblin 2');
    expect(text(rows[1].querySelector('.row__detail'))).toBe(
      'Ao lado · morreu na rodada 6, há 2 rodadas',
    );
    expect(text(h.el)).toContain('Só aparece quem a magia pode alcançar agora.');
    expect(text(h.el)).not.toContain('Encerrar turno');
  });

  it('asks for the diamonds before it casts, and casts once with the dead target and the slot', async () => {
    const h = await mount(true);
    await click(h, 'Próximo');
    expect(text(h.el.querySelector('.frame__title'))).toBe('Revivificar em Toren');
    expect(text(h.el.querySelector('.frame__sub'))).toBe('Toren volta à vida com 1 PV.');
    expect(text(h.el)).toContain('Gasta um espaço de 3º nível (você tem 2) e a sua ação.');
    const go = button(h, 'Conjurar Revivificar')!;
    expect(go.getAttribute('aria-disabled')).toBe('true');
    expect(go.hasAttribute('disabled')).toBe(false);
    expect(text(h.el)).toContain('Marque que você tem os diamantes');
    await click(h, 'Conjurar Revivificar');
    expect(h.cast).not.toHaveBeenCalled();

    await tick(h);
    expect(button(h, 'Conjurar Revivificar')!.getAttribute('aria-disabled')).not.toBe('true');
    expect(text(h.el)).not.toContain('Marque que você tem os diamantes');
    await click(h, 'Conjurar Revivificar');
    expect(h.cast).toHaveBeenCalledTimes(1);
    expect(h.cast.mock.calls[0].slice(0, 5)).toEqual([
      'c',
      'enc',
      'i',
      { level: 3, pact: false },
      'toren',
    ]);
    const result = text(h.el.querySelector('[role=status].live'));
    expect(result).toContain(
      'Toren voltou à vida. Está com 1 PV, acordado e sem testes contra a morte.',
    );
    expect(result).toContain('Você gastou um espaço de 3º nível (restam 1 de 2) e a sua ação.');
    expect(result).toContain('Diamantes de 300 PO gastos, como você confirmou.');
    expect(button(h, 'Encerrar turno')).toBeDefined();
  });

  it('"Voltar" goes back a step and keeps the target', async () => {
    const h = await mount(true);
    await click(h, 'Próximo');
    await click(h, 'Voltar');
    expect(h.el.querySelectorAll('.row')).toHaveLength(2);
    expect(h.el.querySelector<HTMLInputElement>('input[type=radio]')?.checked).toBe(true);
  });

  it('"Encerrar turno" ends the turn and closes the sheet', async () => {
    const h = await mount(true);
    await click(h, 'Próximo');
    await tick(h);
    await click(h, 'Conjurar Revivificar');
    await click(h, 'Encerrar turno');
    expect(h.endTurn).toHaveBeenCalledWith('c', 'enc', 'i', false, 8);
    expect(h.closed).toHaveBeenCalledWith(true);
  });

  it('says one generic sentence when the target is refused, and reads the list again', async () => {
    const h = await mount(true, {
      cast: () =>
        Promise.reject(
          new ConnectError('that creature cannot be revived now', Code.FailedPrecondition),
        ),
    });
    await click(h, 'Próximo');
    await tick(h);
    await click(h, 'Conjurar Revivificar');
    expect(text(h.el.querySelector('[role=alert]'))).toContain(
      'Não dá para reviver esta criatura agora.',
    );
    expect(h.preview).toHaveBeenCalledTimes(2);
    expect(h.el.querySelectorAll('.row')).toHaveLength(2);
    expect(text(h.el)).not.toMatch(/longe|escondid|bloquead|1 minuto/i);
  });

  it('keeps the combat refusals that have their own words', async () => {
    const blocked = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: EncounterBlockedSchema,
        value: create(EncounterBlockedSchema, { reason: EncounterBlockedReason.ACTION_USED }),
      },
    ]);
    const h = await mount(true, { cast: () => Promise.reject(blocked) });
    await click(h, 'Próximo');
    await tick(h);
    await click(h, 'Conjurar Revivificar');
    expect(text(h.el.querySelector('[role=alert]'))).toContain(
      'Sua ação já foi usada neste turno.',
    );
  });
});

describe('RevivifySheet outside a combat', () => {
  it('shows the master confirms the time and asks for the character', async () => {
    const h = await mount(false);
    expect(h.preview).toHaveBeenCalledWith('c', { casterCharacterId: 'ilaria' });
    expect(text(h.el)).toContain('O mestre confirma que faz menos de 1 minuto.');
    expect(text(h.el)).toContain(
      'Fora de combate o app não conta o tempo: ao conjurar, o mestre responde.',
    );
    expect(text(h.el)).toContain('Ao lado · morreu fora de combate');
    expect(text(h.el)).not.toContain('Pode ser revivido');
  });

  async function waiting(over: Parameters<typeof mount>[1] = {}): Promise<Harness> {
    const h = await mount(false, over);
    await click(h, 'Próximo');
    expect(text(h.el)).toContain('Gasta um espaço de 3º nível (você tem 2).');
    expect(text(h.el)).not.toContain('a sua ação');
    await tick(h);
    await click(h, 'Conjurar Revivificar');
    return h;
  }

  it('waits for the master with nothing spent', async () => {
    const h = await waiting();
    expect(h.request).toHaveBeenCalledWith(
      'c',
      'ilaria',
      'toren',
      { level: 3, pact: false },
      expect.any(String),
    );
    expect(h.cast).not.toHaveBeenCalled();
    const live = h.el.querySelector('[role=status].live')!;
    expect(text(live)).toContain('Esperando o mestre');
    expect(text(h.el)).not.toContain('voltou à vida');
    expect(button(h, 'Encerrar turno')).toBeUndefined();
  });

  it('shows the result when the master says yes, with no action and no end of turn', async () => {
    let answer: unknown[] = [];
    const h = await waiting({ list: () => Promise.resolve(answer) });
    answer = [
      create(RevivifyRequestSchema, {
        id: 'r1',
        status: RevivifyRequestStatus.CONFIRMED,
        targetName: 'Toren',
        slot: create(SpellSlotSchema, { level: 3 }),
        slotsLeft: 1,
      }),
    ];
    h.reload.set(1);
    await settle(h);
    const result = text(h.el.querySelector('[role=status].live'));
    expect(result).toContain('Toren voltou à vida.');
    expect(result).toContain('Você gastou um espaço de 3º nível (restam 1 de 2).');
    expect(result).toContain('Diamantes de 300 PO gastos, como você confirmou.');
    expect(result).not.toContain('a sua ação');
    expect(button(h, 'Encerrar turno')).toBeUndefined();
    expect(button(h, 'Voltar à sessão')).toBeDefined();
  });

  it('shows only "O mestre disse que não dá." when he says no', async () => {
    let answer: unknown[] = [];
    const h = await waiting({ list: () => Promise.resolve(answer) });
    answer = [create(RevivifyRequestSchema, { id: 'r1', status: RevivifyRequestStatus.DENIED })];
    h.reload.set(1);
    await settle(h);
    const said = text(h.el.querySelector('[role=status].live'));
    expect(said).toContain('O mestre disse que não dá.');
    expect(said).not.toMatch(/gastou|espaço|diamantes|voltou/i);
    expect(text(h.el)).not.toContain('Esperando o mestre');
  });
});
