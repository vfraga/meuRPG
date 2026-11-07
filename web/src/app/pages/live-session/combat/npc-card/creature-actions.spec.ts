import { TestBed } from '@angular/core/testing';
import { create, type MessageInitShape } from '@bufbuild/protobuf';
import { describe, expect, it, vi } from 'vitest';

import {
  AreaPlacement,
  CombatantSide,
  CreatureActionResultSchema,
  LimitedActionSchema,
  PendingDamageSchema,
  SaveOutcome,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  Ability,
  type Creature,
  CreatureActionClass,
  CreatureActionKind,
  CreatureAreaShape,
  CreatureSchema,
  SpellSaveSuccess,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import type { CombatState } from '../../../../core/combat/combat-state';
import { combatant, creatureAction, encounter } from '../../../../core/combat/combat-testing';
import { settle } from '../../../../../test-settle';
import { CreatureActions, type CreatureAreaRequest } from './creature-actions';

const breath = () =>
  creatureAction({
    key: 'monster:hell-hound#fire-breath',
    name: 'Fire Breath',
    namePt: 'Sopro de fogo',
    text: 'The hound exhales fire in a 15-foot cone.',
    area: { shape: CreatureAreaShape.CONE, sizeFt: 15, widthFt: 0 },
    save: {
      ability: Ability.DEXTERITY,
      dc: 13,
      onSuccess: SpellSaveSuccess.HALF,
      damage: [{ dice: '6d6', damageTypeKey: 'damage:fire', damageTypePt: 'fogo' }],
    },
  } as never);
const gaze = () =>
  creatureAction({
    key: 'monster:ghost#horrifying-visage',
    name: 'Horrifying Visage',
    namePt: 'Semblante horripilante',
    text: 'Each non-undead creature within 60 feet that can see the ghost must succeed on a DC 13 Wisdom saving throw or be frightened for 1 minute.',
    planClass: CreatureActionClass.PARTIAL,
    save: {
      ability: Ability.WISDOM,
      dc: 13,
      onSuccess: SpellSaveSuccess.NONE,
      damage: [],
      conditionKey: 'condition:frightened',
      conditionPt: 'Amedrontado',
      duration: '1 minute',
    },
  } as never);
const reminder = () =>
  creatureAction({
    key: 'monster:ghost#etherealness',
    name: 'Etherealness',
    namePt: 'Forma etérea',
    text: 'The ghost enters the Ethereal Plane from the Material Plane, or vice versa.',
    kind: CreatureActionKind.OTHER,
    planClass: CreatureActionClass.TEXT,
  });
const lore = (): Creature =>
  create(CreatureSchema, { actions: [breath(), gaze(), reminder()] as never });

const hound = combatant({ id: 'hound', label: 'Cão Infernal', side: CombatantSide.ENEMY });
const toren = combatant({ id: 'toren', label: 'Toren', side: CombatantSide.PARTY });
const brisa = combatant({ id: 'brisa', label: 'Brisa', side: CombatantSide.PARTY });
const orc = combatant({ id: 'orc', label: 'Orc', side: CombatantSide.ENEMY });

const sentResult = () =>
  create(CreatureActionResultSchema, {
    castId: 'cast-1',
    actionKey: 'monster:hell-hound#fire-breath',
    targets: [
      {
        combatantId: 'toren',
        save: {
          outcome: SaveOutcome.FAILED,
          dc: 13,
          roll: { diceCount: 1, diceSides: 20, faces: [5], modifier: 1, total: 6 },
        },
        pendingDamageId: 'pd1',
      },
      {
        combatantId: 'brisa',
        save: {
          outcome: SaveOutcome.SAVED,
          dc: 13,
          roll: { diceCount: 1, diceSides: 20, faces: [17], modifier: 1, total: 18 },
        },
        pendingDamageId: 'pd2',
      },
    ],
    pendingDamages: [
      create(PendingDamageSchema, { id: 'pd1' }),
      create(PendingDamageSchema, { id: 'pd2' }),
    ],
  });

async function open(
  over: {
    theatre?: boolean;
    spent?: boolean;
    useCreatureAction?: ReturnType<typeof vi.fn>;
    limited?: readonly MessageInitShape<typeof LimitedActionSchema>[];
  } = {},
) {
  TestBed.resetTestingModule();
  const subject = {
    ...hound,
    actionUsed: over.spent ?? false,
    limitedActions: (over.limited ?? []).map((l) => create(LimitedActionSchema, l)),
  } as typeof hound;
  const enc = encounter({ combatants: [subject, toren, brisa, orc] });
  const apply = vi.fn();
  const useCreatureAction =
    over.useCreatureAction ??
    vi.fn(async () => ({ encounter: enc, result: sentResult(), area: null }));
  TestBed.configureTestingModule({
    providers: [{ provide: CombatClient, useValue: fakeCombatClient({ useCreatureAction }) }],
  });
  const fixture = TestBed.createComponent(CreatureActions);
  const area: CreatureAreaRequest[] = [];
  fixture.componentInstance.pickArea.subscribe((r) => area.push(r));
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('encounter', enc);
  fixture.componentRef.setInput('subject', subject);
  fixture.componentRef.setInput('state', { apply } as unknown as CombatState);
  fixture.componentRef.setInput('creature', lore());
  fixture.componentRef.setInput('theatre', over.theatre ?? false);
  await settle(fixture);
  const root = fixture.nativeElement as HTMLElement;
  const use = (name: string) =>
    root.querySelector<HTMLButtonElement>(`button[aria-label="Usar ${name}"]`)!;
  return { fixture, root, use, useCreatureAction, apply, area };
}

const text = (root: HTMLElement) => (root.textContent ?? '').replace(/\s+/g, ' ');

describe('CreatureActions: the list, by the class of the plan', () => {
  it('names each action in Portuguese and says what it does, from the plan', async () => {
    const { root } = await open();
    const t = text(root);
    expect(t).toContain('Sopro de fogo');
    expect(t).toContain('Teste de resistência de Destreza, CD 13');
    expect(t).toContain('cone de 4,5 m');
    expect(t).toContain('Dano: 6d6 de fogo, metade se passar');
    expect(t).toContain('Semblante horripilante');
    expect(t).toContain('Se falhar: Amedrontado por 1 minuto');
    expect(t).toContain('Sem teste: gasta a ação');
  });

  it('says the limit of an action in Portuguese', async () => {
    const { fixture, root } = await open();
    const limited = breath();
    limited.usage = 'Recharge 5-6';
    fixture.componentRef.setInput(
      'creature',
      create(CreatureSchema, { actions: [limited] as never }),
    );
    await settle(fixture);
    const t = text(root);
    expect(t).toContain('Recarga 5\u20136');
    expect(t).not.toContain('Recharge');
  });

  it('folds the SRD text behind the document icon beside the name, only where the plan does not hold the whole action', async () => {
    const { fixture, root } = await open();
    const items = () => [...root.querySelectorAll('li.act')] as HTMLElement[];
    const doc = (li: HTMLElement) => li.querySelector<HTMLButtonElement>('button.act__doc');
    // Folded to begin with: no English text on the card, a button only on the actions that have some.
    expect(root.querySelector('[lang="en"]')).toBeNull();
    expect(doc(items()[0])).toBeNull(); // STRUCTURED
    expect(doc(items()[1])).not.toBeNull(); // PARTIAL
    expect(doc(items()[2])).not.toBeNull(); // TEXT / no roll
    expect(doc(items()[1])!.getAttribute('aria-expanded')).toBe('false');
    expect(doc(items()[1])!.getAttribute('aria-label')).toBe(
      'Texto do SRD de Semblante horripilante',
    );
    // Opened by the icon, closed by it again; a space separates the tag from the text.
    doc(items()[1])!.click();
    await settle(fixture);
    expect(doc(items()[1])!.getAttribute('aria-expanded')).toBe('true');
    expect(text(items()[1])).toContain('Texto do SRD, em inglês');
    expect(text(items()[1])).toContain('must succeed on a DC 13 Wisdom');
    expect(text(items()[1])).toContain('o app não calcula'); // PARTIAL
    expect(root.querySelector('.act__tag')?.nextElementSibling?.getAttribute('lang')).toBe('en');
    doc(items()[1])!.click();
    await settle(fixture);
    expect(root.querySelector('[lang="en"]')).toBeNull();
    doc(items()[2])!.click();
    await settle(fixture);
    expect(text(items()[2])).toContain('The ghost enters the Ethereal Plane'); // TEXT / no roll
  });

  it('lists nothing for a stat block with no such action', async () => {
    const { fixture, root } = await open();
    fixture.componentRef.setInput('creature', create(CreatureSchema, {}));
    await settle(fixture);
    expect(root.querySelector('section')).toBeNull();
  });
});

describe('CreatureActions: using a save action', () => {
  it('starts the targets on the hostile creatures (the party, for an enemy) and lets the master change them, in a labelled group', async () => {
    const { root } = await open({ theatre: true });
    const group = root.querySelector('[role="group"]')!;
    expect(group.getAttribute('aria-labelledby')).toBe('acts-targets');
    const boxes = [...group.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')];
    const state = Object.fromEntries(
      boxes.map((b) => [b.closest('li')!.querySelector(':scope > span')!.textContent, b.checked]),
    );
    expect(state).toMatchObject({ Toren: true, Brisa: true, Orc: false });
  });

  it('sends the key and the named targets, then shows each save against the DC and the damage it opened', async () => {
    const { root, use, useCreatureAction, apply, fixture } = await open({ theatre: true });
    const boxes = [...root.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')];
    boxes.find((b) => b.closest('li')!.textContent!.includes('Brisa'))!.click(); // off
    await settle(fixture);
    use('Sopro de fogo').click();
    await settle(fixture);
    expect(useCreatureAction).toHaveBeenCalledWith(
      'c',
      expect.any(String),
      'hound',
      'monster:hell-hound#fire-breath',
      ['toren'],
    );
    expect(apply).toHaveBeenCalled();
    const done = text(root.querySelector('[role="status"]') as HTMLElement);
    expect(done).toContain('Sopro de fogo');
    expect(done).toContain('Toren');
    expect(done).toContain('Falhou');
    expect(done).toContain('contra CD 13');
    expect(done).toContain('Brisa');
    expect(done).toContain('Passou: metade do dano');
  });

  it('is off, with the reason, when no target is named', async () => {
    const { root, use, fixture } = await open({ theatre: true });
    for (const b of root.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')) {
      if (b.checked) {
        b.click();
      }
    }
    await settle(fixture);
    const button = use('Sopro de fogo');
    expect(button.getAttribute('aria-disabled') ?? button.disabled.toString()).toBe('true');
    expect(text(root)).toContain('Escolha ao menos um alvo.');
    expect(button.getAttribute('aria-describedby')).toBe('act-why-0');
  });

  it('is off once the action of the turn is spent', async () => {
    const { root, use, useCreatureAction } = await open({ spent: true, theatre: true });
    use('Sopro de fogo').click();
    expect(useCreatureAction).not.toHaveBeenCalled();
    expect(text(root)).toContain('A ação deste turno já foi usada.');
  });

  it('says the refusal in words and keeps the actions', async () => {
    const { ConnectError, Code } = await import('@connectrpc/connect');
    const { root, use, fixture } = await open({
      theatre: true,
      useCreatureAction: vi.fn().mockRejectedValue(new ConnectError('no', Code.Unavailable)),
    });
    use('Semblante horripilante').click();
    await settle(fixture);
    expect(root.querySelector('[role="alert"]')).not.toBeNull();
    expect(root.querySelector('[role="status"]')).toBeNull();
  });
});

describe('CreatureActions: an action with an area, on a map', () => {
  it('asks the page for the map picker with the placement, sends no target, and keeps what the picker answers', async () => {
    const { use, area, useCreatureAction, fixture, root } = await open();
    use('Sopro de fogo').click();
    await settle(fixture);
    expect(useCreatureAction).not.toHaveBeenCalled();
    expect(area).toHaveLength(1);
    expect(area[0].placement).toBe(AreaPlacement.DIRECTION);
    expect(area[0].action.key).toBe('monster:hell-hound#fire-breath');
    // The picker used the action and answers with the result.
    area[0].done({ action: area[0].action, result: sentResult() });
    await settle(fixture);
    expect(text(root.querySelector('[role="status"]') as HTMLElement)).toContain('Falhou');
  });

  it('does nothing more when the master gives the picker up', async () => {
    const { root, use, area, fixture } = await open();
    use('Sopro de fogo').click();
    await settle(fixture);
    area[0].done(null);
    await settle(fixture);
    expect(root.querySelector('[role="status"]')).toBeNull();
    expect(use('Sopro de fogo').disabled).toBe(false);
  });
});

describe('CreatureActions: an action with no roll', () => {
  it('is used with no targets, writes the reminder and spends the action', async () => {
    const { root, use, useCreatureAction, fixture } = await open({
      useCreatureAction: vi.fn(async () => ({
        encounter: encounter({}),
        result: create(CreatureActionResultSchema, {
          actionKey: 'monster:ghost#etherealness',
          reminder: true,
        }),
        area: null,
      })),
    });
    use('Forma etérea').click();
    await settle(fixture);
    expect(useCreatureAction).toHaveBeenCalledWith(
      'c',
      expect.any(String),
      'hound',
      'monster:ghost#etherealness',
      [],
    );
    expect(text(root.querySelector('[role="status"]') as HTMLElement)).toContain(
      'Lembrete registrado',
    );
  });
});

describe('CreatureActions: the limits of the actions (RN-29)', () => {
  const breath = {
    actionKey: 'monster:hell-hound#fire-breath',
    kind: 'recharge',
    usage: 'Recharge 5-6',
    usesAllowed: 1,
  };

  it('tags a ready Recharge action "Pronta" and leaves "Usar" on', async () => {
    const { root, use } = await open({ limited: [{ ...breath, available: true, usesLeft: 1 }] });
    expect(text(root)).toContain('Pronta');
    expect(use('Sopro de fogo').disabled).toBe(false);
  });

  it('tags a spent Recharge action "Gasta", turns "Usar" off and says when it comes back', async () => {
    const { root, use, useCreatureAction } = await open({
      limited: [{ ...breath, available: false, usesLeft: 0 }],
    });
    expect(text(root)).toContain('Gasta');
    expect(text(root)).toContain(
      'Volta com 5 ou 6 no d6 que o app rola no começo de cada turno do Cão Infernal.',
    );
    use('Sopro de fogo').click();
    expect(useCreatureAction).not.toHaveBeenCalled();
    expect(use('Sopro de fogo').getAttribute('aria-disabled')).toBe('true');
    expect(use('Sopro de fogo').getAttribute('aria-describedby')).not.toBeNull();
  });

  it('counts the uses a day with dots and "N de M"', async () => {
    const { root } = await open({
      limited: [
        {
          actionKey: breath.actionKey,
          kind: 'per_day',
          usage: '3/day',
          usesAllowed: 3,
          usesLeft: 2,
          available: true,
        },
      ],
    });
    expect(text(root)).toContain('2 de 3');
    expect(root.querySelectorAll('.act__dot--left')).toHaveLength(2);
    expect(root.querySelectorAll('.act__dot')).toHaveLength(3);
  });

  it('shows no notice and no state for a creature the server sent no limits for', async () => {
    const { root } = await open({});
    expect(root.querySelector('[data-testid="creature-keeps"]')).toBeNull();
    expect(root.querySelector('.act__tag-state')).toBeNull();
  });

  it('says a persistent NPC keeps what it spent', async () => {
    const { root } = await open({
      limited: [
        {
          actionKey: breath.actionKey,
          kind: 'per_day',
          usage: '1/day',
          usesAllowed: 1,
          usesLeft: 0,
          kept: true,
        },
      ],
    });
    expect(root.querySelector('[data-testid="creature-keeps"]')?.textContent).toContain(
      'Cão Infernal guarda o que gastou.',
    );
    expect(text(root)).toContain('Sem usos: voltam no descanso longo da mesa.');
  });
});

describe('CreatureActions: reading the stat block', () => {
  async function opened(state: 'loading' | 'failed' | 'ready') {
    const o = await open({});
    o.fixture.componentRef.setInput('blockState', state);
    await settle(o.fixture);
    return o;
  }

  it('keeps one live region mounted through loading, failing and loading again, changing only its text', async () => {
    const { fixture, root } = await opened('loading');
    const live = root.querySelector('.acts__live');
    expect(live?.getAttribute('aria-live')).toBe('polite');
    expect(live?.textContent).toContain('Lendo a ficha da criatura…');
    fixture.componentRef.setInput('blockState', 'failed');
    await settle(fixture);
    expect(root.querySelector('.acts__live')).toBe(live);
    expect(live?.textContent).toContain('Não deu para ler as outras ações da criatura.');
    expect(root.querySelector('[role="alert"]')).toBeNull();
    fixture.componentRef.setInput('blockState', 'loading');
    await settle(fixture);
    expect(root.querySelector('.acts__live')).toBe(live);
    expect(live?.textContent).toContain('Lendo a ficha da criatura…');
  });

  it('keeps the idle live region in the accessibility tree: its box is not display none', async () => {
    const { root } = await opened('ready');
    const live = root.querySelector('.acts__live');
    expect(live).not.toBeNull();
    expect(live?.textContent?.trim()).toBe('');
    expect(getComputedStyle(live as Element).display).not.toBe('none');
    expect(getComputedStyle(live as Element).visibility).not.toBe('hidden');
  });

  it('keeps the focus on the section heading when "Tentar de novo" swaps the button for the skeleton', async () => {
    const { fixture, root } = await opened('failed');
    const retried: number[] = [];
    fixture.componentInstance.retry.subscribe(() => retried.push(1));
    const retry = [...root.querySelectorAll('button')].find((b) =>
      (b.textContent ?? '').includes('Tentar de novo'),
    )!;
    retry.focus();
    expect(document.activeElement).toBe(retry);
    retry.click();
    fixture.componentRef.setInput('blockState', 'loading');
    await settle(fixture);
    expect(retried).toHaveLength(1);
    expect(root.querySelector('button.offer')).toBeNull();
    expect(document.activeElement).toBe(root.querySelector('#acts-title'));
    expect(document.activeElement?.tagName).not.toBe('BODY');
  });
});
