import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';

import {
  type SolveChoice,
  type SolveDraft,
  MESSAGE_MAX,
  NO_SOLVE,
  textLength,
} from '../../../core/puzzles/puzzle-draft';
import {
  type ClueChoice,
  type DoorChoice,
  type MapChoice,
  type PointChoice,
  SolveTargets,
} from '../../../core/puzzles/solve-targets';
import { DoorCrop } from '../../../shared/door-crop/door-crop';
import { type PickOption, PickGroup } from './pick-group';

const CHOICES: readonly PickOption<SolveChoice>[] = [
  { value: 'notify', title: 'Só me avisar' },
  { value: 'door', title: 'Abrir uma porta' },
  { value: 'point', title: 'Revelar um ponto do mapa' },
  { value: 'clue', title: 'Revelar uma pista' },
];

type Load = 'loading' | 'ready' | 'error';

/**
 * "Ao resolver" of the three forms (E10-06 state 2, Q79): what happens when the players solve it. "Só me avisar" is the
 * default; the others name a target the master picks from reads that already exist: a map and a door of it (the server opens a
 * door in any state, the puzzle is its key), a map and a point (it appears on the players' map), or a clue of a scene (it goes
 * to the character whose move solved it). The master is always told, and the app never rolls dice: the line under the choices
 * says so. The optional message is what the players read once it is solved.
 */
