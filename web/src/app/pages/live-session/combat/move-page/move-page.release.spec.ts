import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CombatantKind,
  GetMoveOptionsResponseSchema,
  ReachableSquareSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { JumpLimitsSchema } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { type MoveRelease, moveReleaseView } from '../../../../core/combat/move-release';
import { MovePage } from './move-page';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

// Tor's turn is over (0 m left of it), and the Ogro is on turn: the readied move spends the server's own budget, 9,0 m.
const tor = combatant({
  id: 'tor',
  label: 'Tor',
  kind: CombatantKind.PLAYER,
  mine: true,
  col: 3,
  row: 4,
  speedFt: 30,
  speedDft: 300,
  movementLeftFt: 0,
  movementLeftDft: 0,
  movementUsedDft: 300,
});
const ogro = combatant({ id: 'og', label: 'Ogro', col: 9, row: 4 });

const options = create(GetMoveOptionsResponseSchema, {
  movementLeftDft: 300,
  reachable: [create(ReachableSquareSchema, { col: 4, row: 4, costDft: 50, leftAfterDft: 250 })],
});

const held: MoveRelease = {
  windowId: 'w1',
  readiedId: 'r1',
  reactorId: 'tor',
  reactor: 'Tor',
  trigger: 'Quando o ogro passar pela porta',
  master: false,
};

function setup(over: { master?: boolean; gone?: boolean } = {}) {
  const fixture = TestBed.createComponent(MovePage);
  const ref = fixture.componentRef;
  ref.setInput(
    'encounter',
    encounter({ combatants: [tor, ogro], currentCombatantId: 'og', turnGroupIds: ['og'] }),
  );
  ref.setInput('image', { url: '/images/x', width: 2000, height: 1400 });
  ref.setInput('mapName', 'A cripta');
  ref.setInput('sessionNumber', 6);
  ref.setInput('options', options);
  ref.setInput('layers', {
    columns: 20,
    rows: 14,
    walls: [],
    terrain: [],
    half: [],
    threeQuarters: [],
  });
  // A character with a long jump: the readied move is a walk, so the page keeps no way to jump.
  ref.setInput(
    'jumps',
    create(JumpLimitsSchema, { longStandingDft: 100, longRunningDft: 200 } as never),
  );
  ref.setInput('canDisengage', false);
  ref.setInput('moverId', over.master ? 'tor' : '');
  ref.setInput(
    'release',
    moveReleaseView(
      { ...held, master: over.master ?? false },
      'vez do Ogro',
      over.gone
        ? { lead: 'A janela fechou.', rest: 'Você não se moveu, e a reação continua livre.' }
        : null,
    ),
  );
  const backed = vi.fn();
  fixture.componentInstance.back.subscribe(backed);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (name: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => plain(b.textContent).includes(name));
  return { fixture, el, button, backed };
}

