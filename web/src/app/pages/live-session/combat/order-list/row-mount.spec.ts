import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { mountMarkView } from '../../../../core/combat/mount-mark';
import { RowMount } from './row-mount';

describe('RowMount, the master\'s "Montaria" select of a row (#588, 588-08)', () => {
  const text = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();
  const tor = combatant({ id: 'tor', label: 'Tor', kind: CombatantKind.PLAYER, mountedOnId: 'h' });
  const horse = (over: object) =>
    combatant({ id: 'h', label: 'Cavalo de guerra', ...over } as never);

  async function setup(over: object, busy = false) {
    const fixture = TestBed.createComponent(RowMount);
    const view = mountMarkView(encounter({ combatants: [tor, horse(over)] }), 'h')!;
    fixture.componentRef.setInput('view', view);
    fixture.componentRef.setInput('label', 'Cavalo de guerra');
    fixture.componentRef.setInput('busy', busy);
    const picked: string[] = [];
    let closed = 0;
    fixture.componentInstance.pick.subscribe((k) => picked.push(k));
    fixture.componentInstance.closed.subscribe(() => closed++);
    fixture.detectChanges();
    // The select takes its value in a microtask.
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const open = () => {
      el.querySelector<HTMLElement>('.mat-mdc-select-trigger')!.click();
      fixture.detectChanges();
      return Array.from(document.querySelectorAll<HTMLElement>('mat-option'));
    };
    return { fixture, el, open, picked, closed: () => closed };
  }

  afterEach(() => document.querySelectorAll('.cdk-overlay-container').forEach((n) => n.remove()));

  it('shows the current mark in a select named for the combatant, with what it means under it', async () => {
    const { el } = await setup({ mountKind: 'controlled', riderId: 'tor' });
    expect(text(el.querySelector('mat-label'))).toBe('Montaria');
    const select = el.querySelector('mat-select')!;
    expect(select.getAttribute('aria-label')).toBe('Montaria: Cavalo de guerra');
    expect(text(select.querySelector('.mat-mdc-select-value'))).toBe('Adestrada');
    // The saddle before the word is decoration: the word says it.
    expect(select.querySelector('.mat-mdc-select-value app-saddle-icon')).not.toBeNull();
    expect(text(el.querySelector('.mount__hint'))).toBe(
      'Age logo depois de Tor e só usa Disparada, Desengajar e Esquivar.',
    );
  });

  it('lists the three marks, each with its line; the current one is the only one picked', async () => {
    const { open } = await setup({ mountKind: 'independent' });
    const options = open();
    expect(options.map((o) => text(o))).toEqual([
      'Nenhuma: não aceita cavaleiro.',
      'Adestrada: age logo depois de quem monta e só usa Disparada, Desengajar e Esquivar.',
      'Independente: mantém a própria vez e leva quem monta.',
    ]);
    expect(options.map((o) => o.getAttribute('aria-selected'))).toEqual(['false', 'false', 'true']);
  });

  it('sends the mark that was picked', async () => {
    const { open, picked } = await setup({});
    open()[1].click();
    expect(picked).toEqual(['controlled']);
  });

  it('keeps "Nenhuma" off, with the reason in words, while the mount carries its rider', async () => {
    const { el, open, picked } = await setup({ mountKind: 'controlled', riderId: 'tor' });
    expect(text(el.querySelector('.mount__why'))).toBe('Nenhuma: só depois que Tor desmontar.');
    const none = open()[0];
    expect(none.getAttribute('aria-disabled')).toBe('true');
    none.click();
    expect(picked).toEqual([]);
  });

  it('has no reason line for a mount that carries nobody, and "Fechar" closes the select', async () => {
    const { el, closed } = await setup({ mountKind: 'controlled' });
    expect(el.querySelector('.mount__why')).toBeNull();
    const close = Array.from(el.querySelectorAll('button')).find((b) => text(b) === 'Fechar')!;
    close.click();
    expect(closed()).toBe(1);
  });

  it('is off while a call is in the air', async () => {
    const { el } = await setup({ mountKind: 'controlled' }, true);
    expect(el.querySelector('mat-select')?.getAttribute('aria-disabled')).toBe('true');
  });
});
