import { ComponentFixture, TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { type SceneAction, SceneActionSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import { SceneChecks } from '../../../core/maps/scene-actions';
import { SceneActions } from './scene-actions';
import { nextTurn } from '../../../../test-settle';

function action(
  id: string,
  checkName: string,
  extra: Partial<Omit<SceneAction, '$typeName'>> = {},
): SceneAction {
  return create(SceneActionSchema, { id, key: 'skill:x', checkName, ...extra });
}

const FIVE = [
  action('a1', 'Investigação', { name: 'Procurar pistas na carroça', dc: 12 }),
  action('a2', 'Sobrevivência', { name: 'Seguir os rastros dos goblins', dc: 13 }),
  action('a3', 'Adestrar Animais', { name: 'Acalmar os cavalos' }),
  action('a4', 'Percepção', { key: 'skill:perception' }),
  action('a5', 'Teste de resistência de Constituição', {
    name: 'Resistir ao cheiro de fumaça',
    dc: 10,
    key: 'save:con',
  }),
];

describe('SceneActions', () => {
  let api: FakeMapsClient;
  let fixture: ComponentFixture<SceneActions>;
  let el: HTMLElement;
  let emitted: (readonly SceneAction[])[];

  function setup(actions: SceneAction[]) {
    api = new FakeMapsClient();
    api.sceneActions = actions;
    emitted = [];
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        {
          provide: SceneChecks,
          useValue: {
            skills: () =>
              Promise.resolve([
                { key: 'skill:arcana', label: 'Arcanismo' },
                { key: 'skill:investigation', label: 'Investigação' },
              ]),
          },
        },
      ],
    });
    fixture = TestBed.createComponent(SceneActions);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('mapId', 'm1');
    fixture.componentRef.setInput('pointId', 'p1');
    fixture.componentRef.setInput('actions', actions);
    fixture.componentInstance.actionsChange.subscribe((list) => {
      emitted.push(list);
      // The panel puts the answer on the point, and the point comes back as the input.
      fixture.componentRef.setInput('actions', list);
    });
    fixture.detectChanges();
    el = fixture.nativeElement;
    // jsdom has no layout: the form scrolls itself into view.
    Element.prototype.scrollIntoView = vi.fn();
  }

  async function settle(): Promise<void> {
    for (let i = 0; i < 4; i++) {
      await fixture.whenStable();
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const rows = () => Array.from(el.querySelectorAll('.sa__row'));
  const control = (id: string, which: 'up' | 'down' | 'remove') =>
    el.querySelector<HTMLButtonElement>(`[data-action="${id}"][data-control="${which}"]`)!;
  const button = (name: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.includes(name),
    )!;
  const flat = (e: Element | null | undefined) =>
    e?.textContent
      ?.replace(/\u00a0/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
  /** Picks a check in the new-action form's select, as a person does. */
  function choose(key: string): void {
    const select = el.querySelector<HTMLSelectElement>('app-scene-action-form select')!;
    select.value = key;
    select.dispatchEvent(new Event('change'));
    fixture.detectChanges();
  }

  function type(input: HTMLInputElement, value: string): void {
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  }

  it('lists the actions in order: the name or the check, the check, and the DC only when there is one', () => {
    setup([...FIVE]);
    expect(flat(el.querySelector('.sa__count'))).toBe('5 de 20');
    expect(rows().map((r) => r.querySelector('.sa__name')?.textContent)).toEqual([
      'Procurar pistas na carroça',
      'Seguir os rastros dos goblins',
      'Acalmar os cavalos',
      'Percepção',
      'Resistir ao cheiro de fumaça',
    ]);
    expect(rows().map((r) => r.querySelector('.sa__check')?.textContent)).toEqual([
      'Investigação',
      'Sobrevivência',
      'Adestrar Animais',
      'Perícia',
      'Teste de resistência de Constituição',
    ]);
    expect(rows().map((r) => flat(r.querySelector('.sa__dc')) ?? null)).toEqual([
      'CD 12',
      'CD 13',
      null,
      null,
      'CD 10',
    ]);
  });

  it('names every control by its action, and quiets the first ↑ and the last ↓', () => {
    setup([...FIVE]);
    expect(control('a2', 'up').getAttribute('aria-label')).toBe(
      'Subir Seguir os rastros dos goblins',
    );
    expect(control('a2', 'down').getAttribute('aria-label')).toBe(
      'Descer Seguir os rastros dos goblins',
    );
    expect(control('a4', 'remove').getAttribute('aria-label')).toBe('Remover Percepção');
    expect(control('a1', 'up').getAttribute('aria-disabled')).toBe('true');
    expect(control('a5', 'down').getAttribute('aria-disabled')).toBe('true');
    expect(control('a2', 'up').getAttribute('aria-disabled')).toBe('false');
  });

  it('invites the first action when there is none', () => {
    setup([]);
    expect(el.textContent).toContain('Nenhuma ação ainda');
    expect(flat(el.querySelector('.sa__count'))).toBe('0 de 20');
    expect(el.querySelector('.sa__list')).toBeNull();
    expect(button('Adicionar ação')).toBeTruthy();
  });

  it('moves an action up, saves at once, and keeps focus on the same button of the moved row', async () => {
    setup([...FIVE]);
    control('a3', 'up').focus();
    control('a3', 'up').click();
    await settle();
    expect(api.calls).toContain('moveSceneAction p1 a3 up');
    expect(
      rows()
        .map((r) => r.querySelector('.sa__name')?.textContent)
        .slice(0, 3),
    ).toEqual([
      'Procurar pistas na carroça',
      'Acalmar os cavalos',
      'Seguir os rastros dos goblins',
    ]);
    expect(document.activeElement).toBe(control('a3', 'up'));
    expect(flat(el.querySelector('[role="status"]'))).toBe('Acalmar os cavalos: posição 2 de 5.');
    control('a3', 'down').click();
    await settle();
    expect(document.activeElement).toBe(control('a3', 'down'));
  });

  it('does nothing for the first ↑', async () => {
    setup([...FIVE]);
    control('a1', 'up').click();
    await settle();
    expect(api.calls).toEqual([]);
  });

  it('removes with no question and focuses the next row; the last one sends focus to "Adicionar ação"', async () => {
    setup([...FIVE]);
    control('a2', 'remove').click();
    await settle();
    expect(el.querySelector('[role="alertdialog"], [role="dialog"]')).toBeNull();
    expect(api.calls).toEqual(['removeSceneAction p1 a2']);
    expect(rows()).toHaveLength(4);
    expect(document.activeElement).toBe(control('a3', 'remove'));
    control('a5', 'remove').click();
    await settle();
    expect(document.activeElement).toBe(button('Adicionar ação'));
  });

  it('focuses "Adicionar ação" after removing the only action', async () => {
    setup([FIVE[0]]);
    control('a1', 'remove').click();
    await settle();
    expect(el.textContent).toContain('Nenhuma ação ainda');
    expect(document.activeElement).toBe(button('Adicionar ação'));
  });

  it('opens the form in place with focus on the first choice, and "Cancelar" gives focus back', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    expect(el.querySelector('form')).not.toBeNull();
    // The button is gone while the form is open (the form has its own "Adicionar ação").
    expect(el.querySelectorAll('.sa__add')).toHaveLength(0);
    expect(document.activeElement).toBe(el.querySelector('input[type="radio"]'));
    expect(Array.from(el.querySelectorAll('.sf__kind'), (l) => flat(l))).toEqual([
      'Perícia',
      'Teste de habilidade',
      'Teste de resistência',
    ]);
    // The buttons are under the fields, "Adicionar ação" outlined and "Cancelar" a text button.
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.sf__actions button'));
    expect(buttons.map((b) => flat(b))).toEqual(['Adicionar ação', 'Cancelar']);
    expect(buttons[0].classList).toContain('mat-mdc-outlined-button');
    expect(buttons[1].classList).toContain('mat-mdc-button');
    const lastField = el.querySelectorAll('mat-form-field')[2];
    expect(
      lastField.compareDocumentPosition(buttons[0]) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    button('Cancelar').click();
    await settle();
    expect(el.querySelector('form')).toBeNull();
    expect(document.activeElement).toBe(button('Adicionar ação'));
    expect(api.calls).toEqual([]);
  });

  it('changes the list under "O que rolar": skills, the six abilities, the six saves', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    const select = () => el.querySelector<HTMLSelectElement>('app-scene-action-form select')!;
    expect(Array.from(select().options, (o) => o.textContent?.trim())).toEqual([
      'Escolha…',
      'Arcanismo',
      'Investigação',
    ]);
    el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].click();
    fixture.detectChanges();
    expect(Array.from(select().options, (o) => o.textContent?.trim())).toEqual([
      'Escolha…',
      'Força',
      'Destreza',
      'Constituição',
      'Inteligência',
      'Sabedoria',
      'Carisma',
    ]);
    el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[2].click();
    fixture.detectChanges();
    expect(Array.from(select().options, (o) => o.value)[5]).toBe('save:wis');
  });

  it('adds an action with a name and a DC, saves at once and focuses "Adicionar ação"', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    const [name, dc] = Array.from(
      el.querySelectorAll<HTMLInputElement>('input[matInput], input.mat-mdc-input-element'),
    );
    type(name, 'Procurar mais pistas');
    expect(
      flat(el.querySelector('mat-form-field:nth-of-type(2) .mat-mdc-form-field-hint-wrapper')),
    ).toContain('20 de 60');
    type(dc, '14');
    choose('skill:arcana');
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(api.calls).toEqual([
      'addSceneAction p1 {"key":"skill:arcana","name":"Procurar mais pistas","dc":14}',
    ]);
    expect(emitted.at(-1)).toHaveLength(6);
    expect(el.querySelector('form')).toBeNull();
    expect(rows()).toHaveLength(6);
    expect(flat(el.querySelector('.sa__count'))).toBe('6 de 20');
    expect(document.activeElement).toBe(button('Adicionar ação'));
  });

  it('says a DC out of 1 to 30 under its field, with icon and words, and focuses the field', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    const dc = el.querySelectorAll<HTMLInputElement>('input.mat-mdc-input-element')[1];
    type(dc, '31');
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    const error = el.querySelector('mat-error');
    expect(flat(error)).toContain('A CD vai de 1 a 30. Digite outro número ou deixe em branco.');
    expect(error?.querySelector('mat-icon')?.textContent).toBe('error');
    expect(document.activeElement).toBe(dc);
    expect(api.calls).toEqual([]);
    // Typing again clears it.
    type(dc, '3');
    await settle();
    expect(el.querySelector('mat-error')).toBeNull();
  });

  it('leaves the DC empty as "no DC" (0)', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    choose('skill:arcana');
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(api.calls[0]).toContain('"dc":0');
  });

  it('preselects no check: the master must choose, and submitting without one says so and sends nothing', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    const select = el.querySelector<HTMLSelectElement>('app-scene-action-form select')!;
    expect(select.value).toBe('');
    expect(flat(el.querySelector('mat-form-field .mat-mdc-form-field-hint-wrapper'))).toContain(
      'Obrigatório.',
    );
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(flat(el.querySelector('form [role="alert"]'))).toContain('Escolha a perícia a rolar.');
    expect(document.activeElement).toBe(select);
    expect(api.calls).toEqual([]);
    choose('skill:arcana');
    expect(el.querySelector('form [role="alert"]')).toBeNull();
  });

  it('shows the free-text notice next to the name field', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    expect(el.querySelector('[role="note"]')?.textContent).toContain(
      'É ficção: não escreva dados reais de pessoas.',
    );
  });

  it('at 20 of 20 says the limit in words above a disabled button, tied to it', () => {
    setup(Array.from({ length: 20 }, (_, i) => action(`x${i}`, 'Arcanismo')));
    expect(flat(el.querySelector('.sa__count'))).toBe('20 de 20');
    expect(flat(el.querySelector('.sa__limit'))).toBe(
      'Limite de 20 ações. Remova uma para adicionar outra.',
    );
    const add = button('Adicionar ação');
    expect(add.getAttribute('aria-disabled')).toBe('true');
    expect(add.getAttribute('aria-describedby')).toBe('sa-limit');
    add.click();
    fixture.detectChanges();
    expect(el.querySelector('form')).toBeNull();
    // The sentence comes before the button.
    expect(
      el.querySelector('.sa__limit')!.compareDocumentPosition(add) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('takes the limit from the server too (`resource_exhausted`) and stays in the form', async () => {
    setup([...FIVE]);
    button('Adicionar ação').click();
    await settle();
    api.sceneActions = Array.from({ length: 20 }, (_, i) => action(`x${i}`, 'Arcanismo'));
    choose('skill:arcana');
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(el.querySelector('form [role="alert"]')?.textContent).toContain('Limite de 20 ações');
    expect(el.querySelector('form')).not.toBeNull();
  });

  it('answers nothing to a second tap while one write is in flight', async () => {
    setup([...FIVE]);
    control('a3', 'up').click();
    control('a4', 'up').click();
    await settle();
    expect(api.calls).toEqual(['moveSceneAction p1 a3 up']);
  });

  describe('editing an action', () => {
    const EDITABLE = [
      action('a1', 'Investigação', {
        key: 'skill:investigation',
        name: 'Procurar pistas na carroça',
        dc: 12,
      }),
      action('a2', 'Sobrevivência', { key: 'skill:survival', name: 'Seguir os rastros', dc: 13 }),
      action('a3', 'Teste de resistência de Constituição', { key: 'save:con', dc: 10 }),
    ];
    const pencil = (id: string) =>
      el.querySelector<HTMLButtonElement>(`[data-action="${id}"][data-control="edit"]`)!;
    const form = () => el.querySelector<HTMLFormElement>('form')!;
    const nameField = () => form().querySelectorAll<HTMLInputElement>('input[matInput]')[0];
    const dcField = () => form().querySelectorAll<HTMLInputElement>('input[matInput]')[1];
    const saveButton = () =>
      Array.from(form().querySelectorAll<HTMLButtonElement>('.sf__actions button'))[0];
    const open = async (id: string) => {
      pencil(id).click();
      await settle();
    };

    it('gives each row a pencil named by its action, between "Descer" and "Remover"', () => {
      setup([...EDITABLE]);
      const labels = Array.from(rows()[0].querySelectorAll('.sa__controls button'), (b) =>
        b.getAttribute('aria-label'),
      );
      expect(labels).toEqual([
        'Subir Procurar pistas na carroça',
        'Descer Procurar pistas na carroça',
        'Editar Procurar pistas na carroça',
        'Remover Procurar pistas na carroça',
      ]);
      expect(pencil('a1').textContent).toContain('edit');
    });

    it('opens in place of the row, filled with what the action has, with the focus on the first choice', async () => {
      setup([...EDITABLE]);
      await open('a1');
      expect(rows()).toHaveLength(3);
      expect(rows()[0].querySelector('form')).not.toBeNull();
      expect(rows()[0].textContent).not.toContain('Tentativas por jogador');
      expect(flat(form().querySelector('h4'))).toBe('Editar ação');
      expect(form().querySelector<HTMLInputElement>('input[type="radio"]:checked')?.value).toBe(
        'skill',
      );
      expect(form().querySelector<HTMLSelectElement>('select')?.value).toBe('skill:investigation');
      expect(nameField().value).toBe('Procurar pistas na carroça');
      expect(dcField().value).toBe('12');
      expect(flat(form())).toContain('26 de 60');
      expect(flat(form())).toContain('Em branco tira a CD da ação.');
      expect(document.activeElement).toBe(form().querySelector('input[type="radio"]'));
      expect(api.calls).toEqual([]);
    });

    it('deduces the kind and the list from the key: a saving throw opens as "Teste de resistência"', async () => {
      setup([...EDITABLE]);
      await open('a3');
      expect(form().querySelector<HTMLInputElement>('input[type="radio"]:checked')?.value).toBe(
        'save',
      );
      expect(form().querySelector<HTMLSelectElement>('select')?.value).toBe('save:con');
      expect(nameField().value).toBe('');
    });

    it('keeps "Salvar ação" dashed, with its reason in words, until a field changes, and sends nothing before', async () => {
      setup([...EDITABLE]);
      await open('a1');
      expect(flat(saveButton())).toBe('Salvar ação');
      expect(saveButton().getAttribute('aria-disabled')).toBe('true');
      expect(saveButton().classList).toContain('sf__off');
      const reason = form().querySelector('.sf__reason')!;
      expect(flat(reason)).toBe('blockMude um campo para salvar.');
      expect(saveButton().getAttribute('aria-describedby')).toBe(reason.id);
      saveButton().click();
      await settle();
      expect(api.calls).toEqual([]);
      type(nameField(), 'Procurar rastros na carroça');
      expect(saveButton().getAttribute('aria-disabled')).not.toBe('true');
      expect(form().querySelector('.sf__reason')).toBeNull();
      // Typing the old name back makes it quiet again.
      type(nameField(), 'Procurar pistas na carroça');
      expect(saveButton().getAttribute('aria-disabled')).toBe('true');
    });

    it('saves only the fields that changed, brings the row back with the new values and focuses its pencil', async () => {
      setup([...EDITABLE]);
      await open('a1');
      type(nameField(), 'Procurar rastros na carroça');
      type(dcField(), '14');
      saveButton().click();
      await settle();
      expect(api.calls).toEqual([
        'updateSceneAction p1 a1 {"name":"Procurar rastros na carroça","dc":14}',
      ]);
      expect(el.querySelector('form')).toBeNull();
      expect(flat(rows()[0])).toContain('Procurar rastros na carroça');
      expect(flat(rows()[0])).toContain('CD 14');
      expect(document.activeElement).toBe(pencil('a1'));
      expect(flat(el.querySelector('[role="status"]'))).toBe(
        'Ação salva: Procurar rastros na carroça.',
      );
    });

    it('takes the check to the first option of the new list, and sends its key', async () => {
      setup([...EDITABLE]);
      await open('a1');
      form()
        .querySelectorAll<HTMLInputElement>('input[type="radio"]')[1]
        .dispatchEvent(new Event('change'));
      await settle();
      expect(form().querySelector<HTMLSelectElement>('select')?.value).toBe('ability:str');
      saveButton().click();
      await settle();
      expect(api.calls).toEqual(['updateSceneAction p1 a1 {"key":"ability:str"}']);
    });

    it('a blank name takes the name off (empty) and a blank DC takes the DC off (0)', async () => {
      setup([...EDITABLE]);
      await open('a1');
      type(nameField(), '');
      type(dcField(), '');
      saveButton().click();
      await settle();
      expect(api.calls).toEqual(['updateSceneAction p1 a1 {"name":"","dc":0}']);
      expect(flat(rows()[0])).not.toContain('CD');
    });

    it('says a DC out of 1 to 30 under its field, marks it, focuses it and sends nothing', async () => {
      setup([...EDITABLE]);
      await open('a1');
      type(dcField(), '35');
      expect(saveButton().getAttribute('aria-disabled')).not.toBe('true');
      saveButton().click();
      await settle();
      expect(flat(form().querySelector('mat-error'))).toContain(
        'A CD vai de 1 a 30. Digite outro número ou deixe em branco.',
      );
      expect(document.activeElement).toBe(dcField());
      expect(flat(form())).not.toContain('Em branco tira a CD da ação.');
      expect(api.calls).toEqual([]);
    });

    it('keeps the form and what was typed when the server refuses, says why inside it, and can try again', async () => {
      setup([...EDITABLE]);
      await open('a1');
      type(nameField(), 'Outro nome');
      api.updateSceneActionError = new ConnectError('bad', Code.InvalidArgument);
      saveButton().click();
      await settle();
      expect(form().querySelector('[role="alert"]')?.textContent).toContain(
        'Não deu para salvar a ação: o nome vai até 60 caracteres e a CD de 1 a 30.',
      );
      expect(nameField().value).toBe('Outro nome');
      api.updateSceneActionError = null;
      saveButton().click();
      await settle();
      expect(el.querySelector('form')).toBeNull();
      expect(flat(rows()[0])).toContain('Outro nome');
    });

    it('says a lost point and a master-only call in their own words', async () => {
      setup([...EDITABLE]);
      await open('a1');
      type(nameField(), 'Outro nome');
      api.updateSceneActionError = new ConnectError('gone', Code.NotFound);
      saveButton().click();
      await settle();
      expect(form().querySelector('[role="alert"]')?.textContent).toContain(
        'Esse ponto não existe mais. Recarregue a página.',
      );
      api.updateSceneActionError = new ConnectError('no', Code.PermissionDenied);
      saveButton().click();
      await settle();
      expect(form().querySelector('[role="alert"]')?.textContent).toContain(
        'Só o mestre da campanha muda as ações da cena.',
      );
    });

    it('"Cancelar" closes without saving and gives the focus back to the pencil', async () => {
      setup([...EDITABLE]);
      await open('a1');
      type(nameField(), 'Outro nome');
      button('Cancelar').click();
      await settle();
      expect(el.querySelector('form')).toBeNull();
      expect(document.activeElement).toBe(pencil('a1'));
      expect(flat(rows()[0])).toContain('Procurar pistas na carroça');
      expect(api.calls).toEqual([]);
    });

    it('keeps one form at a time: another pencil, or "Adicionar ação", closes this one with nothing saved', async () => {
      setup([...EDITABLE]);
      await open('a1');
      await open('a2');
      expect(el.querySelectorAll('form')).toHaveLength(1);
      expect(rows()[1].querySelector('form')).not.toBeNull();
      expect(rows()[0].querySelector('form')).toBeNull();
      button('Adicionar ação').click();
      await settle();
      expect(el.querySelectorAll('form')).toHaveLength(1);
      expect(flat(form().querySelector('h4'))).toBe('Nova ação');
      await open('a1');
      expect(flat(form().querySelector('h4'))).toBe('Editar ação');
      expect(el.querySelectorAll('form')).toHaveLength(1);
      expect(api.calls).toEqual([]);
    });

    it('answers nothing to a pencil while a write is in flight', async () => {
      setup([...EDITABLE]);
      control('a2', 'up').click();
      pencil('a3').click();
      await settle();
      expect(el.querySelector('form')).toBeNull();
    });
  });

  describe('the DC switch and the attempts (E8-13, MR-015, RN-20)', () => {
    const select = (id: string) => el.querySelector<HTMLSelectElement>(`#sa-att-${id}`)!;
    const dcSwitch = () => el.querySelector<HTMLButtonElement>('.sa__dcswitch [role="switch"]')!;

    it('starts the list with "Mostrar a CD aos jogadores", off, saying only the master sees the DC', () => {
      setup([...FIVE]);
      expect(flat(el.querySelector('.sa__dcswitch .sw__label'))).toBe('Mostrar a CD aos jogadores');
      expect(dcSwitch().getAttribute('aria-checked')).toBe('false');
      expect(flat(el.querySelector('.sa__dcswitch .sw__hint'))).toBe(
        'Desligado: só você vê a CD. Os jogadores veem só o resultado.',
      );
      expect(el.querySelector('.sa__preview')).toBeNull();
      // The switch is above the first action.
      expect(
        dcSwitch().compareDocumentPosition(rows()[0]) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    });

    it('turns on with its own copy and shows how the player sees the DC, saving at once with show_dc alone', async () => {
      setup([...FIVE]);
      const saved: boolean[] = [];
      fixture.componentInstance.showDcSaved.subscribe((on) => {
        saved.push(on);
        fixture.componentRef.setInput('showDc', on);
      });
      dcSwitch().click();
      await settle();
      expect(api.calls).toEqual(['updatePoint m1 p1 {"showDc":true}']);
      expect(saved).toEqual([true]);
      expect(dcSwitch().getAttribute('aria-checked')).toBe('true');
      expect(flat(el.querySelector('.sa__dcswitch .sw__hint'))).toBe(
        'Ligado: cada jogador vê a CD na ação e, depois de rolar, se passou ou não.',
      );
      const samples = Array.from(el.querySelectorAll('.sa__sample'), (e) => [
        flat(e.querySelector('.mr-tag'))?.replace(/^(check|close)\s*/, ''),
        flat(e.querySelector(':scope > span:not(.mr-tag)')),
      ]);
      expect(samples).toEqual([
        ['CD 12', 'antes de rolar'],
        ['Passou · CD 12', 'depois, se passou'],
        ['Não passou · CD 12', 'ou se não passou'],
      ]);
    });

    it('leaves the switch where it was and says why when the save is refused', async () => {
      setup([...FIVE]);
      api.failWith = new ConnectError('gone', Code.NotFound);
      dcSwitch().click();
      await settle();
      expect(dcSwitch().getAttribute('aria-checked')).toBe('false');
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
    });

    it('gives every action a 44px select "Tentativas por jogador": 1 to 5 and "Sem limite", 1 by default', () => {
      setup([...FIVE.map((a) => ({ ...a, maxAttempts: 1 }))]);
      const first = select('a1');
      expect(Array.from(first.options, (o) => o.textContent?.trim())).toEqual([
        '1',
        '2',
        '3',
        '4',
        '5',
        'Sem limite',
      ]);
      expect(first.value).toBe('1');
      expect(flat(el.querySelector('label[for="sa-att-a1"]'))).toBe('Tentativas por jogador');
      expect(el.querySelectorAll('.sa__select select')).toHaveLength(5);
    });

    it('shows the saved limit, "Sem limite" included, with what it means', () => {
      setup([
        action('a1', 'Percepção', { maxAttempts: 3 }),
        action('a2', 'Adestrar Animais', { maxAttempts: 0 }),
      ]);
      expect(select('a1').value).toBe('3');
      expect(select('a2').value).toBe('0');
      expect(select('a2').selectedOptions[0].textContent?.trim()).toBe('Sem limite');
      expect(el.querySelectorAll('.sa__unlimited')).toHaveLength(1);
      expect(flat(el.querySelector('.sa__unlimited'))).toContain(
        'o jogador rola quantas vezes quiser e você vê cada rolagem',
      );
    });

    it('puts the select back on what the server has when the save is refused', async () => {
      setup([action('a1', 'Percepção', { maxAttempts: 1 })]);
      api.failWith = new ConnectError('gone', Code.NotFound);
      select('a1').value = '4';
      select('a1').dispatchEvent(new Event('change'));
      await settle();
      expect(select('a1').value).toBe('1');
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
    });

    it('saves a change at once and says it in the live region', async () => {
      setup([action('a1', 'Percepção', { maxAttempts: 1 })]);
      select('a1').value = '3';
      select('a1').dispatchEvent(new Event('change'));
      await settle();
      expect(api.calls).toEqual(['setSceneActionAttempts p1 a1 3']);
      expect(emitted.at(-1)?.[0].maxAttempts).toBe(3);
      expect(flat(el.querySelector('[role="status"]'))).toBe(
        'Percepção: 3 tentativas por jogador.',
      );
      select('a1').value = '0';
      select('a1').dispatchEvent(new Event('change'));
      await settle();
      expect(flat(el.querySelector('[role="status"]'))).toBe(
        'Percepção: sem limite de tentativas.',
      );
    });
  });
});
