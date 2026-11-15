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

  it('does not say "= 0" for the part of a typed group that carries no sum of its own', () => {
    const typed = [
      {
        partKey: 'weapon',
        labelPt: 'Espada longa',
        diceCount: 2,
        diceSides: 6,
        faces: [],
        flat: 3,
        sum: 6,
        counted: true,
        damageTypeKey: 'piercing',
        rerolled: [],
      },
      {
        partKey: 'edge',
        labelPt: 'Gume Afiado',
        diceCount: 1,
        diceSides: 8,
        faces: [],
        flat: 0,
        sum: 0,
        counted: true,
        damageTypeKey: 'piercing',
        rerolled: [],
      },
    ] as never[];
    const { plain } = setup({ amount: 9, rolls: typed });
    expect(plain()).toContain('Espada longa 2d6 · dado físico + 3 = 9');
    expect(plain()).toContain('Gume Afiado 1d8 · somado em Espada longa');
    expect(plain()).not.toContain('= 0');
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
  it('is a list by part with "Tirar" only on the extras that count', () => {
    const { el } = setup({
      master: true,
      amount: 21,
      removable: new Set(['sneak-attack', 'divine-smite-extra']),
    });
    expect(el.querySelector('table')).toBeNull();
    expect(el.querySelectorAll('ul.rows > li')).toHaveLength(3);
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

describe('DamageBreakdown take-out of a part of a typed group', () => {
  const group = [
    {
      partKey: 'weapon',
      labelPt: 'Espada curta',
      diceCount: 1,
      diceSides: 6,
      faces: [],
      flat: 3,
      sum: 10,
      counted: true,
      damageTypeKey: 'damage-type:piercing',
      rerolled: [],
    },
    {
      partKey: 'sneak-attack',
      labelPt: 'Ataque Furtivo',
      diceCount: 3,
      diceSides: 6,
      faces: [],
      flat: 0,
      sum: 0,
      counted: true,
      damageTypeKey: 'damage-type:piercing',
      rerolled: [],
    },
  ] as never[];
  const plan = {
    group: true,
    title: 'Tirar o Ataque Furtivo do dano?',
    lines: [
      'Desfaz os 19 de dano: o Zumbi 1 volta a 22 PV.',
      'Pede a Jabuti só a soma de perfurante (1d6).',
    ],
    notice: '',
    confirm: 'Tirar e pedir a soma',
  };
  function open(extra: Record<string, unknown> = {}) {
    const fixture = TestBed.createComponent(DamageBreakdown);
    fixture.componentRef.setInput('rolls', group);
    fixture.componentRef.setInput('master', true);
    fixture.componentRef.setInput('amount', 13);
    fixture.componentRef.setInput('removable', new Set(['sneak-attack']));
    fixture.componentRef.setInput('typeNames', new Map([['damage-type:piercing', 'perfurante']]));
    fixture.componentRef.setInput('plans', new Map([['sneak-attack', plan]]));
    for (const [k, v] of Object.entries(extra)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    (el.querySelector('.take') as HTMLButtonElement).click();
    fixture.detectChanges();
    return { fixture, el, text: () => el.textContent!.replace(/\s+/g, ' ') };
  }

  it('heads the group and offers "Tirar" on the extra inside it', () => {
    const { el, text } = open();
    expect(text()).toContain('Perfurante');
    expect(text()).toContain('4d6 (10) + 3 · uma soma digitada');
    expect(el.querySelectorAll('.take')).toHaveLength(1);
  });

  it('says "automático" on the parts the app adds, and marks the row being asked', () => {
    const { el, fixture } = open({ autoKeys: new Set(['sneak-attack']) });
    const rows = Array.from(el.querySelectorAll('ul.rows > li'));
    expect(rows.filter((r) => r.textContent!.includes('automático'))).toHaveLength(1);
    expect(rows.filter((r) => r.classList.contains('row--asked'))).toHaveLength(1);
    expect(rows.find((r) => r.classList.contains('row--asked'))!.textContent).toContain(
      'Ataque Furtivo',
    );
    expect(el.querySelectorAll('.row--child')).toHaveLength(2);
    expect(fixture.nativeElement.querySelector('.row--group')).not.toBeNull();
  });

  it('lists what taking it out undoes and names the button for the new sum', () => {
    const { el, text } = open();
    expect(text()).toContain('Tirar o Ataque Furtivo do dano?');
    expect(
      Array.from(el.querySelectorAll('.ask__list li')).map((l) =>
        l.textContent!.replace(/^(undo|casino)/, '').trim(),
      ),
    ).toEqual(plan.lines);
    expect(el.querySelector('.ask__danger')!.textContent).toContain('Tirar e pedir a soma');
  });

  it('ties the missing-reason message to the field with the hint, and tells the page when the master goes back', () => {
    const { fixture, el } = open();
    let closed = 0;
    fixture.componentInstance.closed.subscribe(() => closed++);
    const field = el.querySelector<HTMLInputElement>('.ask__field')!;
    expect(field.getAttribute('aria-describedby')!.split(' ')).toHaveLength(1);
    field.value = 'x';
    field.dispatchEvent(new Event('input'));
    field.value = '';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    const ids = field.getAttribute('aria-describedby')!.split(' ');
    expect(ids).toHaveLength(2);
    expect(el.querySelector('.ask__error')!.id).toBe(ids[0]);
    expect(el.querySelector('.ask__hint')!.id).toBe(ids[1]);
    el.querySelector<HTMLButtonElement>('.ask__back')!.click();
    expect(closed).toBe(1);
  });

  it('gives the focus back to "Tirar" when the master goes back', async () => {
    const { fixture, el } = open();
    const back = el.querySelector<HTMLButtonElement>('.ask__back')!;
    back.click();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(el.querySelector('.ask')).toBeNull();
    expect(document.activeElement).toBe(el.querySelector('.take'));
  });

  it('says the reason is missing once it was left empty, with the confirm button dashed', () => {
    const { fixture, el, text } = open();
    const field = el.querySelector<HTMLInputElement>('.ask__field')!;
    field.value = 'x';
    field.dispatchEvent(new Event('input'));
    field.value = '';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(text()).toContain('Escreva o motivo, com até 120 caracteres.');
    expect(el.querySelector('.ask__danger')!.classList).toContain('ask__off');
  });

  it('keeps the panel open while sending, both buttons off, and shows what the server refused', () => {
    const { fixture, el, text } = open({ sending: false });
    fixture.componentRef.setInput('sending', true);
    fixture.detectChanges();
    expect(text()).toContain('Tirando…');
    expect(el.querySelector('.ask__back')!.classList).toContain('ask__off');
    fixture.componentRef.setInput(
      'error',
      'Não deu para tirar a parte agora. Nada mudou. Tente de novo.',
    );
    fixture.componentRef.setInput('sending', false);
    fixture.detectChanges();
    expect(el.querySelector('.ask__banner')!.textContent).toContain('Nada mudou');
    expect(el.querySelector('.ask')).not.toBeNull();
  });
});
