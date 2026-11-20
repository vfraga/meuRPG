import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { mountMarkView } from '../../../../core/combat/mount-mark';
import { RowMount } from './row-mount';

describe('RowMount, the master\'s "Montaria" select of a row', () => {
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
    fixture.componentInstance.pick.subscribe((k) => picked.push(k));
    document.body.appendChild(fixture.nativeElement);
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
    return { fixture, el, open, picked };
  }

  afterEach(() => document.body.replaceChildren());

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
    // two lines an option, the name in bold and its line under it.
    expect(options.map((o) => text(o.querySelector('.opt__t')))).toEqual([
      'Nenhuma',
      'Adestrada',
      'Independente',
    ]);
    expect(options.map((o) => text(o.querySelector('.opt__d')))).toEqual([
      'Não aceita cavaleiro.',
      'Age logo depois de quem monta e só usa Disparada, Desengajar e Esquivar.',
      'Mantém a própria vez e leva quem monta.',
    ]);
    expect(getComputedStyle(options[1].querySelector('.opt__d')!).display).toBe('block');
    expect(options.map((o) => o.getAttribute('aria-selected'))).toEqual(['false', 'false', 'true']);
    // The check is on the left, before the name, and only on the picked one.
    expect(options.map((o) => text(o.querySelector('.opt__check')))).toEqual(['', '', 'check']);
    const check = options[2].querySelector('.opt__check')!;
    expect(check.compareDocumentPosition(options[2].querySelector('.opt__body')!)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  });

  it('sends the mark that was picked', async () => {
    const { open, picked } = await setup({});
    open()[1].click();
    expect(picked).toEqual(['controlled']);
  });

  it('keeps "Nenhuma" off, with the reason in words, while the mount carries its rider', async () => {
    const { el, open, picked } = await setup({ mountKind: 'controlled', riderId: 'tor' });
    expect(text(el.querySelector('.mount__why'))).toBe('Só depois que Tor desmontar.');
    const none = open()[0];
    expect(none.getAttribute('aria-disabled')).toBe('true');
    none.click();
    expect(picked).toEqual([]);
  });

  it('has no reason line for a mount that carries nobody, and no button of its own: "Montaria…" closes it', async () => {
    const { el } = await setup({ mountKind: 'controlled' });
    expect(el.querySelector('.mount__why')).toBeNull();
    expect(el.querySelector('button')).toBeNull();
    expect(el.querySelector('.mount__note')).toBeNull();
  });

  it('with no mark yet, says above the select what marking does, where the open list does not cover it', async () => {
    const { el } = await setup({});
    const note = el.querySelector('.mount__note')!;
    expect(
      (note.compareDocumentPosition(el.querySelector('mat-form-field')!) &
        Node.DOCUMENT_POSITION_FOLLOWING) !==
        0,
    ).toBe(true);
    expect(text(el.querySelector('.mount__hint'))).toBe('Não aceita cavaleiro.');
    expect(text(el.querySelector('.mount__note'))).toBe(
      'Com a marca, o Cavalo de guerra aceita um cavaleiro, que monta na própria vez. Você monta qualquer criatura.',
    );
  });

  it('takes the focus when it opens, as the cover picker does', async () => {
    const { fixture, el } = await setup({ mountKind: 'controlled' });
    await fixture.whenStable();
    expect(document.activeElement).toBe(el.querySelector('mat-select'));
  });

  it('shows no saddle before "Nenhuma": nothing is marked', async () => {
    const { el } = await setup({});
    expect(text(el.querySelector('.mat-mdc-select-value'))).toBe('Nenhuma');
    expect(el.querySelector('.mat-mdc-select-value app-saddle-icon')).toBeNull();
  });

  it('while a call is in the air, keeps the select and its focus and holds every option off', async () => {
    const { fixture, el, open, picked } = await setup({ mountKind: 'controlled' });
    fixture.componentRef.setInput('busy', true);
    fixture.detectChanges();
    const select = el.querySelector('mat-select')!;
    expect(select.getAttribute('aria-disabled')).toBe('false');
    expect(select.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(select);
    const options = open();
    expect(options.map((o) => o.getAttribute('aria-disabled'))).toEqual(['true', 'true', 'true']);
    options[2].click();
    expect(picked).toEqual([]);
  });
});
