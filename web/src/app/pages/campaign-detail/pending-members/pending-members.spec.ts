import { Injectable } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';

import { PendingMember } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { PendingMembers } from './pending-members';
import { nextTurn } from '../../../../test-settle';

function pending(userId: string, name: string): PendingMember {
  return {
    userId,
    displayName: name,
    joinedAt: timestampFromDate(new Date(2026, 8, 28)),
    expiresAt: timestampFromDate(new Date(2026, 9, 28)),
  } as PendingMember;
}

@Injectable()
class FakeCampaignsService {
  members: PendingMember[] = [pending('u2', 'Lia'), pending('u3', 'Davi')];
  removeError: ConnectError | null = null;
  removed: string[] = [];
  listPendingMembers() {
    return Promise.resolve({ members: this.members });
  }
  removePendingMember(_campaignId: string, userId: string) {
    if (this.removeError) {
      return Promise.reject(this.removeError);
    }
    this.removed.push(userId);
    return Promise.resolve({});
  }
}

describe('PendingMembers', () => {
  let fake: FakeCampaignsService;
  let fixture: ComponentFixture<PendingMembers>;
  let el: HTMLElement;

  async function render(): Promise<void> {
    TestBed.configureTestingModule({
      imports: [PendingMembers],
      providers: [{ provide: CampaignsService, useClass: FakeCampaignsService }],
    });
    fake = TestBed.inject(CampaignsService) as unknown as FakeCampaignsService;
    fixture = TestBed.createComponent(PendingMembers);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('campaignName', 'Mirathel');
    el = fixture.nativeElement as HTMLElement;
    await settle();
  }

  async function settle(): Promise<void> {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  const button = (text: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text)!;

  it('lists each person with the tag and their dates', async () => {
    await render();
    expect(el.querySelectorAll('li').length).toBe(2);
    expect(el.textContent).toContain('Sem personagem');
    expect(el.textContent).toContain(
      'Entrou pelo convite em 28/09, ainda sem personagem. Sai da campanha em 28/10, se nada mudar.',
    );
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('renders nothing for an empty list', async () => {
    TestBed.configureTestingModule({
      imports: [PendingMembers],
      providers: [{ provide: CampaignsService, useClass: FakeCampaignsService }],
    });
    (TestBed.inject(CampaignsService) as unknown as FakeCampaignsService).members = [];
    fixture = TestBed.createComponent(PendingMembers);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('campaignName', 'Mirathel');
    el = fixture.nativeElement as HTMLElement;
    await settle();
    expect(el.querySelector('li')).toBeNull();
  });

  it('confirms in place, with focus on "Cancelar", and cancelling removes nothing', async () => {
    await render();
    document.body.append(el);
    (el.querySelector('[aria-label="Remover Lia da campanha"]') as HTMLElement).click();
    await settle();
    const dialog = el.querySelector('[role="alertdialog"]')!;
    expect(dialog.textContent).toContain('Remover Lia de Mirathel?');
    expect(dialog.textContent).toContain('em vez de esperar até 28/10');
    expect(document.activeElement).toBe(button('Cancelar'));
    button('Cancelar').click();
    await settle();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(fake.removed).toEqual([]);
    el.remove();
  });

  it('removes the person, says so, and moves focus to the next row', async () => {
    await render();
    document.body.append(el);
    (el.querySelector('[aria-label="Remover Lia da campanha"]') as HTMLElement).click();
    await settle();
    button('Remover Lia').click();
    await settle();
    expect(fake.removed).toEqual(['u2']);
    expect(el.querySelectorAll('li').length).toBe(1);
    expect(el.querySelector('[role="status"]')!.textContent).toContain('Lia saiu de Mirathel.');
    expect(document.activeElement).toBe(
      el.querySelector('[aria-label="Remover Davi da campanha"]'),
    );
    el.remove();
  });

  it('on failed_precondition, refreshes the list and points to the pending characters', async () => {
    await render();
    fake.removeError = new ConnectError('x', Code.FailedPrecondition);
    fake.members = [pending('u3', 'Davi')];
    (el.querySelector('[aria-label="Remover Lia da campanha"]') as HTMLElement).click();
    await settle();
    button('Remover Lia').click();
    await settle();
    expect(el.querySelector('[role="alert"]')!.textContent).toContain(
      'Lia acabou de criar o personagem: aprove ou recuse em Esperando aprovação.',
    );
    expect(el.querySelectorAll('li').length).toBe(1);
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('maps other errors by code', async () => {
    await render();
    fake.removeError = new ConnectError('x', Code.Unavailable);
    (el.querySelector('[aria-label="Remover Lia da campanha"]') as HTMLElement).click();
    await settle();
    button('Remover Lia').click();
    await settle();
    expect(el.querySelector('[role="alert"]')!.textContent).toContain(
      'Não foi possível falar com o servidor',
    );
  });
});
