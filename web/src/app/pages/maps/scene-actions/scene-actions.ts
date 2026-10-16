import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { SceneAction } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { sceneActionErrorMessage } from '../../../core/maps/map-errors';
import { MapsClient } from '../../../core/maps/maps-client';
import { ActionKey } from '../../../core/connect/idempotency';
import { SCENE_ACTION_LIMIT, actionSubtitle, actionTitle } from '../../../core/maps/scene-actions';
import { RevealSwitch } from '../../../shared/reveal-switch/reveal-switch';
import { SceneActionForm, type NewSceneAction, type SceneActionChanges } from './scene-action-form';

/** 1 to 5 attempts, or 0: unlimited (maps.proto, `UpdateSceneAction`). */
const ATTEMPT_OPTIONS = [1, 2, 3, 4, 5, 0] as const;

/**
 * "Ações da cena" in the point panel of a SCENE point (E7-01, MR-015): what
 * the players may roll in the scene, one to twenty checks. Each change saves
 * at once (add, ↑, ↓, remove: no "save all", no question before removing), so
 * the panel's "Salvar ponto" never carries them. One write at a time: while
 * one is in flight, the other buttons answer nothing (`aria-disabled`, not
 * `disabled`, so focus stays where it was).
 *
 * Each action also has "Tentativas por jogador" (a 44px select, 1 by default, up to 5 or
 * "Sem limite"; MR-015, question 55) and the list starts with the switch "Mostrar a CD aos
 * jogadores" (RN-20, a flag of the point). Both save at once (E8-13): the switch with `show_dc`
 * alone, so the panel's unsaved text is never sent or overwritten.
 *
 * "Editar" (the pencil of a row) opens "Editar ação" in place of the row, filled with the action's check, name and DC;
 * it saves only the fields that changed and the focus returns to the pencil.
 *
 * Focus: after ↑ or ↓ it stays on the same button of the moved row; after a
 * removal it goes to the next row's "Remover", or to "Adicionar ação" when
 * there is none; "Adicionar ação" opens the form with focus on its first
 * choice, and returns there when the form closes.
 */
@Component({
  selector: 'app-scene-actions',
  imports: [MatButtonModule, MatIconModule, RevealSwitch, SceneActionForm],
  templateUrl: './scene-actions.html',
  styleUrl: './scene-actions.scss',
})
export class SceneActions {
  private readonly api = inject(MapsClient);
  private readonly addKey = new ActionKey();
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  readonly mapId = input.required<string>();
  readonly pointId = input.required<string>();
  readonly actions = input.required<readonly SceneAction[]>();
  /** "Mostrar a CD aos jogadores" as the server has it. */
  readonly showDc = input(false);
  /** The switch was saved (`UpdateMapPoint` with `show_dc` alone): the point carries it. */
  readonly showDcSaved = output<boolean>();
  /** The point's actions as the server has them now (after each write). */
  readonly actionsChange = output<readonly SceneAction[]>();

  protected readonly limit = SCENE_ACTION_LIMIT;
  protected readonly attemptOptions = ATTEMPT_OPTIONS;
  protected readonly attemptLabel = (o: number): string => (o === 0 ? 'Sem limite' : String(o));
  protected readonly title = actionTitle;
  protected readonly subtitle = actionSubtitle;
  protected readonly adding = signal(false);
  protected readonly busy = signal(false);
  /** A refusal of the add form, shown inside it. */
  protected readonly formError = signal('');
  /** The action whose "Editar ação" form is open in place of its row: one at a time. */
  protected readonly editing = signal<string | null>(null);
  /** A refusal of the edit form, shown inside it. */
  protected readonly editError = signal('');
  /** A refusal of a move or a removal. */
  protected readonly error = signal('');
  /** What a screen reader hears after a change. */
  protected readonly status = signal('');
  protected readonly full = computed(() => this.actions().length >= SCENE_ACTION_LIMIT);

  private readonly addButton = viewChild('addButton', { read: ElementRef<HTMLButtonElement> });

  protected openForm(): void {
    if (this.full()) {
      return;
    }
    // One form at a time: nothing of an edit was saved, so it closes without asking.
    this.editing.set(null);
    this.formError.set('');
    this.adding.set(true);
  }

  protected closeForm(): void {
    this.adding.set(false);
    this.formError.set('');
    this.focusAdd();
  }

  /** The pencil of a row: its form opens in place of the row, filled with what the action has. */
  protected openEdit(action: SceneAction): void {
    if (this.busy()) {
      return;
    }
    this.adding.set(false);
    this.formError.set('');
    this.editError.set('');
    this.editing.set(action.id);
  }

  protected closeEdit(action: SceneAction): void {
    this.editing.set(null);
    this.editError.set('');
    this.focusRow(action.id, 'edit');
  }

