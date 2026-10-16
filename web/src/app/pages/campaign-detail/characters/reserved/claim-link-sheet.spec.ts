import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import { ClaimsService } from '../../../../core/characters/claims.service';
import { ClaimLinkSheet, type ClaimLinkData } from './claim-link-sheet';

// With --coverage test-setup.ts only hooks the first file of a worker, so the stubbed globals this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.unstubAllGlobals();
});

const DATA: ClaimLinkData = {
  campaignId: 'camp-1',
  characterId: 'char-1',
  characterName: 'Kai',
  description: 'Monge 5, Humano',
};

describe('ClaimLinkSheet: "Gerar link para o jogador" (MR-049, PM-09 state 4)', () => {
  let claims: { createLink: ReturnType<typeof vi.fn> };
  let close: ReturnType<typeof vi.fn>;

  async function setup() {
    claims = { createLink: vi.fn() };
    close = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        { provide: ClaimsService, useValue: claims },
        { provide: MAT_DIALOG_DATA, useValue: DATA },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(ClaimLinkSheet);
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    const text = () => (el.textContent ?? '').replace(/\s+/g, ' ');
    const button = (label: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(label),
      )!;
    return { fixture, el, settle, text, button };
  }

  it('asks how long the link works before making it: 1, 7 or 30 days, 7 by default', async () => {
    const { el, text } = await setup();

    expect(text()).toContain(
      'Personagem: Kai (Monge 5, Humano). O jogador entra com a conta Google dele e assume o personagem.',
    );
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[name="claim-days"]'));
    expect(radios.map((r) => r.value)).toEqual(['1', '7', '30']);
    expect(radios.find((r) => r.checked)?.value).toBe('7');
    expect(text()).toContain('1 dia');
    expect(text()).toContain('7 dias');
    expect(text()).toContain('30 dias');
    expect(text()).toContain('Uso único. Depois que o link é mostrado, a validade não muda.');
    expect(claims.createLink).not.toHaveBeenCalled();
  });

  it('makes the link for the chosen validity and shows it once, whole, with its date', async () => {
    const { fixture, el, settle, text, button } = await setup();
    const expires = new Date(2026, 9, 16, 19, 5);
    claims.createLink.mockResolvedValue({
      token: 'tok-secret',
      expiresAt: timestampFromDate(expires),
    });

    el.querySelectorAll<HTMLInputElement>('input[name="claim-days"]')[2].click();
    await settle();
    button('Gerar link').click();
    await settle();

    expect(claims.createLink).toHaveBeenCalledWith('camp-1', 'char-1', 30);
    const field = el.querySelector<HTMLTextAreaElement>('.claim-link__field')!;
    expect(field.value).toBe(`${window.location.origin}/claim#t=tok-secret`);
    expect(field.readOnly).toBe(true);
    expect(text()).toContain('Link de Kai · válido por 30 dias (até 16/10 às 19:05) · uso único.');
    expect(text()).toContain('Este link aparece só agora.');
    expect(text()).toContain('Para revogar, use a linha do personagem.');
    expect(text()).toContain('Fechar');
    expect(text()).not.toContain('Pronto');
    expect(text()).not.toContain('Revogar este link');
    fixture.destroy();
  });

  it('copies the whole link and says so in a live region', async () => {
    const { el, settle, text, button } = await setup();
    claims.createLink.mockResolvedValue({
      token: 'tok-secret',
      expiresAt: timestampFromDate(new Date()),
    });
    const writeText = vi.fn(() => Promise.resolve());
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    button('Gerar link').click();
    await settle();

    button('Copiar link').click();
    await settle();

    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/claim#t=tok-secret`);
    expect(text()).toContain('Link copiado.');
    expect(el.querySelector('.copied')?.getAttribute('aria-live')).toBe('polite');
    expect(el.querySelector('[data-copy] mat-icon')?.textContent).toContain('content_copy');
  });

  it('when the clipboard is not allowed, selects the link for the keyboard', async () => {
    const { settle, text, button } = await setup();
    claims.createLink.mockResolvedValue({
      token: 'tok-secret',
      expiresAt: timestampFromDate(new Date()),
    });
    vi.stubGlobal('navigator', { clipboard: { writeText: () => Promise.reject(new Error('no')) } });
    button('Gerar link').click();
    await settle();

    button('Copiar link').click();
    await settle();

    expect(text()).toContain('Não foi possível copiar sozinho');
  });

  it('a failure stays on the choice with the reason, and nothing is shown', async () => {
    const { el, settle, text, button } = await setup();
    claims.createLink.mockRejectedValue(new ConnectError('down', Code.PermissionDenied));

    button('Gerar link').click();
    await settle();

    expect(text()).toContain('Você não tem permissão para fazer isso.');
    expect(el.querySelector('.claim-link__field')).toBeNull();
    expect(button('Gerar link')).toBeTruthy();
  });

  it('closing tells the list whether a link was made', async () => {
    const { settle, button } = await setup();
    button('Cancelar').click();
    expect(close).toHaveBeenLastCalledWith(false);

    claims.createLink.mockResolvedValue({ token: 't', expiresAt: timestampFromDate(new Date()) });
    button('Gerar link').click();
    await settle();
    button('Fechar').click();
    expect(close).toHaveBeenLastCalledWith(true);
  });
});
