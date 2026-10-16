import { TestBed } from '@angular/core/testing';

import { NO_LAYERS } from '../../core/maps/layers';
import { EditorOverlay } from './editor-overlay';

describe('EditorOverlay, the grid lines (RN-25)', () => {
  function render(columns: number, rows: number, factor?: number) {
    const fixture = TestBed.createComponent(EditorOverlay);
    fixture.componentRef.setInput('columns', columns);
    fixture.componentRef.setInput('rows', rows);
    fixture.componentRef.setInput('layers', NO_LAYERS);
    if (factor !== undefined) {
      fixture.componentRef.setInput('factor', factor);
    }
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('draws one grid for a map that was never calibrated', () => {
    const el = render(4, 2);
    expect(el.querySelectorAll('.grid path')).toHaveLength(1);
    expect(el.querySelector('.grid path')?.classList.contains('rules')).toBe(false);
  });

  it('on a calibrated map draws the rules grid dotted and the drawing lines solid, every `factor` squares', () => {
    const el = render(8, 4, 2);
    const paths = el.querySelectorAll('.grid path');
    expect(paths).toHaveLength(2);
    expect(paths[0].classList.contains('rules')).toBe(true);
    expect(paths[1].classList.contains('drawn')).toBe(true);
    // Columns 0, 2, 4, 6 and 8, rows 0, 2 and 4: the edges and every second line.
    const d = paths[1].getAttribute('d')!;
    expect(d.match(/M\d+ 0V4/g)).toEqual(['M0 0V4', 'M2 0V4', 'M4 0V4', 'M6 0V4', 'M8 0V4']);
    expect(d.match(/M0 \dH8/g)).toEqual(['M0 0H8', 'M0 2H8', 'M0 4H8']);
  });
});
