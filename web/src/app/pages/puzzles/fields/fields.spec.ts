import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import {
  GetMapLayersResponseSchema,
  GetMapResponseSchema,
  MapSchema,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { type SolveDraft, NO_SOLVE } from '../../../core/puzzles/puzzle-draft';
import { MapsClient } from '../../../core/maps/maps-client';
import { HintsField } from './hints-field';
import { type PickOption, PickGroup } from './pick-group';
import { SolveField } from './solve-field';
import { Stepper } from './stepper';
import { nextTurn } from '../../../../test-settle';

const settle = async (fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) => {
  for (let i = 0; i < 4; i++) {
    fixture.detectChanges();
    await fixture.whenStable();
    await nextTurn();
  }
  fixture.detectChanges();
};

@Component({
  imports: [PickGroup],
  template: `<app-pick-group legend="Tamanho do painel" [layout]="layout" [options]="options" [value]="value" (valueChange)="value = $event" />`,
})
class PickHost {
  layout: 'cards' | 'rows' | 'segments' = 'segments';
  options: PickOption<number>[] = [3, 4, 5].map((n) => ({
    value: n,
    title: String(n),
    sub: `${n} de lado`,
    icon: 'lightbulb',
  }));
  value = 5;
}

describe('PickGroup', () => {
  it('is a group of native radios named by its legend, with the marked one checked', async () => {
    const fixture = TestBed.createComponent(PickHost);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('legend')?.textContent?.trim()).toBe('Tamanho do painel');
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
    expect(radios.map((r) => r.checked)).toEqual([false, false, true]);
    expect(new Set(radios.map((r) => r.name)).size).toBe(1);
  });

  it('puts a check on the marked segment, so the state is not the color alone', async () => {
    const fixture = TestBed.createComponent(PickHost);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('.opt__check')).toHaveLength(1);
    expect(el.querySelector('.opt--on')?.textContent).toContain('5');
  });

  it('emits the value that was picked', async () => {
    const fixture = TestBed.createComponent(PickHost);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    const first = el.querySelector<HTMLInputElement>('input[value="3"]')!;
    first.click();
    expect(fixture.componentInstance.value).toBe(3);
  });

  it('draws a card with its icon and its line, and a row with its round radio', async () => {
    const cards = TestBed.createComponent(PickHost);
    cards.componentInstance.layout = 'cards';
    await settle(cards);
    const el = cards.nativeElement as HTMLElement;
    expect(el.querySelectorAll('.opt__icon')).toHaveLength(3);
    expect(el.querySelector('.opt__sub')?.textContent).toBe('3 de lado');
    const rows = TestBed.createComponent(PickHost);
    rows.componentInstance.layout = 'rows';
    await settle(rows);
    expect((rows.nativeElement as HTMLElement).querySelectorAll('.opt__dot')).toHaveLength(3);
  });
});

@Component({
  imports: [Stepper],
  template: `<app-stepper label="Rodas" noun="roda" [value]="value" [min]="2" [max]="6" (valueChange)="value = $event" />`,
})
class StepHost {
  value = 4;
}

describe('Stepper', () => {
  it('steps by one and says its limits beside the number', async () => {
    const fixture = TestBed.createComponent(StepHost);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('De 2 a 6');
    (el.querySelector('[aria-label="Mais roda"]') as HTMLElement).click();
    expect(fixture.componentInstance.value).toBe(5);
    fixture.detectChanges();
    (el.querySelector('[aria-label="Menos roda"]') as HTMLElement).click();
    expect(fixture.componentInstance.value).toBe(4);
  });

  it('disables the button at a limit', async () => {
    const fixture = TestBed.createComponent(StepHost);
    fixture.componentInstance.value = 6;
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('[aria-label="Mais roda"]')?.getAttribute('aria-disabled')).toBe(
      'true',
    );
    expect(el.querySelector('[aria-label="Menos roda"]')?.getAttribute('aria-disabled')).toBeNull();
    // It stays focusable (a button that turns itself off never drops the focus), and does nothing.
    (el.querySelector('[aria-label="Mais roda"]') as HTMLElement).click();
    expect(fixture.componentInstance.value).toBe(6);
  });
});

@Component({
  imports: [HintsField],
  template: `<app-hints-field [hints]="hints" [errors]="errors" (hintsChange)="hints = $event" />`,
})
class HintsHost {
  hints: string[] = [];
  errors: Record<number, string> = {};
}

