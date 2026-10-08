import { signal } from '@angular/core';
import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { MapPointKind, MapPointSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { TrapFireSheet, type TrapFireData, fireLabel } from './trap-fire-sheet';

describe('fireLabel', () => {
  it('says who is caught, or the area when nobody is checked', () => {
    expect(fireLabel([], false)).toBe('Disparar para quem está na área');
    expect(fireLabel(['Toren'], false)).toBe('Disparar para Toren');
    expect(fireLabel(['A', 'B'], false)).toBe('Disparar para 2 personagens');
    expect(fireLabel([], true)).toBe('Incluir');
    expect(fireLabel(['A', 'B'], true)).toBe('Incluir 2 personagens');
  });
});

describe('TrapFireSheet', () => {
  function setup(extra: Partial<TrapFireData> = {}, firstCallFails = false) {
    const calls: unknown[][] = [];
    const api = {
      fire: async (...a: unknown[]) => {
        calls.push(a);
        if (firstCallFails && calls.length === 1) {
          throw new ConnectError('lost', Code.Unavailable);
        }
        return { firing: { caught: [] } };
      },
    };
    const close = vi.fn();
    const data: TrapFireData = {
      campaignId: 'c',
      mapId: 'm',
      point: create(MapPointSchema, {
        id: 'x',
        kind: MapPointKind.TRAP,
        name: 'Fosso escondido',
        description: 'No corredor',
      }),
      targets: signal([
        { id: 't', name: 'Toren', sub: 'Perto da armadilha' },
        { id: 'b', name: 'Brisa', sub: 'Longe da armadilha' },
      ]),
      extendFiringId: '',
      ...extra,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TrapsClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(TrapFireSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, calls, close };
  }

  it('fires for the area with nobody checked, and says the server knows who is there', async () => {
    const { fixture, el, calls } = setup();
    expect(el.textContent).toContain('Ninguém marcado. Dispara para quem estiver na área');
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await fixture.whenStable();
    expect(calls[0].slice(0, 4)).toEqual(['c', 'm', 'x', []]);
    expect(calls[0][5]).toBe('');
  });

  it('fires for the checked ones only', async () => {
    const { fixture, el, calls } = setup();
    el.querySelectorAll<HTMLInputElement>('input[type=checkbox]')[1].click();
    fixture.detectChanges();
    const go = el.querySelector<HTMLButtonElement>('.pf__go')!;
    expect(go.textContent).toContain('Disparar para Brisa');
    go.click();
    await fixture.whenStable();
    expect(calls[0][3]).toEqual(['b']);
  });

  it("adds creatures to a firing already made: someone must be checked, and the firing's ID goes with them", async () => {
    const { fixture, el, calls } = setup({ extendFiringId: 'f1' });
    expect(el.textContent).toContain('Pegar mais gente');
    expect(el.querySelector('.pf__off')?.getAttribute('aria-disabled')).toBe('true');
    el.querySelectorAll<HTMLInputElement>('input[type=checkbox]')[0].click();
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await fixture.whenStable();
    expect(calls[0][3]).toEqual(['t']);
    expect(calls[0][5]).toBe('f1');
  });

  it('says the read failed, not that nobody is on the map', () => {
    const { el } = setup({ targets: signal([]), targetsFailed: signal(true) });
    expect(el.textContent).toContain('Não deu para ler quem está no mapa');
    expect(el.textContent).not.toContain('Ninguém com token no mapa');
  });

  it('opens before the list is read, says so, and fills it in when it arrives (the master may be quicker than the read)', () => {
    const targets = signal<readonly { id: string; name: string; sub: string }[] | null>(null);
    const { fixture, el } = setup({ targets });
    expect(el.textContent).toContain('Lendo quem está no mapa');
    expect(el.textContent).not.toContain('Ninguém com token no mapa');
    targets.set([{ id: 't', name: 'Toren', sub: 'Perto da armadilha' }]);
    fixture.detectChanges();
    expect(el.textContent).toContain('Toren');
    expect(el.textContent).not.toContain('Lendo quem está no mapa');
  });

  const go = async (fixture: ComponentFixture<TrapFireSheet>, el: HTMLElement) => {
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const toggle = (fixture: ComponentFixture<TrapFireSheet>, el: HTMLElement, i: number) => {
    el.querySelectorAll<HTMLInputElement>('input[type=checkbox]')[i].click();
    fixture.detectChanges();
  };

  it('keeps the key of a request that is retried as it was', async () => {
    const { fixture, el, calls } = setup({}, true);
    toggle(fixture, el, 0);
    await go(fixture, el);
    await go(fixture, el);
    expect(calls).toHaveLength(2);
    expect(calls[1][3]).toEqual(['t']);
    expect(calls[1][4]).toBe(calls[0][4]);
  });

  it('takes a new key when the retry has other targets, so the server does not answer with the first request', async () => {
    const { fixture, el, calls } = setup({}, true);
    toggle(fixture, el, 0);
    await go(fixture, el);
    toggle(fixture, el, 0);
    toggle(fixture, el, 1);
    await go(fixture, el);
    expect(calls[0][3]).toEqual(['t']);
    expect(calls[1][3]).toEqual(['b']);
    expect(calls[1][4]).not.toBe(calls[0][4]);
  });

  describe('when someone ticked leaves the list', () => {
    function withToren() {
      const targets = signal([
        { id: 't', name: 'Toren', sub: 'Perto' },
        { id: 'b', name: 'Brisa', sub: 'Longe' },
      ]);
      const s = setup({ targets });
      toggle(s.fixture, s.el, 0);
      return { ...s, targets };
    }

    it('fires for the ticked person while they are listed', async () => {
      const { fixture, el, calls } = withToren();
      await go(fixture, el);
      expect(calls[0][3]).toEqual(['t']);
    });

    it('does not fire for the whole area: it names who left and waits for a new pick', async () => {
      const { fixture, el, calls, targets } = withToren();
      targets.set([{ id: 'b', name: 'Brisa', sub: 'Longe' }]);
      fixture.detectChanges();
      expect(el.textContent).toContain('Toren saiu da lista');
      expect(el.querySelector('.pf__go')).toBeNull();
      el.querySelector<HTMLButtonElement>('.pf__off')!.click();
      await fixture.whenStable();
      expect(calls).toEqual([]);

      toggle(fixture, el, 0);
      expect(el.textContent).not.toContain('saiu da lista');
      await go(fixture, el);
      expect(calls[0][3]).toEqual(['b']);
    });
  });
});