@Component({
  selector: 'app-solve-field',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DoorCrop, MatFormFieldModule, MatInputModule, PickGroup],
  providers: [SolveTargets],
  template: `
    <section class="sf" aria-labelledby="solve-title">
      <h3 class="sf__title" id="solve-title">Ao resolver</h3>
      <app-pick-group legend="O que acontece ao resolver" [hideLegend]="true" layout="rows" [options]="choices" [value]="solve().choice" (valueChange)="choose($event)" />

      @switch (solve().choice) {
        @case ('door') {
          @if (load() === 'loading') {
            <p class="sf__note" role="status">Procurando as portas dos mapas...</p>
          } @else if (load() === 'error') {
            <p class="sf__note sf__note--bad" role="alert">Não deu para ler os mapas. Volte e tente de novo.</p>
          } @else if (maps().length === 0) {
            <p class="sf__note">A campanha ainda não tem mapas. Crie um mapa e pinte uma porta nele.</p>
          } @else {
            <mat-form-field appearance="outline" subscriptSizing="dynamic" class="sf__field">
              <mat-label>Mapa</mat-label>
              <select matNativeControl [value]="solve().mapId" (change)="pickMap($any($event.target).value, 'door')">
                <option value="" [selected]="solve().mapId === ''">Escolha um mapa</option>
                @for (m of maps(); track m.id) {
                  <option [value]="m.id" [selected]="m.id === solve().mapId">{{ m.name }}</option>
                }
              </select>
            </mat-form-field>
            @if (solve().mapId !== '') {
              @if (doors().length === 0) {
                <p class="sf__note">Este mapa não tem portas. Pinte uma porta no editor do mapa.</p>
              } @else {
                <mat-form-field appearance="outline" subscriptSizing="dynamic" class="sf__field">
                  <mat-label>Qual porta</mat-label>
                  <select matNativeControl [value]="doorKey()" (change)="pickDoor($any($event.target).value)">
                    <option value="" [selected]="doorKey() === ''">Escolha uma porta</option>
                    @for (d of doors(); track d.col * 1000 + d.row) {
                      <option [value]="d.col + ',' + d.row" [selected]="d.col + ',' + d.row === doorKey()">{{ d.label }}</option>
                    }
                  </select>
                </mat-form-field>
                @if (solve().col >= 0) {
                  <app-door-crop [campaignId]="campaignId()" [mapId]="solve().mapId" [col]="solve().col" [row]="solve().row" />
                }
              }
            }
          }
        }
        @case ('point') {
          @if (load() === 'loading') {
            <p class="sf__note" role="status">Procurando os pontos dos mapas...</p>
          } @else if (load() === 'error') {
            <p class="sf__note sf__note--bad" role="alert">Não deu para ler os mapas. Volte e tente de novo.</p>
          } @else if (maps().length === 0) {
            <p class="sf__note">A campanha ainda não tem mapas. Crie um mapa e ponha um ponto nele.</p>
          } @else {
            <mat-form-field appearance="outline" subscriptSizing="dynamic" class="sf__field">
              <mat-label>Mapa</mat-label>
              <select matNativeControl [value]="solve().mapId" (change)="pickMap($any($event.target).value, 'point')">
                <option value="" [selected]="solve().mapId === ''">Escolha um mapa</option>
                @for (m of maps(); track m.id) {
                  <option [value]="m.id" [selected]="m.id === solve().mapId">{{ m.name }}</option>
                }
              </select>
            </mat-form-field>
            @if (solve().mapId !== '') {
              @if (points().length === 0) {
                <p class="sf__note">Este mapa não tem pontos. Ponha um ponto no editor do mapa.</p>
              } @else {
                <mat-form-field appearance="outline" subscriptSizing="dynamic" class="sf__field">
                  <mat-label>Qual ponto</mat-label>
                  <select matNativeControl [value]="solve().pointId" (change)="patch({ pointId: $any($event.target).value })">
                    <option value="" [selected]="solve().pointId === ''">Escolha um ponto</option>
                    @for (p of points(); track p.id) {
                      <option [value]="p.id" [selected]="p.id === solve().pointId">{{ p.name }}</option>
                    }
                  </select>
                </mat-form-field>
              }
            }
          }
        }
        @case ('clue') {
          @if (load() === 'loading') {
            <p class="sf__note" role="status">Procurando as pistas das cenas...</p>
          } @else if (load() === 'error') {
            <p class="sf__note sf__note--bad" role="alert">Não deu para ler as pistas. Volte e tente de novo.</p>
          } @else if (clues().length === 0) {
            <p class="sf__note">Nenhuma cena da campanha tem pistas ainda. Escreva uma no editor do mapa.</p>
          } @else {
            <mat-form-field appearance="outline" subscriptSizing="dynamic" class="sf__field">
              <mat-label>Qual pista</mat-label>
              <select matNativeControl [value]="solve().clueId" (change)="patch({ clueId: $any($event.target).value })">
                <option value="" [selected]="solve().clueId === ''">Escolha uma pista</option>
                @for (c of clues(); track c.id) {
                  <option [value]="c.id" [selected]="c.id === solve().clueId">{{ c.pointName }}: {{ shorten(c.text) }}</option>
                }
              </select>
            </mat-form-field>
            <p class="sf__note">A pista vai só para o jogador do personagem que resolver.</p>
          }
        }
      }
      @if (error()) {
        <p class="sf__note sf__note--bad" role="alert">{{ error() }}</p>
      }

      <mat-form-field appearance="outline" subscriptSizing="dynamic" class="sf__field" [class.field-bad]="messageError()">
        <mat-label>Mensagem para os jogadores (opcional)</mat-label>
        <textarea matInput rows="2" [value]="solve().message" (input)="patch({ message: $any($event.target).value })"></textarea>
        @if (messageError()) {
          <mat-hint class="field-error" role="alert">{{ messageError() }}</mat-hint>
        }
        @if (!messageError()) {
          <mat-hint>Eles leem quando resolverem. Sem texto, leem uma linha pronta.</mat-hint>
        }
        @if (!messageError()) {
          <mat-hint align="end">{{ length(solve().message) }}&nbsp;de&nbsp;{{ max }}</mat-hint>
        }
      </mat-form-field>
      <p class="sf__note">O mestre é sempre avisado. Resolver não rola dado nenhum.</p>
    </section>
  `,
  styleUrl: './solve-field.scss',
})
export class SolveField implements OnInit {
  private readonly targets = inject(SolveTargets);
  protected readonly choices = CHOICES;
  protected readonly max = MESSAGE_MAX;

