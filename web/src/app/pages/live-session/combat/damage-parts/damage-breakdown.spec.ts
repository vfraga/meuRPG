import { TestBed } from '@angular/core/testing';

import { DamageStepKind } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { DamageBreakdown } from './damage-breakdown';

const rolls = [
  {
    partKey: 'weapon',
    labelPt: 'Espada longa',
    diceCount: 2,
    diceSides: 6,
    faces: [4, 5],
    flat: 3,
    sum: 9,
    counted: true,
    rerolled: [{ index: 0, from: 1, to: 4 }],
  },
  {
    partKey: 'sneak-attack',
    labelPt: 'Ataque Furtivo',
    diceCount: 2,
    diceSides: 6,
    faces: [3, 6],
    flat: 0,
    sum: 9,
    counted: true,
    rerolled: [],
  },
  {
    partKey: 'divine-smite-extra',
    labelPt: 'Golpe Divino contra morto-vivo',
    diceCount: 1,
    diceSides: 8,
    faces: [5],
    flat: 0,
    sum: 5,
    counted: false,
    rerolled: [],
  },
] as never[];
const fire: Record<string, unknown> = {
  kind: DamageStepKind.RESISTANCE,
  sourceKeys: ['race:tiefling'],
  labelPt: 'Resistência a fogo (tiefling)',
  before: 10,
  after: 5,
  ignored: false,
};

function setup(inputs: Record<string, unknown>) {
  const fixture = TestBed.createComponent(DamageBreakdown);
  fixture.componentRef.setInput('rolls', rolls);
  for (const [k, v] of Object.entries(inputs)) {
    fixture.componentRef.setInput(k, v);
  }
  const removed: { partKey: string; reason: string }[] = [];
  fixture.componentInstance.remove.subscribe((r) => removed.push(r));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const plain = () => el.textContent!.replace(/\s+/g, ' ');
  const button = (name: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(name))!;
  return { fixture, el, plain, button, removed };
}

describe('DamageBreakdown for the player', () => {
  it('lists what each part rolled, what was rolled again and what does not count', () => {
    const { plain } = setup({ amount: 21 });
    expect(plain()).toContain('Espada longa 2d6 (4, 5) + 3 = 12');
    expect(plain()).toContain('rolou de novo 1→4');
    expect(plain()).toContain('Golpe Divino contra morto-vivo 1d8 (5) = 5 não conta');
  });

  it('writes the resistance line and the damage after it', () => {
    const { plain } = setup({ amount: 10, steps: [fire], amountAfterSteps: 5 });
    expect(plain()).toContain('Resistência a fogo (tiefling): 10 → 5');
    expect(plain()).toContain('Dano depois dos ajustes: 5');
  });

  it('shows the rider of a maneuver only when the server sent it (the master)', () => {
    const golpe = {
      partKey: 'feature:golpe-da-garca@mesa',
      labelPt: 'Golpe da Garça',
      diceCount: 1,
      diceSides: 8,
      faces: [4],
      flat: 0,
      sum: 4,
      counted: true,
      rerolled: [],
      riderPt: 'O alvo faz um teste de Sabedoria.',
    };
    const withRider = setup({ amount: 8, rolls: [...rolls, golpe] });
    expect(withRider.plain()).toContain('Golpe da Garça 1d8 (4) = 4');
    expect(withRider.plain()).toContain('O alvo faz um teste de Sabedoria.');
    const player = setup({ amount: 8, rolls: [...rolls, { ...golpe, riderPt: '' }] });
    expect(player.plain()).not.toContain('teste de Sabedoria');
  });

  it('has no controls for the player', () => {
    const { el } = setup({ amount: 10, steps: [fire] });
    expect(el.querySelector('button')).toBeNull();
    expect(el.querySelector('input')).toBeNull();
  });
});