  /** "Salvar ação": the fields that changed go to the server; the row comes back with the new values and the focus on its pencil. */
  protected async save(action: SceneAction, changes: SceneActionChanges): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.editError.set('');
    try {
      const actions = await this.api.updateSceneAction(
        this.campaignId(),
        this.mapId(),
        this.pointId(),
        action.id,
        changes,
      );
      this.actionsChange.emit(actions);
      this.editing.set(null);
      const saved = actions.find((a) => a.id === action.id) ?? action;
      this.status.set(`Ação salva: ${actionTitle(saved)}.`);
      this.focusRow(action.id, 'edit');
    } catch (err) {
      this.editError.set(sceneActionErrorMessage(err, 'salvar a ação'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async add(action: NewSceneAction): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.formError.set('');
    try {
      const actions = await this.api.addSceneAction(
        this.campaignId(),
        this.mapId(),
        this.pointId(),
        action,
        this.addKey.keyFor([this.pointId(), action]),
      );
      this.addKey.renew();
      this.actionsChange.emit(actions);
      this.adding.set(false);
      this.status.set(`Ação adicionada. ${actions.length} de ${SCENE_ACTION_LIMIT}.`);
      this.focusAdd();
    } catch (err) {
      this.formError.set(sceneActionErrorMessage(err, 'adicionar a ação'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async move(action: SceneAction, direction: 'up' | 'down'): Promise<void> {
    const index = this.actions().findIndex((a) => a.id === action.id);
    const edge = direction === 'up' ? index === 0 : index === this.actions().length - 1;
    if (this.busy() || edge) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const actions = await this.api.moveSceneAction(
        this.campaignId(),
        this.mapId(),
        this.pointId(),
        action.id,
        direction,
      );
      this.actionsChange.emit(actions);
      const place = actions.findIndex((a) => a.id === action.id) + 1;
      this.status.set(`${actionTitle(action)}: posição ${place} de ${actions.length}.`);
      // The moved row keeps the same button, as soon as it is drawn.
      this.focusRow(action.id, direction === 'up' ? 'up' : 'down');
    } catch (err) {
      this.error.set(sceneActionErrorMessage(err, 'mover a ação'));
    } finally {
      this.busy.set(false);
    }
  }

  /** "Mostrar a CD aos jogadores": saved at once, with only `show_dc` set, so whatever else
   * is being typed in the panel stays as it is. A refusal leaves the switch where it was
   * (it is controlled) and says why. */
  protected async setShowDc(on: boolean): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.updatePoint(this.campaignId(), this.mapId(), this.pointId(), { showDc: on });
      this.showDcSaved.emit(on);
      this.status.set(on ? 'Os jogadores agora veem a CD.' : 'Só você vê a CD.');
    } catch (err) {
      this.error.set(sceneActionErrorMessage(err, 'mudar a CD'));
    } finally {
      this.busy.set(false);
    }
  }

  /** "Tentativas por jogador": saved at once, like the rest of the list. The select is put
   * back on what the server has when the save is refused or ignored (one write at a time). */
  protected async setAttempts(action: SceneAction, select: HTMLSelectElement): Promise<void> {
    const max = Number(select.value);
    if (this.busy() || max === action.maxAttempts) {
      select.value = String(action.maxAttempts);
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const actions = await this.api.setSceneActionAttempts(
        this.campaignId(),
        this.mapId(),
        this.pointId(),
        action.id,
        max,
      );
      this.actionsChange.emit(actions);
      this.status.set(
        `${actionTitle(action)}: ${max === 0 ? 'sem limite de tentativas' : `${max} ${max === 1 ? 'tentativa' : 'tentativas'} por jogador`}.`,
      );
    } catch (err) {
      select.value = String(action.maxAttempts);
      this.error.set(sceneActionErrorMessage(err, 'mudar as tentativas'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async remove(action: SceneAction): Promise<void> {
    if (this.busy()) {
      return;
    }
    const before = this.actions();
    const index = before.findIndex((a) => a.id === action.id);
    this.busy.set(true);
    this.error.set('');
    try {
      const actions = await this.api.removeSceneAction(
        this.campaignId(),
        this.mapId(),
        this.pointId(),
        action.id,
      );
      this.actionsChange.emit(actions);
      this.status.set(`Ação removida: ${actionTitle(action)}.`);
      const next = actions[index];
      if (next) {
        this.focusRow(next.id, 'remove');
      } else {
        this.focusAdd();
      }
    } catch (err) {
      this.error.set(sceneActionErrorMessage(err, 'remover a ação'));
    } finally {
      this.busy.set(false);
    }
  }

  private focusRow(actionId: string, control: 'up' | 'down' | 'edit' | 'remove'): void {
    afterNextRender(
      () =>
        this.host.nativeElement
          .querySelector<HTMLElement>(`[data-action="${actionId}"][data-control="${control}"]`)
          ?.focus(),
      { injector: this.injector },
    );
  }

  private focusAdd(): void {
    afterNextRender(() => this.addButton()?.nativeElement.focus(), { injector: this.injector });
  }
}
