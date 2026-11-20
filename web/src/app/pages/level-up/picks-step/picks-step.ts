import { Component, computed, input } from '@angular/core';

import type { PickItem } from '../../../core/levelup/levelup-flow';
import { ChoiceGroups } from '../../../shared/choice-groups/choice-groups';
import { PickList } from '../pick-list/pick-list';
import { LevelUpSession } from '../level-up-session';
import { SwapPicker } from '../swap-picker/swap-picker';

/**
 * Step "Escolhas" of the guided level-up (MR-040): what a level offers that is neither
 * an ability nor a spell, in the order the rules need it. The subclass comes first, since
 * it can add cantrips, skills and feature options to the level; then each feature's options
 * (a fighting style, a metamagic), the new skills and the expertise. It exists only for a
 * class and level that offer one of them (the step list drops it otherwise). The artboards
 * of E8-15 do not draw it: it is the same picker as the Magias step, one panel per choice.
 */
@Component({
  selector: 'app-picks-step',
  imports: [ChoiceGroups, PickList, SwapPicker],
  template: `
    @if (s().options.lateChoices.length > 0) {
      <section class="choices" id="pick-late" aria-labelledby="late-title">
        <h2 class="choices__title" id="late-title">Escolhas que ficaram para trás</h2>
        <p class="choices__lead">
          Estas escolhas ficaram abertas em níveis anteriores. Faça todas antes de subir de nível: o que já foi escolhido não muda.
        </p>
        <app-choice-groups
          [groups]="s().options.lateChoices"
          [onlyOpen]="true"
          [offerCantrips]="false"
          (selected)="s().draft.selectLate($event.choice, $event.optionKeys)"
          (textEdited)="s().draft.editLateText($event.choice, $event.n, $event.text)"
        />
      </section>
    }
    @if (s().options.newChoices.length > 0) {
      <section class="choices" id="pick-new" aria-labelledby="new-title">
        <h2 class="choices__title" id="new-title">Escolhas do nível {{ s().options.toLevel }}</h2>
        <app-choice-groups
          [groups]="s().options.newChoices"
          [onlyOpen]="true"
          [offerCantrips]="false"
          (selected)="s().draft.selectNew($event.choice, $event.optionKeys)"
          (textEdited)="s().draft.editNewText($event.choice, $event.n, $event.text)"
        />
      </section>
    }
    @if (s().options.subclassDue) {
      <app-pick-list
        pickId="subclass"
        title="Subclasse"
        lead="Escolha a subclasse que o personagem segue a partir deste nível."
        noun="subclasse"
        nounMany="subclasses"
        [items]="subclasses()"
        [picked]="subclassPicked()"
        [count]="1"
        (pick)="s().draft.setSubclass($event)"
      />
    }
    @for (g of groups(); track g.id) {
      <app-pick-list
        [pickId]="g.id"
        [title]="g.title"
        [lead]="g.lead"
        noun="opção"
        nounMany="opções"
        [items]="g.items"
        [picked]="g.picked"
        [count]="g.choose"
        [fullNote]="'Limite de ' + g.choose + ' opções'"
        (pick)="s().draft.toggleFeature($event, g.keys, g.choose)"
      />
    }
    @if (!s().options.abilityScoreImprovement && s().draft.swap.groups().length > 0) {
      <app-swap-picker [swap]="s().draft.swap" />
    }
    @if (s().draft.skillsAsked() > 0) {
      <app-pick-list
        pickId="skills"
        [title]="s().draft.skillsAsked() === 1 ? 'Perícia nova' : 'Perícias novas'"
        [lead]="'Escolha ' + s().draft.skillsAsked() + ' ' + (s().draft.skillsAsked() === 1 ? 'perícia' : 'perícias') + ' em que o personagem ainda não é treinado.'"
        noun="perícia"
        nounMany="perícias"
        [items]="s().draft.skillItems()"
        [picked]="s().draft.skills()"
        [count]="s().draft.skillsAsked()"
        [fullNote]="'Limite de ' + s().draft.skillsAsked() + ' perícias novas'"
        (pick)="s().draft.toggleSkill($event)"
      />
    }
    @if (s().draft.instrumentAsked() > 0) {
      <app-pick-list
        pickId="instrument"
        title="Instrumento musical"
        lead="Escolha 1 instrumento. A tabela de multiclasse do Bardo dá um instrumento à escolha."
        noun="instrumento"
        nounMany="instrumentos"
        [items]="s().draft.instrumentItems()"
        [picked]="instrumentPicked()"
        [count]="1"
        (pick)="s().draft.toggleInstrument($event)"
      />
    }
    @if (s().draft.expertiseAsked() > 0) {
      <app-pick-list
        pickId="expertise"
        title="Especialização"
        [lead]="'Escolha ' + s().draft.expertiseAsked() + ' ' + (s().draft.expertiseAsked() === 1 ? 'perícia' : 'perícias') + ' em que o personagem já é treinado, para dobrar o bônus de proficiência.'"
        noun="perícia"
        nounMany="perícias"
        [items]="s().draft.expertiseItems()"
        [picked]="s().draft.expertise()"
        [count]="s().draft.expertiseAsked()"
        [fullNote]="'Limite de ' + s().draft.expertiseAsked() + ' especializações'"
        (pick)="s().draft.toggleExpertise($event)"
      />
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-4);
    }

    .choices__title {
      margin: 0 0 var(--mr-space-1);
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-21);
      font-weight: 700;
      line-height: 26px;
    }

    .choices__lead {
      margin: 0 0 var(--mr-space-3);
      color: var(--mr-ink-muted);
    }

    // The first missing choice, after a tap on the dashed "Próximo".
    .choices[data-attn] {
      outline: 2px solid var(--mr-warning-ink);
      outline-offset: 6px;
      border-radius: var(--mr-radius-md);
    }
  `,
})
export class PicksStep {
  readonly s = input.required<LevelUpSession>();

  protected readonly subclasses = computed<PickItem[]>(() =>
    this.s().options.subclasses.map((c) => ({ key: c.key, name: c.namePt, sub: '' })),
  );
  protected readonly instrumentPicked = computed(
    () => new Set(this.s().draft.instrument() ? [this.s().draft.instrument()] : []),
  );
  protected readonly subclassPicked = computed(
    () => new Set(this.s().draft.subclassKey() ? [this.s().draft.subclassKey()] : []),
  );

  /** One group per feature that offers options, with the picks that belong to it. */
  protected readonly groups = computed(() => {
    const d = this.s().draft;
    const chosen = d.features();
    return d.totals().featureChoices.map((f, i) => {
      const takable = f.options.map((o) => ({ key: o.key, name: o.namePt, sub: '' }));
      const keys = takable.map((o) => o.key);
      // An option the character cannot take now stays on the list, off, with why (an invocation whose prerequisite is unmet).
      const items = [
        ...takable,
        ...f.blocked.map((b) => ({
          key: b.option?.key ?? '',
          name: b.option?.namePt ?? '',
          sub: '',
          disabled: b.reasonPt,
        })),
      ];
      return {
        id: `feature-${i}`,
        title: f.feature?.namePt ?? 'Característica',
        lead: f.choose === 1 ? 'Escolha 1 opção.' : `Escolha ${f.choose} opções.`,
        items,
        keys,
        choose: f.choose,
        picked: new Set(keys.filter((k) => chosen.has(k))),
      };
    });
  });
}