describe('DamageBreakdown for the master', () => {
  it('is a table by part with "Tirar" only on the extras that count', () => {
    const { el } = setup({
      master: true,
      amount: 21,
      removable: new Set(['sneak-attack', 'divine-smite-extra']),
    });
    expect(el.querySelectorAll('tbody tr')).toHaveLength(3);
    const takes = Array.from(el.querySelectorAll('button')).map((b) =>
      b.getAttribute('aria-label'),
    );
    expect(takes).toEqual(['Tirar o Ataque Furtivo']);
  });

  it('asks in place how far the damage falls and needs a reason before it takes the part out', () => {
    const { fixture, el, plain, button, removed } = setup({
      master: true,
      amount: 21,
      removable: new Set(['sneak-attack']),
    });
    button('Tirar').click();
    fixture.detectChanges();
    expect(plain()).toContain('Tirar o Ataque Furtivo? O dano cai de 21 para 12.');
    const confirm = Array.from(el.querySelectorAll('.ask button')).find(
      (b) => b.textContent!.trim() === 'Tirar',
    )!;
    expect(confirm.getAttribute('aria-disabled')).toBe('true');
    const field = el.querySelector<HTMLInputElement>('.ask__field')!;
    expect(document.activeElement).toBe(field);
    field.value = 'a mesa combinou que não vale';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    (confirm as HTMLButtonElement).click();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    expect(removed).toEqual([{ partKey: 'sneak-attack', reason: 'a mesa combinou que não vale' }]);
  });

  it('offers "Ignorar a resistência" on a step that stands, and sends its source keys', () => {
    const { fixture, el } = setup({ master: true, amount: 10, steps: [fire] });
    const box = el.querySelector<HTMLInputElement>('.step__toggle input')!;
    expect(el.querySelector('.step__toggle')!.textContent).toContain('Ignorar a resistência');
    box.click();
    fixture.detectChanges();
    expect(fixture.componentInstance.ignored()).toEqual(['race:tiefling']);
    box.click();
    expect(fixture.componentInstance.ignored()).toEqual([]);
  });

  it('shows a damage reduction step with its own "Ignorar a redução" toggle', () => {
    const cut = {
      kind: DamageStepKind.REDUCTION,
      sourceKeys: ['feat:couro-grosso@mesa'],
      labelPt: 'Redução de dano de arma (Couro grosso)',
      before: 8,
      after: 5,
      ignored: false,
    };
    const { fixture, el, plain } = setup({ master: true, amount: 5, steps: [cut] });
    expect(plain()).toContain('Redução de dano de arma (Couro grosso): 8 → 5');
    expect(el.querySelector('.step__toggle')!.textContent).toContain('Ignorar a redução');
    el.querySelector<HTMLInputElement>('.step__toggle input')!.click();
    fixture.detectChanges();
    expect(fixture.componentInstance.ignored()).toEqual(['feat:couro-grosso@mesa']);
  });

  it('shows a step already ignored in words, with no toggle', () => {
    const { el, plain } = setup({ master: true, amount: 10, steps: [{ ...fire, ignored: true }] });
    expect(plain()).toContain('(ignorada pelo mestre)');
    expect(el.querySelector('.step__toggle')).toBeNull();
  });
});

describe('DamageBreakdown with a single part', () => {
  it('shows no table and no list when the damage has one part and no step', () => {
    const { el } = setup({ master: true, rolls: [rolls[0]], amount: 12 });
    expect(el.querySelector('table')).toBeNull();
    expect(el.querySelector('thead')).toBeNull();
    const player = setup({ rolls: [rolls[0]], amount: 12 });
    expect(player.el.querySelector('ul')).toBeNull();
  });

  it('keeps the resistance steps of a single part damage, with no table', () => {
    const { el, plain } = setup({
      master: true,
      rolls: [rolls[0]],
      amount: 10,
      steps: [fire],
      amountAfterSteps: 5,
    });
    expect(el.querySelector('table')).toBeNull();
    expect(plain()).toContain('Resistência a fogo (tiefling)');
  });
});