describe('MovePage: a readied move released off the readier’s turn (#856, boards 857-06 and 857-07)', () => {
  it('titles it "Mover Tor" with the band, and leads with the readied budget, not the movement of the turn that ended', () => {
    const { el } = setup();
    expect(plain(el.querySelector('h1')?.textContent)).toBe('Mover Tor');
    expect(plain(el.querySelector('[data-testid=release-band]')?.textContent)).toContain(
      'Ação preparada: “Quando o ogro passar pela porta”Fora da sua vez · vez do Ogro · gasta a ■ reação',
    );
    expect(plain(el.querySelector('.move__lead')?.textContent)).toContain('Restam 9,0 m de 9,0 m');
  });

  it('reads the band before the budget for the player (857-06) and the budget before the band for the master (857-07)', () => {
    const order = (el: HTMLElement) =>
      Array.from(
        el.querySelectorAll('.move__main > .move__lead, .move__main > app-release-band'),
      ).map((n) => n.tagName.toLowerCase() + (n.classList.contains('move__lead') ? '.lead' : ''));
    expect(order(setup().el)).toEqual(['app-release-band', 'p.lead']);
    expect(order(setup({ master: true }).el)).toEqual(['p.lead', 'app-release-band']);
  });

  it('names the mover in the legend when the master moves for someone ("Tor alcança"), and keeps "Você alcança" for the player (857-07)', () => {
    const legend = (el: HTMLElement) =>
      Array.from(el.querySelectorAll('.mr-legend li'), (li) => plain(li.textContent));
    expect(legend(setup({ master: true }).el)).toContain('Tor alcança');
    expect(legend(setup().el)).toContain('Você alcança');
  });

  it('keeps no way to jump or to disengage: the server’s budget is a walk', () => {
    const { el } = setup();
    expect(el.querySelector('app-segmented')).toBeNull();
    expect(plain(el.textContent)).not.toContain('Desengajar');
  });

  it('says what the move costs under the cost card, and puts "Voltar" in place of "Cancelar"', () => {
    const { el, button, backed } = setup();
    expect(plain(el.querySelector('[data-testid=release-note]')?.textContent)).toBe(
      'É o movimento preparado: gasta a ■ Reação e não tira nada do deslocamento da sua vez. Vale um movimento só.',
    );
    expect(button('Cancelar')).toBeUndefined();
    button('Voltar')!.click();
    expect(backed).toHaveBeenCalled();
    expect(el.querySelector('[data-testid=release-foot]')).toBeNull();
  });

  it('puts the cost and the note in one card once a square is chosen, with no "Depois restam", and waits for the table above the buttons (857-06)', () => {
    const { el, fixture } = setup();
    const surface = el.querySelector<HTMLElement>('.cm__surface')!;
    surface.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        width: 200,
        height: 140,
        right: 200,
        bottom: 140,
        x: 0,
        y: 0,
        toJSON: () => '',
      }) as DOMRect;
    surface.dispatchEvent(new MouseEvent('click', { clientX: 45, clientY: 45, bubbles: true }));
    fixture.detectChanges();
    const card = el.querySelector('.status');
    expect(plain(card?.querySelector('.status__title')?.textContent)).toBe('Mover 1,5 m');
    expect(plain(card?.textContent)).toContain(
      'É o movimento preparado: gasta a ■ Reação e não tira nada do deslocamento da sua vez. Vale um movimento só.',
    );
    expect(plain(card?.textContent)).not.toContain('Depois restam');
    expect(el.querySelector('.move__release-note')).toBeNull();
    expect(plain(el.querySelector('[data-testid=release-wait]')?.textContent)).toBe(
      'A mesa espera você: até 2 min.',
    );
  });

  it('says the master’s band with who he answers for, and the footer that the player may still answer', () => {
    const { el } = setup({ master: true });
    expect(plain(el.querySelector('[data-testid=release-band]')?.textContent)).toContain(
      'Ação preparada de Tor:',
    );
    expect(plain(el.querySelector('[data-testid=release-band]')?.textContent)).toContain(
      'você responde por Tor',
    );
    expect(plain(el.querySelector('[data-testid=release-foot]')?.textContent)).toBe(
      'Tor ainda pode responder no próprio aparelho: vale a primeira resposta.',
    );
    expect(plain(el.querySelector('[data-testid=release-note]')?.textContent)).toContain(
      'É o movimento preparado de Tor: gasta a ■ Reação de Tor',
    );
  });

  it('caps the map width on the release page only, and names the mover in the legend only for the master', () => {
    const player = setup();
    expect(player.el.querySelector('.move__map')?.classList.contains('move__map--release')).toBe(
      true,
    );
    expect(plain(player.el.querySelector('.move__legend')?.textContent)).toContain('Você alcança');
    const master = setup({ master: true });
    expect(plain(master.el.querySelector('.move__legend')?.textContent)).toMatch(/\S+ alcança/);
    expect(plain(master.el.querySelector('.move__legend')?.textContent)).not.toContain('undefined');
  });

  it('drops the move when the window went away: the notice and "Voltar à sessão" only (857-09 card 7)', () => {
    const { el, button, backed } = setup({ gone: true });
    expect(plain(el.querySelector('[data-testid=release-gone]')?.textContent)).toContain(
      'A janela fechou. Você não se moveu, e a reação continua livre.',
    );
    expect(button('Mover para cá')).toBeUndefined();
    // The map, the move text and the range notice go away with the window.
    expect(el.querySelector('app-combat-map')).toBeNull();
    expect(el.querySelector('.move__lead')).toBeNull();
    expect(el.querySelector('[data-testid=release-note]')).toBeNull();
    expect(el.querySelector('[data-testid=release-band]')).toBeNull();
    expect(plain(el.textContent)).not.toContain('Não deu para ler o alcance');
    expect(
      Array.from(el.querySelectorAll('.move__foot button')).map((x) => plain(x.textContent)),
    ).toEqual(['Voltar à sessão']);
    button('Voltar à sessão')!.click();
    expect(backed).toHaveBeenCalled();
  });
});
