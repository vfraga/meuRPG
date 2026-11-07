import { NgTemplateOutlet } from '@angular/common';
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

import { type Encounter, EncounterStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  combatantInitial,
  missingInitiative,
  isPlayer,
  roundLabel,
  turnBanner,
} from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { pendingCountText } from '../../../../core/held-rolls/held-text';
import { unactedPlayerOnTurn } from '../../../../core/combat/combat-derived';
import { NextTurn } from './next-turn';
import { TheatrePill } from '../theatre/theatre-pill';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';

/**
 * The master's combat bar (E6-04, E6-11, E6-12, E6-16): which combat, whose
 * turn, and the two buttons that move it. From 1024 px it sticks under the app bar. In SETUP it says "Iniciativa"
 * and what to do, and "Começar o combate" (the page's one filled button, dashed and `aria-disabled` while someone has not
 * rolled) and "Cancelar combate" sit where "Próximo turno" and "Encerrar combate" will be. Running, "Próximo turno" is the page's one filled button
 * (EndTurn), and "Encerrar combate" asks in place: the confirmation takes
 * the bar's buttons, and the focus goes to the safe "Cancelar".
 */
@Component({
  selector: 'app-combat-bar',
  imports: [
    CombatantToken,
    MatButtonModule,
    MatIconModule,
    NextTurn,
    NgTemplateOutlet,
    TheatrePill,
  ],
  templateUrl: './combat-bar.html',
  styleUrl: './combat-bar.scss',
})
export class CombatBar {
  readonly encounter = input.required<Encounter>();
  readonly busy = input(false);
  /** How many questions about the rolls that wait for their roller are open: a chip beside the combat's title. */
  readonly pending = input(0);
  protected readonly pendingText = computed(() => pendingCountText(this.pending()));
  /** Only "Encerrar combate" and its question: the phone's page end (E6-12). */
  readonly endOnly = input(false);
  /** What the turn still owes ("Falta aplicar 5 de dano"), or `null`. */
  readonly pendingNote = input<string | null>(null);
  /** An opportunity attack waits for an answer ("Esperando a sua reação: Goblin 2"): the turn does not pass. */
  readonly waitNote = input('');
  /** Why "Próximo turno" waits, written under it ("Responda ao pedido abaixo para seguir."), or `''`. */
  readonly waitWhy = input('');
  /** "Restam 2 ações lendárias: ...": under "Próximo turno". */
  readonly legendary = input('');
  /** The name of the character the master acts for (C8-01): the bar says "Você age por …" and its one filled button ends that turn. */
  readonly actingFor = input('');
  protected readonly nextLabel = computed(() =>
    this.actingFor() ? `Encerrar o turno de ${this.actingFor()}` : 'Próximo turno',
  );
  /** The player character on turn who has not acted: passing their turn asks first (never while the master acts for one). */
  protected readonly idle = computed(() =>
    this.actingFor() ? '' : unactedPlayerOnTurn(this.encounter()),
  );
  /** The combat is played without a map (RN-25): the pill says it, and the end of the combat leaves no tokens behind. */
  readonly theatre = input(false);
  /** "Próximo turno": ends the turn of whoever is on turn; `true` when the
   * master passes it although a damage waits (it is discarded). */
  readonly next = output<boolean>();
  /** "Encerrar combate", confirmed. */
  readonly end = output<void>();
  /** "Começar o combate" (setup, everyone has rolled): the first turn begins. */
  readonly begin = output<void>();
  /** "Cancelar combate" in setup, confirmed. */
  readonly cancelCombat = output<void>();

  protected readonly confirming = signal(false);
  /** The question "Cancelar o combate?" of the setup is open. */
  protected readonly cancelling = signal(false);
  private readonly cancelButton = viewChild('safe', { read: ElementRef<HTMLButtonElement> });

  /** Setup: someone has no initiative yet, so "Começar o combate" waits (it keeps its focus stop and points at the reason). */
  protected readonly ready = computed(
    () => missingInitiative(this.encounter().combatants).length === 0,
  );
  protected readonly setup = computed(() => this.encounter().status === EncounterStatus.SETUP);
  protected readonly turnKey = computed(
    () => `${this.encounter().currentCombatantId}:${this.encounter().round}`,
  );
  protected readonly banner = computed(() => turnBanner(this.encounter()));
  protected readonly round = computed(() => roundLabel(this.encounter().round));
  protected readonly initial = computed(() => combatantInitial(this.banner().who?.label ?? ''));
  protected readonly npc = computed(() => {
    const who = this.banner().who;
    return !who || (!isPlayer(who) && !isCreature(who));
  });
  protected readonly creature = computed(() => {
    const who = this.banner().who;
    return !!who && isCreature(who);
  });
  protected readonly nextLine = computed(() => {
    const after = this.banner().after;
    return after ? `Em seguida: ${after.name}` : '';
  });

  private readonly injector = inject(Injector);

  protected ask(): void {
    this.confirming.set(true);
    // The confirmation opens on the safe button (a stray Enter must not end
    // a combat).
    afterNextRender(() => this.cancelButton()?.nativeElement.focus(), { injector: this.injector });
  }

  protected cancel(): void {
    this.confirming.set(false);
    this.cancelling.set(false);
  }

  protected start(): void {
    if (this.ready() && !this.busy()) {
      this.begin.emit();
    }
  }

  protected askCancel(): void {
    this.cancelling.set(true);
    afterNextRender(() => this.cancelButton()?.nativeElement.focus(), { injector: this.injector });
  }

  protected confirmCancel(): void {
    this.cancelling.set(false);
    this.cancelCombat.emit();
  }

  protected confirmEnd(): void {
    this.confirming.set(false);
    this.end.emit();
  }
}