  readonly campaignId = input.required<string>();
  readonly solve = input.required<SolveDraft>();
  /** What is wrong with the target ("Escolha a porta que se abre."), after a try to save. */
  readonly error = input('');
  readonly messageError = input('');
  readonly solveChange = output<SolveDraft>();

  protected readonly load = signal<Load>('loading');
  protected readonly maps = signal<readonly MapChoice[]>([]);
  protected readonly doors = signal<readonly DoorChoice[]>([]);
  protected readonly points = signal<readonly PointChoice[]>([]);
  protected readonly clues = signal<readonly ClueChoice[]>([]);
  private cluesLoaded = false;

  ngOnInit(): void {
    this.targets.use(this.campaignId());
    void this.start();
  }

  private async start(): Promise<void> {
    try {
      this.maps.set(await this.targets.maps());
      await this.loadFor(this.solve());
      this.load.set('ready');
    } catch {
      this.load.set('error');
    }
  }

  /** What the current choice needs: a map's doors or points, or the clues. */
  private async loadFor(solve: SolveDraft): Promise<void> {
    this.chosenMap = solve.mapId;
    if (solve.choice === 'door' && solve.mapId !== '') {
      const doors = await this.targets.doors(solve.mapId);
      if (solve.mapId === this.chosenMap) {
        this.doors.set(doors);
      }
    } else if (solve.choice === 'point' && solve.mapId !== '') {
      const points = await this.targets.points(solve.mapId);
      if (solve.mapId === this.chosenMap) {
        this.points.set(points);
      }
    } else if (solve.choice === 'clue' && !this.cluesLoaded) {
      this.clues.set(await this.targets.clues());
      this.cluesLoaded = true;
    }
  }

  protected doorKey(): string {
    const s = this.solve();
    return s.col >= 0 && s.row >= 0 ? `${s.col},${s.row}` : '';
  }

  protected patch(partial: Partial<SolveDraft>): void {
    this.solveChange.emit({ ...this.solve(), ...partial });
  }

  protected choose(choice: SolveChoice): void {
    // The message stays: it is the master's words, whatever the action.
    const next: SolveDraft = { ...NO_SOLVE, choice, message: this.solve().message };
    this.solveChange.emit(next);
    this.load.set('loading');
    this.loadFor(next).then(
      () => this.load.set('ready'),
      () => this.load.set('error'),
    );
  }

  /** The map the master chose last: an answer for another map (a slower read) is never shown. */
  private chosenMap = '';

  protected pickMap(mapId: string, kind: 'door' | 'point'): void {
    this.chosenMap = mapId;
    // The lists of the map before are gone at once, so a door or a point of another map is never on offer.
    this.doors.set([]);
    this.points.set([]);
    this.solveChange.emit({ ...this.solve(), mapId, col: -1, row: -1, pointId: '' });
    if (mapId === '') {
      return;
    }
    if (kind === 'door') {
      this.targets.doors(mapId).then(
        (d) => mapId === this.chosenMap && this.doors.set(d),
        () => this.load.set('error'),
      );
    } else {
      this.targets.points(mapId).then(
        (p) => mapId === this.chosenMap && this.points.set(p),
        () => this.load.set('error'),
      );
    }
  }

  protected pickDoor(key: string): void {
    const [col, row] = key === '' ? [-1, -1] : key.split(',').map(Number);
    this.patch({ col: col ?? -1, row: row ?? -1 });
  }

  protected length(text: string): number {
    return textLength(text.trim());
  }

  /** A clue is a line of up to 500 letters: the list shows its first words. */
  protected shorten(text: string): string {
    return text.length > 60 ? `${text.slice(0, 57)}…` : text;
  }
}
