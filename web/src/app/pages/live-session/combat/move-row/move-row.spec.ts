import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import { CombatantSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { MoveRow } from './move-row';

describe('MoveRow', () => {
  function setup(
    inputs: Partial<{
      locked: string;
      button: boolean;
      drag: boolean;
      grappled: boolean;
      left: number;
    }> = {},
  ) {
    const fixture = TestBed.createComponent(MoveRow);
    const left = inputs.left ?? 30;
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, {
        label: 'Brisa',
        speedDft: 300,
        movementLeftDft: left * 10,
        movementLeftFt: left,
      }),
    );
    for (const [k, v] of Object.entries(inputs)) {
      if (k !== 'left') {
        fixture.componentRef.setInput(k, v);
      }
    }
    const moves: number[] = [];
    fixture.componentInstance.move.subscribe(() => moves.push(1));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    return { el, moves, button: () => el.querySelector<HTMLButtonElement>('button') };
  }
  const text = (el: HTMLElement) => (el.textContent ?? '').replace(/\s+/g, ' ');

  it('says the movement and has one "Mover" that opens the move page', () => {
    const { el, moves, button } = setup();
    expect(text(el)).toContain('Restam 9,0 m · 6 quadrados');
    expect(el.querySelectorAll('button')).toHaveLength(1);
    expect(button()!.textContent).toContain('Mover');
    expect(el.querySelector('.row__bar')).not.toBeNull();
    button()!.click();
    expect(moves).toEqual([1]);
  });

  it('turns "Mover" off with the reason instead of failing on click', () => {
    const { el, moves, button } = setup({ locked: 'Esperando a reação do mestre' });
    expect(button()!.getAttribute('aria-disabled') === 'true' || button()!.disabled).toBe(true);
    expect(button()!.getAttribute('aria-describedby')).toBe(el.querySelector('.row__why')!.id);
    expect(text(el)).toContain('Esperando a reação do mestre');
    button()!.click();
    expect(moves).toEqual([]);
  });

  it("does not repeat the master's wait: the button points to the card that says it once", () => {
    const { el, moves, button } = setup({ locked: 'Esperando o mestre' });
    expect(el.querySelector('.row__why')).toBeNull();
    expect(text(el)).not.toContain('Esperando o mestre');
    expect(button()!.getAttribute('aria-disabled') === 'true' || button()!.disabled).toBe(true);
    expect(button()!.getAttribute('aria-describedby')).toBe('master-wait-note');
    button()!.click();
    expect(moves).toEqual([]);
  });

  it('has no button when there is nothing to move, or when the token is dragged on the map', () => {
    expect(setup({ left: 0 }).button()).toBeNull();
    expect(setup({ grappled: true }).button()).toBeNull();
    const dragging = setup({ button: false, drag: true });
    expect(dragging.button()).toBeNull();
    expect(text(dragging.el)).toContain('Restam 9,0 m · 6 quadrados · arraste o seu token no mapa');
    expect(dragging.el.querySelector('.row__bar')).toBeNull();
  });
});
