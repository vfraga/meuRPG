import type { Mock } from 'vitest';
import { ChangeDetectionStrategy, Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatSelect, MatSelectModule } from '@angular/material/select';
import { By } from '@angular/platform-browser';
import { OpenDown } from './open-down';
import { MatFormFieldModule } from '@angular/material/form-field';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatFormFieldModule, MatSelectModule, OpenDown],
  template: `<mat-form-field><mat-label>Armas</mat-label><mat-select appOpenDown multiple><mat-option value="a">A</mat-option></mat-select></mat-form-field>`,
})
class Host {}

describe('OpenDown', () => {
  // The spec asserts on a mock of its own: the one test-setup.ts gives every test is not renewed per test under
  // `--coverage` (the setup file runs once per worker), so it may hold the calls of earlier spec files.
  let scrollIntoView: Mock<Element['scrollIntoView']>;
  let previousScrollIntoView: typeof Element.prototype.scrollIntoView;

  beforeEach(() => {
    previousScrollIntoView = Element.prototype.scrollIntoView;
    scrollIntoView = vi.fn<Element['scrollIntoView']>();
    Element.prototype.scrollIntoView = scrollIntoView;
  });

  afterEach(() => {
    Element.prototype.scrollIntoView = previousScrollIntoView;
  });

  function setup(selectBottom: number) {
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const select = fixture.debugElement.query(By.directive(MatSelect));
    const field = fixture.nativeElement.querySelector('mat-form-field') as HTMLElement;
    vi.spyOn(select.nativeElement, 'getBoundingClientRect').mockReturnValue({
      bottom: selectBottom,
    } as DOMRect);
    return {
      select: select.componentInstance as MatSelect,
      selectElement: select.nativeElement as HTMLElement,
      field,
    };
  }

  it('keeps only the positions under the field', () => {
    const { select } = setup(100);
    expect(select._positions.length).toBeGreaterThan(0);
    expect(select._positions.every((p) => p.originY === 'bottom' && p.overlayY === 'top')).toBe(
      true,
    );
  });

  it('brings the field to the middle of the screen, at once and before the list opens, when the room below is short', () => {
    vi.stubGlobal('innerHeight', 900);
    const { select, field } = setup(800);
    let openWhenScrolled: boolean | undefined;
    scrollIntoView.mockImplementation(() => {
      openWhenScrolled = select.panelOpen;
    });
    select.open();
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
    expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'instant' });
    expect(scrollIntoView.mock.contexts[0]).toBe(field);
    expect(openWhenScrolled).toBe(false);
    expect(select.panelOpen).toBe(true);
  });

  it('leaves the page where it is when there is room below the field', () => {
    vi.stubGlobal('innerHeight', 900);
    const { select } = setup(200);
    select.open();
    expect(select.panelOpen).toBe(true);
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it('does not scroll the field when the select is disabled and cannot open', () => {
    vi.stubGlobal('innerHeight', 900);
    const { select } = setup(800);
    select.disabled = true;
    select.open();
    expect(select.panelOpen).toBe(false);
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it('does not scroll the field when the list is already open', () => {
    vi.stubGlobal('innerHeight', 900);
    const { select, selectElement } = setup(200);
    select.open();
    expect(select.panelOpen).toBe(true);
    vi.mocked(selectElement.getBoundingClientRect).mockReturnValue({ bottom: 800 } as DOMRect);
    select.open();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });
});
