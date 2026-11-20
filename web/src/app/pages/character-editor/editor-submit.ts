import { type Injector, type Signal, type WritableSignal, afterNextRender } from '@angular/core';

import { characterBlockedMessage, contentRef } from '../../core/characters/character-errors';
import { isFullSheetKind } from '../../core/characters/characters.types';
import type { ChoiceGroups } from '../../shared/choice-groups/choice-groups';
import { offControlOf } from './catalog-changes';
import type { EditorCatalog } from './editor-catalog';
import type { EditorDraft } from './editor-draft';
import { EDITOR_STEP_LABELS, type EditorStepKey } from './editor-labels';
import type { EditorCore } from './editor-core';
import type { EditorStepper } from './editor-stepper/editor-stepper';
import type { SaveFlow } from './save-flow';

/** What `EditorSubmit` reads from, and drives on, the page. */
export interface EditorSubmitHost extends EditorCore {
  readonly draft: EditorDraft;
  readonly views: EditorCatalog;
  readonly saving: SaveFlow;
  /** A choice is still open on a player's sheet: the button only leads to it. */
  readonly choicesBlocked: Signal<boolean>;
  readonly bonusesOpen: WritableSignal<boolean>;
  readonly stepper: () => EditorStepper | undefined;
  readonly choiceGroups: () => ChoiceGroups | undefined;
  readonly element: HTMLElement;
  readonly injector: Injector;
}

/**
 * The save button and what it does before the request: a choice still open leads to the "Escolhas" step, a field
 * to fix opens the step that has it, and a key the master switched off marks its field and gets the focus. A plain
 * class the editor builds; the request itself is `SaveFlow`.
 */
export class EditorSubmit {
  constructor(private readonly host: EditorSubmitHost) {}

  async submit(): Promise<void> {
    const h = this.host;
    const s = h.state();
    if (s.status !== 'ready') {
      return;
    }
    const isBasic = !isFullSheetKind(s.kind);
    // A choice still open: the button only leads to it (it is aria-disabled, not disabled, so it can be reached and activated).
    if (!isBasic && h.choicesBlocked()) {
      this.focusFirstPendingChoice();
      return;
    }
    if (h.draft.hasProblems(isBasic)) {
      this.showProblems(isBasic);
      return;
    }
    await h.saving.run(s, isBasic);
  }

  /** The index of the stepper's step with that label; -1 when the page has no stepper or the step. */
  private stepIndexOf(label: string): number {
    return (
      this.host
        .stepper()
        ?.steps.toArray()
        .findIndex((step) => step.label === label) ?? -1
    );
  }

  /** The blocked "Criar personagem": the "Escolhas" step opens and the focus goes to the first choice still open. */
  focusFirstPendingChoice(): void {
    const stepper = this.host.stepper();
    const index = this.stepIndexOf(EDITOR_STEP_LABELS.escolhas);
    if (!stepper || index < 0) {
      return;
    }
    if (index === stepper.selectedIndex) {
      this.host.choiceGroups()?.focusFirstPending();
      return;
    }
    stepper.goTo(index);
    afterNextRender(() => this.host.choiceGroups()?.focusFirstPending(), {
      injector: this.host.injector,
    });
  }

  /** Opens a step by its key; nothing when the page has no stepper or the step. */
  openStep(step: EditorStepKey): void {
    const stepper = this.host.stepper();
    const index = this.stepIndexOf(EDITOR_STEP_LABELS[step]);
    if (stepper && index >= 0 && index !== stepper.selectedIndex) {
      stepper.goTo(index);
    }
  }

  /** After a submit with an invalid field: opens the step of the first one
   * (and "Bônus manuais", if that is where it is), so the field and its
   * error message are on screen. */
  openFirstInvalidStep(): void {
    const [first] = this.host.draft.currentInvalidFullFields();
    if (!first?.step) {
      return;
    }
    if (first.path.startsWith('extraAbilityBonuses.')) {
      this.host.bonusesOpen.set(true);
    }
    const stepper = this.host.stepper();
    const index = this.stepIndexOf(EDITOR_STEP_LABELS[first.step]);
    if (stepper && index >= 0 && index !== stepper.selectedIndex) {
      stepper.goTo(index);
    }
  }

  /** A save with something to fix: every field says so, the notice lists them and the first step opens. */
  private showProblems(isBasic: boolean): void {
    const h = this.host;
    (isBasic ? h.basicForm : h.fullForm).markAllAsTouched();
    h.saving.state.set({ status: 'idle' });
    h.draft.showErrors.set(true);
    if (!isBasic) {
      this.openFirstInvalidStep();
    }
  }

  /**
   * The server refused a key the master switched off: its field gets the error (`switchedOff`, so it is invalid, red and
   * `aria-invalid`), the step is marked "(com erro)", the editor opens it and the focus goes to the field. Returns false when
   * no field holds the key (a spell, say): the notice above the buttons says it by name instead.
   */
  markSwitchedOff(key: string): boolean {
    const h = this.host;
    const control = offControlOf(key, h.fullForm.getRawValue());
    if (!control) {
      return false;
    }
    const ref = contentRef(key, (k) => h.views.nameOfKey(k));
    h.fullForm.controls[control].setErrors({
      switchedOff: characterBlockedMessage('switched_off_content', { ...ref, step: '' }),
    });
    h.fullForm.controls[control].markAsTouched();
    h.draft.showErrors.set(true);
    h.saving.state.set({ status: 'idle' });
    this.openFirstInvalidStep();
    afterNextRender(
      () => h.element.querySelector<HTMLElement>(`[formcontrolname="${control}"]`)?.focus(),
      { injector: h.injector },
    );
    return true;
  }

  /** The sentence of a switched-off option on its field, for the template. */
  switchedOff(control: 'className' | 'race' | 'subrace' | 'subclassName' | 'background'): string {
    return (this.host.fullForm.controls[control].getError('switchedOff') as string | null) ?? '';
  }
}
