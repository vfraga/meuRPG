import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CombatantState,
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterSchema,
  ReadiedKind,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  AttackKind,
  DisabledReasonCode,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter } from '../../../../core/combat/combat-testing';
import { ReadySheet, type ReadySheetData } from './ready-sheet';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').replace(/ /g, ' ').trim() ?? '';

const std = (key: string, namePt: string) => ({
  action: { key, namePt, economy: ActionEconomy.ACTION },
  enabled: true,
});

const sword = {
  attack: {
    key: 'equipment:longsword',
    name: 'Longsword',
    namePt: 'Espada longa',
    attackBonus: 6,
    damage: '1d8+3',
    damageTypePt: 'cortante',
    kind: AttackKind.WEAPON,
    rangeFt: 5,
  },
  enabled: true,
};
const bow = {
  attack: {
    key: 'equipment:longbow',
    name: 'Longbow',
    namePt: 'Arco longo',
    attackBonus: 5,
    damage: '1d8+2',
    damageTypePt: 'perfurante',
    kind: AttackKind.WEAPON,
    rangeFt: 150,
    longRangeFt: 600,
  },
  enabled: true,
};
const missiles = {
  spell: { key: 'spell:magic-missile', name: 'Magic Missile', namePt: 'Mísseis Mágicos', level: 1 },
  economy: ActionEconomy.ACTION,
  enabled: true,
  slots: [
    { level: 1, free: 2, total: 4 },
    { level: 2, free: 1, total: 2 },
  ],
};
const charm = {
  spell: { key: 'spell:charm-person', name: 'Charm Person', namePt: 'Enfeitiçar Pessoa', level: 1 },
  economy: ActionEconomy.ACTION,
  enabled: true,
  slots: [{ level: 1, free: 2, total: 4 }],
};
const shield = {
  spell: { key: 'spell:shield', name: 'Shield', namePt: 'Escudo Arcano', level: 1 },
  economy: ActionEconomy.REACTION,
  enabled: false,
  slots: [{ level: 1, free: 2, total: 4 }],
};

const options = (over: Record<string, unknown[]> = {}) =>
  create(TurnOptionsSchema, {
    attacks: [sword, bow],
    spells: [],
    standardActions: [
      std('standard:attack', 'Atacar'),
      std('standard:dodge', 'Esquivar'),
      std('standard:search', 'Procurar'),
      std('standard:ready', 'Preparar'),
    ],
    ...over,
  } as never);

const combatants = [
  { id: 'tor', label: 'Tor', state: CombatantState.UNHURT },
  { id: 'z1', label: 'Zumbi 1', state: CombatantState.UNHURT },
  { id: 'z2', label: 'Zumbi 2', state: CombatantState.DEFEATED },
] as never;

