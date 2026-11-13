import { TestBed } from '@angular/core/testing';

import { WallSide } from '../../../gen/meurpg/play/v1/combat_pb';
import { type WallChoice, newWall } from '../../core/combat/spell-wall';
import { WallControls } from './wall-controls';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

/** The words of a radio's card, without the icon's name. */
function words(r: HTMLInputElement): string {
  const card = r.closest('label')!.cloneNode(true) as HTMLElement;
  card.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return plain(card.textContent);
}

function setup(
  wall: WallChoice,
  part: 'shape' | 'size' = 'size',
  dice = '5d8',
  extra: { loading?: boolean; stops?: string | null } = {},
) {
  const fixture = TestBed.createComponent(WallControls);
  const ref = fixture.componentRef;
  ref.setInput('wall', wall);
  ref.setInput('part', part);
  ref.setInput('dice', dice);
  ref.setInput('loading', extra.loading ?? false);
  ref.setInput('stops', extra.stops ?? null);
  const changes: WallChoice[] = [];
  fixture.componentInstance.changed.subscribe((w) => {
    changes.push(w);
    ref.setInput('wall', w);
  });
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (label: string) =>
    el.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
  const radios = (group: string) =>
    Array.from(
      el.querySelectorAll<HTMLInputElement>(`[role="radiogroup"][aria-label="${group}"] input`),
    );
  return { fixture, el, changes, button, radios };
}

describe('WallControls: the shape', () => {
  it('offers Linha reta and Anel as one labelled group, the laid one checked', () => {
    const { radios, el } = setup(newWall(false), 'shape');
    const shape = radios('Forma da muralha');
    expect(shape.map((r) => words(r))).toEqual(['Linha reta', 'Anel']);
    expect(shape.map((r) => r.checked)).toEqual([true, false]);
    expect(el.querySelector('[aria-label^="Direção da muralha"]')).toBeNull();
  });

  it('changing to Anel sends a ring at its own size, without direction', () => {
    const { radios, changes } = setup(newWall(false), 'shape');
    radios('Forma da muralha')[1].click();
    expect(changes).toEqual([newWall(true)]);
  });
});

describe('WallControls: a straight wall', () => {
  it('has eight labelled direction buttons that say which is chosen, and no more than the eight', () => {
    const { el, button } = setup(newWall(false));
    const pad = el.querySelectorAll('[role="group"][aria-label^="Direção da muralha"] button');
    expect(pad).toHaveLength(8);
    expect(button('Para o leste').getAttribute('aria-pressed')).toBe('true');
    expect(button('Para o norte').getAttribute('aria-pressed')).toBe('false');
  });

  it('names the direction group with the way the wall runs now', () => {
    const { el, button, fixture } = setup(newWall(false));
    expect(
      el
        .querySelector('[role="group"][aria-label^="Direção da muralha"]')
        ?.getAttribute('aria-label'),
    ).toBe('Direção da muralha: leste');
    button('Para o sul').click();
    fixture.detectChanges();
    expect(
      el
        .querySelector('[role="group"][aria-label^="Direção da muralha"]')
        ?.getAttribute('aria-label'),
    ).toBe('Direção da muralha: sul');
  });

  it('turns the wall when a direction is pressed', () => {
    const { button, changes } = setup(newWall(false));
    button('Para o nordeste').click();
    expect(changes[0].direction).toEqual({ dx: 1, dy: -1 });
    expect(changes[0].ring).toBe(false);
  });

  it('says the length in meters, steps it by 1,5 m and says how many squares', () => {
    const { el, button, changes } = setup({ ...newWall(false), lengthFt: 40 });
    expect(plain(el.querySelector('output')?.textContent)).toBe('12 m');
    expect(plain(el.textContent)).toContain('De 1,5 m a 18 m: 8 quadrados.');
    button('Diminuir o comprimento em 1,5 m').click();
    button('Aumentar o comprimento em 1,5 m').click();
    expect(changes.map((w) => w.lengthFt)).toEqual([35, 40]);
  });

  it('does not step past the least or the most and says so to a reader', () => {
    const { button, changes } = setup({ ...newWall(false), lengthFt: 60 });
    expect(button('Aumentar o comprimento em 1,5 m').getAttribute('aria-disabled')).toBe('true');
    button('Aumentar o comprimento em 1,5 m').click();
    expect(changes).toEqual([]);
  });

  it('offers the sides by where they lie on the screen, none picked, with the arrow note and the dice of the slot', () => {
    const { el, radios, changes } = setup(newWall(false));
    const side = radios('Lado que queima');
    expect(side.map((r) => words(r))).toEqual(['Acima', 'Abaixo']);
    expect(side.map((r) => r.checked)).toEqual([false, false]);
    expect(plain(el.textContent)).toContain('Escolha o lado');
    expect(plain(el.textContent)).toContain(
      'A seta no mapa mostra o lado: 5d8 de fogo para quem termina o turno ali, até 3 m.',
    );
    side[0].click();
    expect(changes[0].side).toBe(WallSide.LEFT);
  });

  it('does not warn once a side is picked, and names the sides again when the wall turns', () => {
    const { el, radios, button, fixture } = setup({ ...newWall(false), side: WallSide.RIGHT });
    expect(plain(el.textContent)).not.toContain('Escolha o lado');
    expect(radios('Lado que queima')[1].checked).toBe(true);
    button('Para o sul').click();
    fixture.detectChanges();
    expect(radios('Lado que queima').map((r) => words(r))).toEqual(['À direita', 'À esquerda']);
    expect(radios('Lado que queima').map((r) => r.checked)).toEqual([false, false]);
    expect(plain(el.textContent)).toContain('Escolha o lado');
  });
});

describe('WallControls: what the server is doing', () => {
  it('says it is working out where the wall goes, with the hourglass, in a live region', () => {
    const { el } = setup(newWall(false), 'shape', '5d8', { loading: true });
    const live = el.querySelector('[role="status"]')!;
    expect(plain(live.textContent)).toContain('Calculando onde a muralha fica…');
    expect(live.querySelector('mat-icon')?.textContent).toBe('hourglass_empty');
    const idle = setup(newWall(false), 'shape');
    expect(idle.el.querySelector('[role="status"]')?.textContent?.trim()).toBe('');
  });

  it('warns when the wall stopped at a wall of the map', () => {
    const { el } = setup(newWall(false), 'shape', '5d8', {
      stops: 'A muralha para na parede: ficou com 9 m (6 quadrados) em vez de 12 m.',
    });
    expect(plain(el.querySelector('[role="status"]')?.textContent)).toContain(
      'A muralha para na parede: ficou com 9 m (6 quadrados) em vez de 12 m.',
    );
  });
});

describe('WallControls: a ring', () => {
  it('has the diameter and inside or outside, and no direction and no left or right', () => {
    const { el, radios, button } = setup(newWall(true));
    expect(el.querySelector('[aria-label^="Direção da muralha"]')).toBeNull();
    expect(plain(el.textContent)).toContain('Diâmetro');
    expect(plain(el.querySelector('output')?.textContent)).toBe('6 m');
    expect(plain(el.textContent)).toContain('3 m ou 6 m de diâmetro.');
    expect(radios('Lado que queima').map((r) => words(r))).toEqual(['Dentro', 'Fora']);
    expect(button('Diminuir o diâmetro em 3 m')).toBeTruthy();
    expect(plain(el.textContent)).toContain('no lado que queima');
  });
});
