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
  isPlayer,
  roundLabel,
  turnBanner,
} from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { NextTurn } from './next-turn';
import { TheatrePill } from '../theatre/theatre-pill';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';

/**
 * The master's combat bar (E6-04, E6-11, E6-12, E6-16): which combat, whose
 * turn, and the two buttons that move it. In SETUP it only says "Iniciativa"
 * and what to do. Running, "Próximo turno" is the page's one filled button
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
  /** Only "Encerrar combate" and its question: the phone's page end (E6-12). */
  readonly endOnly = input(false);
  /** What the turn still owes ("Falta aplicar 5 de dano"), or `null`. */
  readonly pendingNote = input<string | null>(null);
  /** An opportunity attack waits for an answer ("Esperando a sua reação: Goblin 2"): the turn does not pass. */
  readonly waitNote = input('');
  /** The combat is played without a map (RN-25): the pill says it, and the end of the combat leaves no tokens behind. */
  readonly theatre = input(false);
  /** "Próximo turno": ends the turn of whoever is on turn; `true` when the
   * master passes it although a damage waits (it is discarded). */
  readonly next = output<boolean>();
  /** "Encerrar combate", confirmed. */
  readonly end = output<void>();

  protected readonly confirming = signal(false);
  private readonly cancelButton = viewChild('safe', { read: ElementRef<HTMLButtonElement> });

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
  }

  protected confirmEnd(): void {
    this.confirming.set(false);
    this.end.emit();
  }
}