describe('ReadySheet (#582)', () => {
  const readyAction = vi.fn();
  const close = vi.fn();

  function render(over: Partial<ReadySheetData> = {}) {
    const state = new CombatState();
    state.apply(encounter());
    const data: ReadySheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      combatantId: 'tor',
      ownerLabel: 'Tor',
      options: options(),
      summaries: new Map(),
      spellTargets: [],
      theatre: false,
      noSpeed: false,
      speedFt: 25,
      concentrating: '',
      combatants,
      state,
      ...over,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: { ready: { readyAction } } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(ReadySheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) =>
        flat(b)
          .replace(/^(check|alarm|swords|auto_fix_high|directions_walk|more_horiz) ?/, '')
          .startsWith(text),
      )!;
    const click = (b: HTMLElement) => {
      b.click();
      fixture.detectChanges();
    };
    const type = (text: string) => {
      const area = el.querySelector<HTMLTextAreaElement>('#ready-trigger')!;
      area.value = text;
      area.dispatchEvent(new Event('input'));
      fixture.detectChanges();
    };
    const choose = (label: string) => {
      const input = Array.from(el.querySelectorAll<HTMLInputElement>('input[type=radio]')).find(
        (i) => flat(i.closest('label')).includes(label),
      )!;
      input.checked = true;
      input.dispatchEvent(new Event('change'));
      fixture.detectChanges();
    };
    return { fixture, el, settle, button, click, type, choose, state };
  }

  beforeEach(() => {
    readyAction.mockReset();
    close.mockReset();
  });

  it('is titled "Preparar uma ação", says whose it is, and lists the kinds of a fighter', () => {
    const { el } = render();
    expect(flat(el.querySelector('h2'))).toBe('Preparar uma ação');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Tor · fica guardada até o gatilho');
    const kinds = Array.from(el.querySelectorAll('.kind')).map((b) => flat(b).replace(/^\w+ /, ''));
    expect(kinds).toEqual(['Ataque', 'Mover', 'Outra']);
  });

  it('starts on Ataque with the first attack chosen, and lists the attacks with their numbers', () => {
    const { el } = render();
    expect(flat(el.querySelector('.pick__legend'))).toBe('Qual ataque');
    const cards = Array.from(el.querySelectorAll('label.opt')).map((l) => flat(l));
    expect(cards[0]).toContain('Espada longa');
    expect(cards[0]).toContain('+6 para acertar');
    expect(cards[1]).toContain('Arco longo');
    expect(el.querySelector<HTMLInputElement>('label.opt input')!.checked).toBe(true);
  });

  it('counts the trigger and keeps "Preparar" off, with its reason, until one is written', () => {
    const { el, button, click, type } = render();
    expect(flat(el.querySelector('.field__count'))).toBe('0/200');
    expect(button('Preparar').getAttribute('aria-disabled')).toBe('true');
    expect(el.querySelector('#ready-missing')).toBeNull();
    click(button('Preparar'));
    expect(flat(el.querySelector('#ready-missing'))).toContain('Escreva quando a ação dispara.');
    expect(flat(el.querySelector('#ready-trigger-note'))).toBe('Escreva quando a ação dispara.');
    type('Quando o zumbi passar pela porta');
    expect(flat(el.querySelector('.field__count'))).toBe('32/200');
    expect(flat(el.querySelector('#ready-trigger-note'))).toBe('O mestre decide quando acontece.');
    expect(button('Preparar').getAttribute('aria-disabled')).toBeNull();
  });

  it('refuses a trigger over 200 characters in the field, before sending anything', () => {
    const { el, button, type } = render();
    type('a'.repeat(231));
    expect(flat(el.querySelector('.field__count'))).toBe('231/200');
    expect(flat(el.querySelector('#ready-trigger-note'))).toBe('Até 200 caracteres.');
    expect(el.querySelector('#ready-trigger')!.getAttribute('aria-invalid')).toBe('true');
    expect(button('Preparar').getAttribute('aria-disabled')).toBe('true');
  });

  it('readies the attack with its key, the creature and the trigger, applies the combat and closes', async () => {
    readyAction.mockResolvedValue(create(EncounterSchema, { id: 'enc', revision: 9 }));
    const { el, button, click, type, settle, state } = render();
    const select = el.querySelector<HTMLSelectElement>('#ready-target')!;
    expect(Array.from(select.options).map((o) => o.textContent?.trim())).toEqual([
      'Ninguém em especial',
      'Zumbi 1',
    ]);
    select.value = 'z1';
    select.dispatchEvent(new Event('change'));
    type('Quando o zumbi passar pela porta');
    click(button('Preparar'));
    await settle();
    expect(readyAction).toHaveBeenCalledWith(
      {
        campaignId: 'camp',
        encounterId: 'enc',
        combatantId: 'tor',
        kind: ReadiedKind.ATTACK,
        actionKey: 'equipment:longsword',
        spellKey: '',
        slotLevel: 0,
        targetCombatantId: 'z1',
        triggerText: 'Quando o zumbi passar pela porta',
      },
      expect.any(String),
    );
    expect(state.encounter()?.revision).toBe(9);
    expect(close).toHaveBeenCalledWith(true);
  });

  it('turns Mover off with its reason, on a map or not, and "Preparar" cannot send it (#856)', () => {
    const why = 'Mover pela janela ainda não dá: por enquanto o mestre move a ficha.';
    for (const theatre of [true, false]) {
      const r = render({ theatre });
      const move = Array.from(r.el.querySelectorAll('.kind')).find((b) =>
        flat(b).includes('Mover'),
      )!;
      expect(move.classList).toContain('kind--off');
      r.click(move as HTMLElement);
      expect(flat(r.el.querySelector('#ready-kind-why'))).toContain(why);
      r.type('Quando Tor recuar, corro até a porta');
      expect(r.button('Preparar').getAttribute('aria-disabled')).toBe('true');
      expect(r.button('Preparar').getAttribute('aria-describedby')).toBe('ready-kind-why');
      expect(r.el.querySelector('#ready-missing')).toBeNull(); // the reason is written once
      expect(r.el.querySelector('#ready-move')).toBeNull(); // and the move's own text is not under it
      r.click(r.button('Preparar'));
      expect(readyAction).not.toHaveBeenCalled();
    }
  });

  it('writes the reason in the card of a spell the window cannot release, and lets the others be held', () => {
    const { el, button, click } = render({
      options: options({ spells: [missiles, charm] }),
      spellTargets: [
        create(SpellTargetsSchema, { spellKey: 'spell:magic-missile', maxTargets: 3 }),
        create(SpellTargetsSchema, { spellKey: 'spell:charm-person', maxTargets: 1 }),
      ],
    });
    click(button('Magia'));
    const cards = Array.from(el.querySelectorAll('label.opt'));
    expect(cards[0].classList).toContain('opt--off');
    expect(flat(cards[0])).toContain('Soltar pela janela ainda não dá para esta magia');
    expect(cards[1].classList).not.toContain('opt--off');
    expect(flat(cards[1])).toContain('Enfeitiçar Pessoa');
  });

  it('lists "Outra" as Esquivar and Procurar and sends the one chosen', async () => {
    readyAction.mockResolvedValue(create(EncounterSchema, { id: 'enc' }));
    const { el, button, click, type, choose, settle } = render();
    click(button('Outra'));
    expect(flat(el.querySelector('.pick__legend'))).toBe('Qual ação');
    expect(Array.from(el.querySelectorAll('label.opt')).map((l) => flat(l))).toEqual([
      'Esquivar',
      'Procurar',
    ]);
    choose('Procurar');
    type('Se a porta abrir');
    click(button('Preparar'));
    await settle();
    expect(readyAction.mock.calls[0][0]).toMatchObject({
      kind: ReadiedKind.OTHER,
      actionKey: 'standard:search',
    });
  });

  it('lists the spells of 1 action with the slot, writes the reason in the others, and names the spell on the button', async () => {
    readyAction.mockResolvedValue(create(EncounterSchema, { id: 'enc' }));
    const { el, button, click, type, settle } = render({
      options: options({ spells: [missiles, shield] }),
      concentrating: 'Névoa Obscurecente',
    });
    expect(
      Array.from(el.querySelectorAll('.kind')).map((b) => flat(b).replace(/^\w+ /, '')),
    ).toEqual(['Ataque', 'Magia', 'Mover', 'Outra']);
    click(button('Magia'));
    expect(flat(el.querySelector('.pick__legend'))).toBe('Qual magia · só as de 1 ação');
    const cards = Array.from(el.querySelectorAll('label.opt'));
    expect(flat(cards[1])).toContain('É uma reação: só uma magia de 1 ação pode ser preparada.');
    expect(cards[1].classList).toContain('opt--off');
    expect(
      Array.from(el.querySelectorAll('.kinds')).some(
        (f) => flat(f.querySelector('legend')) === 'Com que espaço',
      ),
    ).toBe(true);
    expect(flat(el.querySelector('.warn'))).toContain(
      'A concentração em Névoa Obscurecente acaba agora.',
    );
    expect(flat(el.querySelector('.legend'))).toContain(
      'Ação e espaço de 1º círculo: gastos agora, ao preparar.',
    );
    click(button('2º círculo'));
    expect(flat(el.querySelector('.legend'))).toContain('Ação e espaço de 2º círculo');
    type('Se o wight sair da sombra do pilar');
    expect(flat(button('Preparar Mísseis Mágicos'))).toContain('Preparar Mísseis Mágicos');
    click(button('Preparar Mísseis Mágicos'));
    await settle();
    expect(readyAction.mock.calls[0][0]).toMatchObject({
      kind: ReadiedKind.SPELL,
      actionKey: '',
      spellKey: 'spell:magic-missile',
      slotLevel: 2,
    });
  });

  it('writes "Sem espaços de 2º círculo." in the card of a spell with no slot', () => {
    const scorching = {
      spell: {
        key: 'spell:scorching-ray',
        name: 'Scorching Ray',
        namePt: 'Raio Ardente',
        level: 2,
      },
      economy: ActionEconomy.ACTION,
      enabled: false,
      reason: { code: DisabledReasonCode.NO_SLOT },
      slots: [],
    };
    const { el, click, button } = render({ options: options({ spells: [scorching] }) });
    click(button('Magia'));
    expect(flat(el.querySelector('label.opt'))).toContain('Sem espaços de 2º círculo.');
  });

  it("writes the server's refusal in a red band and keeps what was typed and chosen; nothing closes", async () => {
    const refused = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: EncounterBlockedSchema,
        value: create(EncounterBlockedSchema, { reason: EncounterBlockedReason.READY_ACTION }),
      },
    ]);
    readyAction.mockRejectedValue(refused);
    const { el, button, click, type, settle } = render();
    type('Se a porta abrir');
    click(button('Preparar'));
    await settle();
    expect(flat(el.querySelector('.mr-notice--danger'))).toContain(
      'Essa ação não pode ser preparada agora. Escolha outra. Nada foi gasto.',
    );
    expect(el.querySelector<HTMLTextAreaElement>('#ready-trigger')!.value).toBe('Se a porta abrir');
    expect(close).not.toHaveBeenCalled();
  });

  it('says nothing was spent when the call did not get through, and lets the player try again', async () => {
    readyAction.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    const { el, button, click, type, settle } = render();
    type('Se a porta abrir');
    click(button('Preparar'));
    await settle();
    expect(flat(el.querySelector('.mr-notice--danger'))).toContain(
      'Não deu para preparar agora. Nada foi gasto. Tente de novo.',
    );
    expect(button('Preparar').getAttribute('aria-disabled')).toBeNull();
  });

  it('stops both buttons while it is calling, and does not close under the call', async () => {
    let finish: (v: unknown) => void = () => undefined;
    readyAction.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    const { fixture, button, click, type, settle } = render();
    type('Se a porta abrir');
    click(button('Preparar'));
    expect(flat(button('Preparando…'))).toContain('Preparando…');
    expect(button('Cancelar').disabled).toBe(true);
    click(button('Cancelar'));
    expect(close).not.toHaveBeenCalled();
    finish(create(EncounterSchema, { id: 'enc' }));
    await settle();
    fixture.detectChanges();
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('closes with Cancelar and sends nothing', () => {
    const { button, click } = render();
    click(button('Cancelar'));
    expect(close).toHaveBeenCalledTimes(1);
    expect(readyAction).not.toHaveBeenCalled();
  });
});
