import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { CreaturesClient } from '../../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  raven,
  summary,
  flat,
  isOff,
} from '../../../core/creatures/creatures-testing';
import { GiveCreatureSheet, type GiveCreatureData } from './give-creature-sheet';

describe('GiveCreatureSheet: the master gives a creature (E9-10, MR-037)', () => {
  let api: FakeCreaturesClient;
  let close: ReturnType<typeof vi.fn>;

  async function setup() {
    api = new FakeCreaturesClient();
    api.catalog = [
      summary('monster:mastiff', 'Mastim', {
        name: 'Mastiff',
        sizePt: 'Médio',
        challengeRating: '1/8',
      }),
      summary('monster:sea-horse', 'Cavalo-marinho', { name: 'Sea Horse' }),
      summary('monster:goblin', 'Goblin', {
        name: 'Goblin',
        type: 'humanoid',
        typePt: 'humanoide',
        challengeRating: '1/4',
      }),
    ];
    api.blocks.set('monster:mastiff', raven({ hitPoints: 5 }));
    close = vi.fn();
    const data: GiveCreatureData = {
      campaignId: 'camp-1',
      characterId: 'char-1',
      characterName: 'Toren',
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: CreaturesClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(GiveCreatureSheet);
    const settle = async (ms = 0) => {
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await new Promise((r) => setTimeout(r, ms));
      }
      fixture.detectChanges();
    };
    fixture.detectChanges();
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        flat(b)?.includes(name),
      )!;
    return { fixture, el, flat, button, settle };
  }

  it('opens with the whole book, the title naming the character, and three fields of the same height in a row', async () => {
    const { el, flat } = await setup();
    expect(flat(el.querySelector('.frame__title'))).toBe('Dar uma criatura a Toren');
    expect(flat(el.querySelector('.frame__sub'))).toBe(
      'Escolha uma criatura do livro de regras (SRD) para Toren.',
    );
    expect(el.querySelectorAll('.filters .box')).toHaveLength(3);
    expect(flat(el.querySelector('.list__n'))).toBe('3 de 3 · em ordem de nome');
    expect(flat(el.querySelectorAll('.row')[0])).toContain('Mastim (Mastiff)');
  });

  it('filters by name while typing, by type and by challenge rating, asking the server each time', async () => {
    const { el, settle } = await setup();
    const q = el.querySelector<HTMLInputElement>('input[type=search]')!;
    q.value = 'ma';
    q.dispatchEvent(new Event('input'));
    await settle(300);
    expect(api.searches.at(-1)).toMatchObject({ query: 'ma' });
    const type = el.querySelector<HTMLSelectElement>('select[name=type]')!;
    type.value = 'beast';
    type.dispatchEvent(new Event('change'));
    const cr = el.querySelector<HTMLSelectElement>('select[name=cr]')!;
    cr.value = '1/8';
    cr.dispatchEvent(new Event('change'));
    await settle(300);
    expect(api.searches.at(-1)).toMatchObject({ query: 'ma', type: 'beast', maxCr: '1/8' });
  });

  it('picking a creature names it with the book name, reads its hit points and the button says what happens', async () => {
    const { el, flat, button, fixture, settle } = await setup();
    expect(isOff(button('Dar a criatura'))).toBe(true);
    expect(flat(el.querySelector('.why'))).toBe('Escolha uma criatura da lista.');
    expect(button('Dar a criatura').classList).toContain('mr-button--off');
    const r = el.querySelectorAll<HTMLInputElement>('input[type=radio]')[0];
    r.checked = true;
    r.dispatchEvent(new Event('change'));
    await settle();
    expect(el.querySelector<HTMLInputElement>('input[name=name]')!.value).toBe('Mastim');
    expect(flat(el.querySelector('.note'))).toBe(
      'Vai para a ficha de Toren e entra nos combates com os PV do livro (5). Você corrige PV e condições depois.',
    );
    // The chosen row says its armor class and hit points, read from its stat block.
    expect(flat(el.querySelector('.row--on .row__sub'))).toBe(
      'Médio · fera · ND 1/8 · CA 12 · PV 5',
    );
    expect(isOff(button('Dar Mastim a Toren'))).toBe(false);
    expect(fixture.nativeElement.querySelector('mat-hint').textContent).toContain('6 de 40');
  });

  it('gives it with the name the master typed and closes with it', async () => {
    const { el, button, settle } = await setup();
    const r = el.querySelectorAll<HTMLInputElement>('input[type=radio]')[0];
    r.checked = true;
    r.dispatchEvent(new Event('change'));
    await settle();
    const name = el.querySelector<HTMLInputElement>('input[name=name]')!;
    name.value = 'Brutus';
    name.dispatchEvent(new Event('input'));
    await settle();
    button('Dar Mastim a Toren').click();
    await settle();
    expect(api.give).toHaveBeenCalledWith(
      'camp-1',
      'char-1',
      'monster:mastiff',
      'Brutus',
      expect.any(String),
    );
    expect(close).toHaveBeenCalledWith({ name: 'Brutus' });
  });

  it('a refusal (40 creatures) stays in the dialog in words', async () => {
    const { el, flat, button, settle } = await setup();
    const r = el.querySelectorAll<HTMLInputElement>('input[type=radio]')[0];
    r.checked = true;
    r.dispatchEvent(new Event('change'));
    await settle();
    api.give.mockRejectedValueOnce(new ConnectError('x', Code.PermissionDenied));
    button('Dar Mastim a Toren').click();
    await settle();
    expect(close).not.toHaveBeenCalled();
    expect(flat(el.querySelector('[role=alert]'))).toBe('Você não pode fazer isso.');
  });

  it('a typing pause still pending when the dialog is destroyed never searches', async () => {
    const { el, fixture, settle } = await setup();
    const before = api.searches.length;
    // A clock the spec drives by hand, turned on after the dialog settled: the typing pause is crossed by advancing it.
    vi.useFakeTimers();
    const q = el.querySelector<HTMLInputElement>('input[type=search]')!;
    q.value = 'lobo';
    q.dispatchEvent(new Event('input'));
    expect(vi.getTimerCount()).toBeGreaterThan(0);
    fixture.destroy();
    await vi.advanceTimersByTimeAsync(400);
    expect(api.searches.length).toBe(before);
    void settle;
  });

  it('Cancelar closes giving nothing', async () => {
    const { button } = await setup();
    button('Cancelar').click();
    expect(close).toHaveBeenCalledWith(undefined);
    expect(api.give).not.toHaveBeenCalled();
  });
});
