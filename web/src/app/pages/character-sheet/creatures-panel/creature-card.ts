import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import {
  type CharacterCreature,
  CreatureSource,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import type { Creature } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { sourcePhrase } from '../../../core/creatures/creature-format';
import { joinDots, tight } from '../../../core/format/text';
import { metersText } from '../../../core/units';
import { FamiliarEyesButton } from '../../../shared/familiar-eyes/familiar-eyes-button';
import { CreatureArt } from '../../../shared/creatures/creature-art';
import { CreatureEdit, type EditMode } from '../../../shared/creatures/creature-edit';

/**
 * One creature of the character, as a card (E9-10, quadro 1): its picture, the
 * name the table gave it (a heading), "Corvo · Miúdo · Familiar de Pensantus",
 * three numbers (CA, PV "1 de 1", the speeds in metres) and the actions.
 *
 * The numbers come from the creature's stat block (`GetCreature`, read once per
 * kind) and its own hit points; until the block arrives the tiles show a dash.
 * Only the owner's player and the master get this card at all (RN-20): the
 * server answers `not_found` to everyone else, so the card never has to hide a
 * number.
 *
 * - "Ver a ficha" (named "Ver a ficha de Nanquim" for a screen reader): the stat block page. 48 px, outlined.
 * - "Renomear": the name, in place.
 * - "Dispensar": asks in place. The character's player and the master may dismiss any creature, a
 *   gift included (the server decides, `DismissCreature`).
 * - "Corrigir PV": the master's correction outside a combat (RN-02).
 * The focus goes back to the action that opened a question when it closes.
 */
@Component({
  selector: 'app-creature-card',
  imports: [
    CreatureArt,
    CreatureEdit,
    FamiliarEyesButton,
    MatButtonModule,
    MatIconModule,
    RouterLink,
  ],
  templateUrl: './creature-card.html',
  styleUrl: './creature-card.scss',
})
export class CreatureCard {
  private readonly client = inject(CreaturesClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly characterId = input.required<string>();
  readonly ownerName = input.required<string>();
  readonly creature = input.required<CharacterCreature>();
  readonly isMaster = input(false);
  /** A session is open: a familiar can be looked through ("Ver pelos olhos", MR-036). The player's, not the master's. */
  readonly live = input(false);
  /** The creature changed (renamed, dismissed, corrected): the panel reads the list again, and says how. */
  readonly changed = output<EditMode>();

  protected readonly seeable = computed(
    () => this.live() && !this.isMaster() && this.creature().source === CreatureSource.FAMILIAR,
  );
  protected readonly block = signal<Creature | null>(null);
  protected readonly mode = signal<EditMode | null>(null);

  protected readonly subtitle = computed(() => {
    const c = this.creature();
    return joinDots(
      [
        c.monsterNamePt,
        this.block()?.summary?.sizePt ?? '',
        sourcePhrase(c.source, this.ownerName()),
      ].filter((p) => p),
    );
  });

  protected readonly speed = computed(() => {
    const b = this.block();
    if (!b) {
      return { main: '—', extra: '' };
    }
    const main =
      b.speedWalkFt > 0
        ? metersText(b.speedWalkFt)
        : b.speedFlyFt > 0
          ? metersText(b.speedFlyFt)
          : '0 m';
    const extras: string[] = [];
    if (b.speedWalkFt > 0 && b.speedFlyFt > 0) {
      extras.push(`voo ${metersText(b.speedFlyFt)}`);
    }
    if (b.speedSwimFt > 0) {
      extras.push(`nada ${metersText(b.speedSwimFt)}`);
    }
    if (b.speedClimbFt > 0) {
      extras.push(`escala ${metersText(b.speedClimbFt)}`);
    }
    return { main: tight(main), extra: tight(extras.join(', ')) };
  });

  protected readonly link = computed(() => [
    '/campaigns',
    this.campaignId(),
    'characters',
    this.characterId(),
    'creatures',
    this.creature().id,
  ]);

  constructor() {
    effect(() => {
      const key = this.creature().monsterKey;
      const campaignId = this.campaignId();
      untracked(() => {
        this.client.statBlock(campaignId, key).then(
          (b) => this.block.set(b),
          () => this.block.set(null),
        );
      });
    });
  }

  protected open(mode: EditMode): void {
    this.mode.set(mode);
  }

  protected close(changed: boolean): void {
    const was = this.mode();
    this.mode.set(null);
    if (changed) {
      this.changed.emit(was ?? 'rename');
      return;
    }
    // Backing out: the focus goes back to the action that asked.
    afterNextRender(
      () => focusWithRing(this.host.nativeElement.querySelector<HTMLElement>(`.js-${was}`)),
      { injector: this.injector },
    );
  }
}