describe('HintsField (E10-06 state 2)', () => {
  const buttonNamed = (el: HTMLElement, name: string) =>
    Array.from(el.querySelectorAll('button')).find(
      (b) => b.textContent?.includes(name) || b.getAttribute('aria-label') === name,
    ) as HTMLButtonElement;

  it('invites the first hint when there is none', async () => {
    const fixture = TestBed.createComponent(HintsHost);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Nenhuma dica ainda.');
    expect(el.textContent).toContain('Você solta uma dica de cada vez');
  });

  it('adds a row and puts the focus on it', async () => {
    const fixture = TestBed.createComponent(HintsHost);
    document.body.append(fixture.nativeElement);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    buttonNamed(el, 'Adicionar uma dica').click();
    await settle(fixture);
    expect(fixture.componentInstance.hints).toEqual(['']);
    expect(document.activeElement).toBe(el.querySelector('input'));
    el.remove();
  });

  it("edits a row and removes one, naming the button by the hint's number", async () => {
    const fixture = TestBed.createComponent(HintsHost);
    fixture.componentInstance.hints = [
      'A luz do selo responde ao toque.',
      'Cada toque troca cinco luzes de uma vez.',
    ];
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    const inputs = el.querySelectorAll('input');
    inputs[1].value = 'Outra.';
    inputs[1].dispatchEvent(new Event('input'));
    expect(fixture.componentInstance.hints).toEqual(['A luz do selo responde ao toque.', 'Outra.']);
    fixture.detectChanges();
    buttonNamed(el, 'Remover a dica 1').click();
    expect(fixture.componentInstance.hints).toEqual(['Outra.']);
  });

  it('stops at ten hints, and says so', async () => {
    const fixture = TestBed.createComponent(HintsHost);
    fixture.componentInstance.hints = Array.from({ length: 10 }, (_, i) => `dica ${i}`);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(buttonNamed(el, 'Adicionar uma dica').getAttribute('aria-disabled')).toBe('true');
    expect(el.textContent).toContain('Máximo de 10 dicas.');
  });

  it('shows what is wrong with a row under it, and marks the field', async () => {
    const fixture = TestBed.createComponent(HintsHost);
    fixture.componentInstance.hints = ['ok', ''];
    fixture.componentInstance.errors = { 1: 'Escreva a dica, ou tire esta.' };
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('[role="alert"]')?.textContent).toBe('Escreva a dica, ou tire esta.');
    expect(el.querySelectorAll('input')[1].getAttribute('aria-invalid')).toBe('true');
  });
});

@Component({
  imports: [SolveField],
  template: `<app-solve-field campaignId="camp-1" [solve]="solve" [error]="error" (solveChange)="solve = $event" />`,
})
class SolveHost {
  solve: SolveDraft = NO_SOLVE;
  error = '';
}

describe('SolveField ("Ao resolver", E10-06 state 2)', () => {
  const doors = new Uint8Array(2);
  doors[0] |= 2 << 4; // square 1 of a 2 x 2 map: a closed door
  const api = {
    list: async () => [create(MapSchema, { id: 'm1', name: 'A capela' })],
    get: async () => create(GetMapResponseSchema, { points: [] }),
    layers: async () =>
      create(GetMapLayersResponseSchema, {
        gridColumns: 2,
        gridRows: 2,
        wall: new Uint8Array(1),
        difficultTerrain: new Uint8Array(1),
        cover: new Uint8Array(1),
        doors,
      }),
  };

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [{ provide: MapsClient, useValue: api }] });
  });

  it('offers the four actions, "Só me avisar" first and marked, and says the master is always told', async () => {
    const fixture = TestBed.createComponent(SolveHost);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    const options = Array.from(el.querySelectorAll('.opt__title')).map((t) =>
      t.textContent?.trim(),
    );
    expect(options).toEqual([
      'Só me avisar',
      'Abrir uma porta',
      'Revelar um ponto do mapa',
      'Revelar uma pista',
    ]);
    expect(el.querySelector<HTMLInputElement>('input[type="radio"]')?.checked).toBe(true);
    expect(el.textContent).toContain('O mestre é sempre avisado. Resolver não rola dado nenhum.');
    expect(el.querySelector('mat-form-field select')).toBeNull();
  });

  it('asks for a map and then a door of it, named by kind and place', async () => {
    const fixture = TestBed.createComponent(SolveHost);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    el.querySelector<HTMLInputElement>('input[value="door"]')!.click();
    await settle(fixture);
    expect(fixture.componentInstance.solve.choice).toBe('door');
    const map = el.querySelector<HTMLSelectElement>('select')!;
    map.value = 'm1';
    map.dispatchEvent(new Event('change'));
    await settle(fixture);
    expect(fixture.componentInstance.solve.mapId).toBe('m1');
    const door = el.querySelectorAll<HTMLSelectElement>('select')[1];
    expect(Array.from(door.options).map((o) => o.textContent?.trim())).toEqual([
      'Escolha uma porta',
      'Fechada · (2, 1)',
    ]);
    door.value = '1,0';
    door.dispatchEvent(new Event('change'));
    expect(fixture.componentInstance.solve).toMatchObject({
      choice: 'door',
      mapId: 'm1',
      col: 1,
      row: 0,
    });
  });

  it('keeps the message when the action changes, and counts it', async () => {
    const fixture = TestBed.createComponent(SolveHost);
    fixture.componentInstance.solve = { ...NO_SOLVE, message: 'A porta se abriu.' };
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent?.replace(/\u00a0/g, ' ')).toContain('17 de 200');
    el.querySelector<HTMLInputElement>('input[value="clue"]')!.click();
    await settle(fixture);
    expect(fixture.componentInstance.solve).toMatchObject({
      choice: 'clue',
      message: 'A porta se abriu.',
    });
  });

  it('says there is nothing to pick when no scene has a clue, and shows what is wrong with the target', async () => {
    const fixture = TestBed.createComponent(SolveHost);
    fixture.componentInstance.solve = { ...NO_SOLVE, choice: 'clue' };
    fixture.componentInstance.error = 'Escolha a pista que vai a quem resolver.';
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Nenhuma cena da campanha tem pistas ainda.');
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Escolha a pista');
  });
});
