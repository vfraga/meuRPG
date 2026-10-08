import { ComponentFixture, TestBed } from '@angular/core/testing';

import { type CampaignDocument } from '../../../../gen/meurpg/campaigns/v1/campaign_document_pb';
import { DocumentClient } from '../document-clients';
import { DocumentEditor } from './document-editor';

const doc = { body: 'Texto salvo', revision: 1n } as unknown as CampaignDocument;

describe('DocumentEditor: focus after "Descartar mudanças"', () => {
  let fixture: ComponentFixture<DocumentEditor>;
  let errors: unknown[];

  beforeEach(() => {
    // Zoneless Angular reports an error thrown inside `afterNextRender` to the console and carries on, so the
    // test listens to the console to see it.
    errors = [];
    vi.spyOn(console, 'error').mockImplementation((...args) => void errors.push(args));
    TestBed.configureTestingModule({ providers: [{ provide: DocumentClient, useValue: {} }] });
    fixture = TestBed.createComponent(DocumentEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('doc', doc);
  });

  const el = () => fixture.nativeElement as HTMLElement;
  const button = (label: string) =>
    Array.from(el().querySelectorAll<HTMLButtonElement>('button')).find(
      (b) => b.textContent?.trim() === label,
    )!;

  it('puts the focus on the confirmation button, the one that replaced the button that was pressed', async () => {
    await fixture.whenStable();
    const area = el().querySelector<HTMLTextAreaElement>('textarea')!;
    area.value = 'Texto novo';
    area.dispatchEvent(new Event('input'));
    await fixture.whenStable();

    button('Descartar mudanças').click();
    await fixture.whenStable();

    // The confirmation is the only "Descartar mudanças" left, inside the group that asks the question.
    const confirm = el().querySelector<HTMLButtonElement>('[role=group] .danger')!;
    expect(confirm).toBeTruthy();
    expect(document.activeElement).toBe(confirm);
    expect(errors).toEqual([]);
  });

  it('a save that answers after the person left the page emits nothing (NG0953)', async () => {
    let answer!: (d: CampaignDocument) => void;
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        {
          provide: DocumentClient,
          useValue: { save: () => new Promise<CampaignDocument>((r) => (answer = r)) },
        },
      ],
    });
    fixture = TestBed.createComponent(DocumentEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('doc', doc);
    const saved = vi.fn();
    fixture.componentInstance.saved.subscribe(saved);
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    await fixture.whenStable();
    const area = el().querySelector<HTMLTextAreaElement>('textarea')!;
    area.value = 'Texto novo';
    area.dispatchEvent(new Event('input'));
    await fixture.whenStable();

    button('Salvar documento').click();
    fixture.destroy();
    answer({ body: 'Texto novo', revision: 2n } as unknown as CampaignDocument);
    await Promise.resolve();
    await Promise.resolve();

    expect(saved).not.toHaveBeenCalled();
    expect(warn).not.toHaveBeenCalled();
    expect(errors).toEqual([]);
  });

  it('does not take typing while a save is on its way, because the answer replaces the text with the saved one', async () => {
    let answer!: (d: CampaignDocument) => void;
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        {
          provide: DocumentClient,
          useValue: { save: () => new Promise<CampaignDocument>((r) => (answer = r)) },
        },
      ],
    });
    fixture = TestBed.createComponent(DocumentEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('doc', doc);
    await fixture.whenStable();
    const area = el().querySelector<HTMLTextAreaElement>('textarea')!;
    expect(area.readOnly).toBe(false);
    area.value = 'Texto novo';
    area.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    button('Salvar documento').click();
    await fixture.whenStable();
    expect(area.readOnly).toBe(true);
    answer({ body: 'Texto novo', revision: 2n } as unknown as CampaignDocument);
    await Promise.resolve();
    await Promise.resolve();
    fixture.detectChanges();
    expect(area.readOnly).toBe(false);
  });
});
