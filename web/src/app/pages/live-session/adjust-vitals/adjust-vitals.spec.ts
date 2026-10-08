import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { AuthService } from '../../../core/auth/auth.service';
import { LiveErrorKind, LiveSessionSource, VitalsVm } from '../live-session.types';
import { brisaVitals, pensantusVitals } from '../testing';
import { AdjustVitals } from './adjust-vitals';
import { AdjustVitalsData } from './adjust-vitals.types';

class KindError extends Error {
  constructor(readonly kind: LiveErrorKind) {
    super(kind);
  }
}

describe('AdjustVitals (RN-02, E5-05)', () => {
  const adjustVitals = vi.fn();
  const close = vi.fn();
  const source = {
    adjustVitals,
    classifyError: (err: unknown) => (err instanceof KindError ? err.kind : 'transient'),
  };

  beforeEach(() => {
    adjustVitals.mockReset();
    close.mockReset();
  });

  function setup(vitals: VitalsVm, playerName: string | null = 'Ana') {
    const data: AdjustVitalsData = {
      campaignId: 'mirathel',
      vitals,
      sub: 'Ladina 3, de Ana',
      playerName,
    };
    TestBed.configureTestingModule({
      imports: [AdjustVitals],
      providers: [
        { provide: LiveSessionSource, useValue: source },
        {
          provide: AuthService,
          useValue: { signIn: vi.fn(), state: signal({ status: 'signed-in' }) },
        },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(AdjustVitals);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const byLabel = (el: HTMLElement, label: string) =>
    el.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
  const field = (el: HTMLElement, label: string) =>
    el.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`)!;
  const button = (el: HTMLElement, text: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text)!;

  async function settle(fixture: ComponentFixture<AdjustVitals>) {
    await fixture.whenStable();
    fixture.detectChanges();
  }

  it('shows the character, the maximum and who sees the change', () => {
    const { el } = setup(brisaVitals());
    expect(el.querySelector('h2')?.textContent).toContain('Ajustar Brisa');
    expect(el.textContent).toContain('máximo 24');
    expect(el.textContent).toContain('3d8, 0 de 3 usados');
    expect(el.textContent).toContain('Ana vê a mudança na hora.');
    // Brisa has no spell slots: no section at all.
    expect(el.textContent).not.toContain('Espaços de');
  });

  it('stops the steppers at the limits (RN-02: never above the maximum)', async () => {
    const { el, fixture } = setup(brisaVitals({ hitPointsCurrent: 22 }));
    expect(byLabel(el, 'Somar 5 PV').disabled).toBe(false);
    byLabel(el, 'Somar 5 PV').click();
    await settle(fixture);
    expect(field(el, 'Pontos de vida atuais').value).toBe('24');
    expect(byLabel(el, 'Somar 5 PV').disabled).toBe(true);
    expect(byLabel(el, 'Somar 1 PV').disabled).toBe(true);
    // 0 hit dice used: nothing to give back.
    expect(byLabel(el, 'Devolver 1 dado de vida').disabled).toBe(true);
  });

  it('refuses a typed value above the maximum, and saves nothing', async () => {
    const { el, fixture } = setup(brisaVitals());
    const hp = field(el, 'Pontos de vida atuais');
    hp.value = '30';
    hp.dispatchEvent(new Event('input'));
    await settle(fixture);
    expect(el.textContent).toContain('Use um número de 0 a 24.');
    expect(hp.getAttribute('aria-invalid')).toBe('true');

    button(el, 'Salvar ajuste').click();
    await settle(fixture);
    expect(adjustVitals).not.toHaveBeenCalled();
  });

  it('saves only what changed, with one idempotency key, and closes with the new vitals', async () => {
    const saved = brisaVitals({ hitPointsCurrent: 4, revision: 3 });
    adjustVitals.mockResolvedValue(saved);
    const { el, fixture } = setup(brisaVitals());
    byLabel(el, 'Tirar 5 PV').click();
    await settle(fixture);
    button(el, 'Salvar ajuste').click();
    await settle(fixture);

    expect(adjustVitals).toHaveBeenCalledTimes(1);
    const [campaignId, characterId, key, change] = adjustVitals.mock.calls[0];
    expect([campaignId, characterId]).toEqual(['mirathel', 'brisa']);
    expect(key).toMatch(/^[0-9a-f-]{36}$/);
    expect(change).toEqual({ hitPointsCurrent: 4 });
    expect(close).toHaveBeenCalledWith({ kind: 'saved', vitals: saved });
  });

  it('reuses the key on a retry of the same numbers, and makes a new one for new numbers', async () => {
    adjustVitals.mockRejectedValueOnce(new Error('network'));
    adjustVitals.mockRejectedValueOnce(new Error('network'));
    adjustVitals.mockResolvedValue(pensantusVitals());
    const { el, fixture } = setup(pensantusVitals());
    byLabel(el, 'Usar 1 espaço de 2º nível').click();
    await settle(fixture);

    button(el, 'Salvar ajuste').click();
    await settle(fixture);
    expect(el.textContent).toContain('Não foi possível salvar o ajuste.');
    button(el, 'Salvar ajuste').click();
    await settle(fixture);
    const [first, second] = adjustVitals.mock.calls.map((c) => c[2]);
    expect(second).toBe(first);

    byLabel(el, 'Usar 1 espaço de 2º nível').click();
    await settle(fixture);
    button(el, 'Salvar ajuste').click();
    await settle(fixture);
    expect(adjustVitals.mock.calls[2][2]).not.toBe(first);
    expect(adjustVitals.mock.calls[2][3]).toEqual({ spellSlotsUsed: [{ level: 2, used: 2 }] });
  });

  it('says "A sessão acabou" when the save comes after the session ended', async () => {
    adjustVitals.mockRejectedValue(new KindError('no-session'));
    const { el, fixture } = setup(brisaVitals());
    byLabel(el, 'Somar 1 PV').click();
    await settle(fixture);
    button(el, 'Salvar ajuste').click();
    await settle(fixture);

    expect(el.textContent).toContain('A sessão acabou.');
    expect(button(el, 'Salvar ajuste')).toBeUndefined();
    button(el, 'Fechar').click();
    expect(close).toHaveBeenCalledWith({ kind: 'ended' });
  });

  it('closes without a call when nothing changed', async () => {
    const { el, fixture } = setup(brisaVitals());
    button(el, 'Salvar ajuste').click();
    await settle(fixture);
    expect(adjustVitals).not.toHaveBeenCalled();
    expect(close).toHaveBeenCalledWith(undefined);
  });

  it('says so when the player has no display name', () => {
    const { el } = setup(brisaVitals(), null);
    expect(el.textContent).toContain('Quem joga com Brisa vê a mudança na hora.');
  });

  it('gives back a spent use of a resource, and sends only that resource', async () => {
    adjustVitals.mockResolvedValue(brisaVitals());
    const { el, fixture } = setup(
      brisaVitals({
        resources: [
          { key: 'rage', namePt: 'Fúria', total: 2, used: 2, recharge: 'long_rest' },
          {
            key: 'second_wind',
            namePt: 'Retomar o Fôlego',
            total: 1,
            used: 1,
            recharge: 'short_rest',
          },
        ],
      }),
    );
    expect(el.textContent).toContain('Fúria');
    expect(el.textContent).toContain('Retomar o Fôlego');
    expect(byLabel(el, 'Usar 1 uso de Fúria').disabled).toBe(true);
    byLabel(el, 'Devolver 1 uso de Fúria').click();
    await settle(fixture);
    button(el, 'Salvar ajuste').click();
    await settle(fixture);
    expect(adjustVitals.mock.calls[0][3]).toEqual({ resourcesUsed: [{ key: 'rage', used: 1 }] });
  });

  it('never shows the internal key of a resource that has no Portuguese name', () => {
    const { el } = setup(
      brisaVitals({
        resources: [
          { key: 'feature:odd-pool', namePt: '', total: 1, used: 1, recharge: 'long_rest' },
        ],
      }),
    );
    expect(el.textContent).not.toContain('odd-pool');
    expect(byLabel(el, 'Devolver 1 uso de Recurso')).toBeDefined();
  });

  it("adjusts the beast's hit points apart from the character's, and 0 ends the form", async () => {
    adjustVitals.mockResolvedValue(brisaVitals());
    const wolf = {
      beastKey: 'monster:wolf',
      beastNamePt: 'Lobo',
      hitPointsCurrent: 9,
      hitPointsMax: 11,
    };
    const { el, fixture } = setup(brisaVitals({ wildShape: wolf }));
    expect(el.textContent).toContain('PV da fera: Lobo');
    expect(el.textContent).toContain('0 encerra a forma');
    const beast = field(el, 'Pontos de vida da fera');
    beast.value = '0';
    beast.dispatchEvent(new Event('input'));
    await settle(fixture);
    button(el, 'Salvar ajuste').click();
    await settle(fixture);
    expect(adjustVitals.mock.calls[0][3]).toEqual({ wildShapeHitPointsCurrent: 0 });
  });

  it('offers no beast row and no resource row to a character without them', () => {
    const { el } = setup(brisaVitals());
    expect(el.textContent).not.toContain('PV da fera');
    expect(el.querySelector('input[aria-label="Pontos de vida da fera"]')).toBeNull();
  });
});
