import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
} from '../../../../../gen/meurpg/characters/v1/characters_pb';
import {
  CombatDeathSchema,
  CombatantKind,
  CombatantSide,
  EncounterStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { RevivifyClient } from '../../../../core/revivify/revivify-client';
import type { CombatantInfo } from '../combat-info';
import { DeathsBlock } from './deaths-block';
import { nextTurn } from '../../../../../test-settle';

/** The text as the eye reads it: the lines join with a no-break space before the dot. */
const plain = (node: Element | null) => (node?.textContent ?? '').replace(/\u00a0/g, ' ');

const death = (over: Parameters<typeof create<typeof CombatDeathSchema>>[1]) =>
  create(CombatDeathSchema, over);

const toren = death({
  combatantId: 'toren',
  name: 'Toren',
  deathRound: 3,
  fitsRevivify: true,
  revivifyBlocked: false,
  isPlayerCharacter: true,
  characterId: 'toren-c',
});
const goblin = death({
  combatantId: 'g2',
  name: 'Goblin 2',
  deathRound: 6,
  fitsRevivify: true,
  revivifyBlocked: false,
  isPlayerCharacter: false,
});

const info = new Map<string, CombatantInfo>([
  ['toren-c', { classSummary: 'Guerreiro 4', playerName: 'Davi', kindLabel: '', raceName: '' }],
  ['char-g2', { classSummary: '', playerName: null, kindLabel: 'Inimigo', raceName: '' }],
]);

const rows = [
  combatant({
    id: 'toren',
    label: 'Toren',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'toren-c',
    defeated: true,
  }),
  combatant({ id: 'g2', label: 'Goblin 2', defeated: true }),
];

describe('DeathsBlock', () => {
  const calls: string[] = [];
  let reviveFn: () => Promise<void>;
  let blockedFn: () => Promise<void>;

  beforeEach(() => {
    calls.length = 0;
    reviveFn = () => Promise.resolve();
    blockedFn = () => Promise.resolve();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: RevivifyClient,
          useValue: {
            revive: (_c: string, characterId: string, key: string) => {
              calls.push(`revive ${characterId} ${key === '' ? 'nokey' : 'key'}`);
              return reviveFn();
            },
            setBlocked: (_c: string, combatantId: string, blocked: boolean) => {
              calls.push(`blocked ${combatantId} ${blocked}`);
              return blockedFn();
            },
          },
        },
      ],
    });
  });

  function render(
    over: { deaths?: ReturnType<typeof death>[]; round?: number; status?: EncounterStatus } = {},
  ) {
    const fixture = TestBed.createComponent(DeathsBlock);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        round: over.round ?? 8,
        status: over.status ?? EncounterStatus.ACTIVE,
        combatants: rows,
        deaths: over.deaths ?? [toren, goblin],
      }),
    );
    fixture.componentRef.setInput('info', info);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const row = (name: string) =>
      Array.from(el.querySelectorAll('li.death')).find(
        (li) => li.getAttribute('aria-label') === name,
      ) as HTMLElement;
    const button = (scope: ParentNode, text: string) =>
      Array.from(scope.querySelectorAll('button')).find((b) => b.textContent?.trim() === text)!;
    return { fixture, el, row, button };
  }

  it('lists who died in this fight, with the round and how long ago', () => {
    const { el, row } = render();

    expect(el.querySelector('h2')?.textContent?.trim()).toBe('Mortos nesta luta (2)');
    expect(el.querySelector('.deaths__sub')?.textContent).toBe(
      'Rodada 8 · um minuto são 10 rodadas',
    );
    expect(plain(row('Toren').querySelector('.death__line'))).toBe(
      'Guerreiro 4 · morreu na rodada 3 (há 5)',
    );
    expect(plain(row('Goblin 2').querySelector('.death__line'))).toBe(
      'Inimigo · morreu na rodada 6 (há 2)',
    );
  });

  it('says to the master alone why a creature may not fit', () => {
    const { el } = render();
    expect(el.querySelector('.deaths__foot')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      'Só o mestre vê os motivos quando uma criatura não cabe: longe do conjurador, mais de 10 rodadas, escondida ou marcada “não funciona”.',
    );
  });

  it('shows "Cabe em Revivificar" only when it fits and the switch is off', () => {
    const { row } = render({
      deaths: [
        toren,
        death({ ...goblin, fitsRevivify: false }),
        death({ ...goblin, combatantId: 'g3', name: 'Goblin 3', revivifyBlocked: true }),
      ],
    });

    expect(row('Toren').textContent).toContain('Cabe em Revivificar');
    expect(row('Goblin 2').textContent).not.toContain('Cabe em Revivificar');
    expect(row('Goblin 3').textContent).not.toContain('Cabe em Revivificar');
  });

  it('gives "Reviver" to a player\'s character and not to an NPC', () => {
    const { row, button } = render();
    expect(button(row('Toren'), 'Reviver')).toBeTruthy();
    expect(button(row('Goblin 2'), 'Reviver')).toBeUndefined();
  });

  it('draws nothing without deaths, and nothing once the combat ended', () => {
    expect(render({ deaths: [] }).el.querySelector('.deaths')).toBeNull();
    expect(render({ status: EncounterStatus.ENDED }).el.querySelector('.deaths')).toBeNull();
  });

  it('asks the same question as the page, focused, and revives with a key', async () => {
    const { fixture, el, row, button } = render();
    button(row('Toren'), 'Reviver').click();
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();

    const dialog = el.querySelector('[role="alertdialog"]')!;
    expect(dialog.querySelector('h2')?.textContent).toBe('Reviver Toren?');
    expect(document.activeElement).toBe(button(dialog, 'Reviver Toren'));
    expect(calls).toEqual([]);

    button(dialog, 'Reviver Toren').click();
    await nextTurn();
    fixture.detectChanges();

    expect(calls).toEqual(['revive toren-c key']);
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(row('Toren')).toBeUndefined();
    expect(el.querySelector('h2')?.textContent?.trim()).toBe('Mortos nesta luta (1)');
  });

  it('cancels with Escape and puts the focus back on "Reviver"', async () => {
    const { fixture, el, row, button } = render();
    button(row('Toren'), 'Reviver').click();
    fixture.detectChanges();
    el.querySelector('[role="alertdialog"]')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    );
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();

    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(document.activeElement).toBe(button(row('Toren'), 'Reviver'));
    expect(calls).toEqual([]);
  });

  it('refuses with the card of the other living character, and links to it', async () => {
    reviveFn = () =>
      Promise.reject(
        new ConnectError('blocked', Code.FailedPrecondition, undefined, [
          {
            desc: CharacterBlockedSchema,
            value: {
              reason: CharacterBlockedReason.LIVING_CHARACTER_EXISTS,
              characterId: 'nuvem-1',
              livingCharacterName: 'Nuvem',
            },
          },
        ]),
      );
    const { fixture, el, row, button } = render();
    button(row('Toren'), 'Reviver').click();
    fixture.detectChanges();
    button(el.querySelector('[role="alertdialog"]')!, 'Reviver Toren').click();
    await nextTurn();
    fixture.detectChanges();

    const card = row('Toren').querySelector('[role="alert"]')!;
    expect(card.textContent).toContain('criou Nuvem depois da morte de Toren');
    expect(card.querySelector('a')?.getAttribute('href')).toBe(
      '/campaigns/camp-1/characters/nuvem-1',
    );
    expect(row('Toren')).toBeTruthy();
  });

  it('sets the switch for the combatant and shows the notice for the master alone', async () => {
    const { fixture, row } = render();
    const toggle = row('Goblin 2').querySelector<HTMLButtonElement>('[role="switch"]')!;
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    expect(row('Goblin 2').textContent).not.toContain('Revivificar não vai aparecer');

    toggle.click();
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();

    expect(calls).toEqual(['blocked g2 true']);
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    expect(row('Goblin 2').textContent).toContain('Revivificar não vai aparecer para ele.');
    expect(row('Goblin 2').textContent).not.toContain('Cabe em Revivificar');
  });

  it('puts the switch back and says why when the server refuses it', async () => {
    blockedFn = () => Promise.reject(new ConnectError('gone', Code.NotFound));
    const { fixture, row } = render();
    const toggle = row('Goblin 2').querySelector<HTMLButtonElement>('[role="switch"]')!;
    toggle.click();
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();

    expect(toggle.getAttribute('aria-checked')).toBe('false');
    expect(row('Goblin 2').querySelector('[role="alert"]')).not.toBeNull();
  });

  it('shows the notice of a switch the server already has on', () => {
    const { row } = render({ deaths: [death({ ...goblin, revivifyBlocked: true })] });
    expect(row('Goblin 2').querySelector('[role="switch"]')?.getAttribute('aria-checked')).toBe(
      'true',
    );
    expect(row('Goblin 2').textContent).toContain('Revivificar não vai aparecer para ele.');
  });
});
