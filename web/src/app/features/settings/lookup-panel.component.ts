// web/src/app/features/settings/lookup-panel.component.ts
// The reusable admin panel for managing a single lookup list (property types
// or areas). Both lists share the same shape and rules — that is why the
// panel is written once and instantiated twice.

import { Component, Input, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule, FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { LookupsService } from '../../api/api/lookups.service';
import { Lookup } from '../../api/model/lookup.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { WesternDigitsDirective } from '../../shared/western-digits.directive';
import { CloseOnEscapeDirective } from '../../shared/close-on-escape.directive';

type Entity = 'property-types' | 'areas';

@Component({
  selector: 'app-lookup-panel',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule, WesternDigitsDirective, CloseOnEscapeDirective],
  template: `
    <div class="card blueprint elev-sm">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
      <header class="pms-view-head">
        <div>
          <h2>{{ heading }}</h2>
        </div>
      </header>

      <form (ngSubmit)="add()" class="pms-inline-form">
        <input class="input" type="text" [formControl]="labelCtrl" appWesternDigits
               [placeholder]="msgs.lookupLabel" />
        <button type="submit" class="btn btn-primary" [disabled]="labelCtrl.invalid || saving()">
          {{ saving() ? msgs.loading : msgs.lookupAdd }}
        </button>
      </form>
      @if (fieldError(); as m) { <div class="pms-field-error">{{ m }}</div> }

      @if (errorMessage(); as msg) {
        <div class="pms-error-banner">{{ msg }}</div>
      }

      @if (items().length === 0) {
        <p class="pms-empty">{{ msgs.lookupEmpty }}</p>
      } @else {
        <table class="table">
          <thead>
            <tr>
              <th>{{ msgs.lookupLabel }}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            @for (it of items(); track it.id) {
              <tr>
                <td>
                  @if (editingId() === it.id) {
                    <input class="input" type="text" [formControl]="editCtrl" appWesternDigits />
                  } @else {
                    {{ it.label }}
                  }
                </td>
                <td>
                  @if (editingId() === it.id) {
                    <button type="button" class="btn btn-secondary" (click)="saveEdit(it)">{{ msgs.save }}</button>
                    <button type="button" class="btn btn-secondary" (click)="cancelEdit()">{{ msgs.cancel }}</button>
                  } @else {
                    <button type="button" class="btn btn-secondary" (click)="startEdit(it)">{{ msgs.lookupRename }}</button>
                    <button type="button" class="btn btn-secondary" (click)="askRemove(it)">{{ msgs.lookupRemove }}</button>
                  }
                </td>
              </tr>
            }
          </tbody>
        </table>
      }
    </div>

    @if (confirmRemove(); as it) {
      <div class="pms-modal-backdrop" (click)="cancelRemove()" [pmsCloseOnEscape]="cancelRemove">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.lookupRemove }}</h2>
          <p>{{ format(msgs.lookupConfirmRemove, { name: it.label }) }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelRemove()">{{ msgs.cancel }}</button>
            <button type="button" class="btn btn-primary" (click)="doRemove(it)" [disabled]="removing()">
              {{ msgs.lookupRemove }}
            </button>
          </div>
        </div>
      </div>
    }
  `,
  styles: [`
    .pms-inline-form { display: flex; gap: .5rem; align-items: flex-end; margin-block-end: 1rem; }
  `],
})
export class LookupPanelComponent implements OnInit {
  @Input({ required: true }) entity!: Entity;
  @Input({ required: true }) heading!: string;

  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;

  private readonly lookups = inject(LookupsService);

  protected readonly items = signal<Lookup[]>([]);
  protected readonly saving = signal(false);
  protected readonly removing = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly fieldError = signal<string | null>(null);
  protected readonly editingId = signal<string | null>(null);
  protected readonly confirmRemove = signal<Lookup | null>(null);

  protected readonly labelCtrl = new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(60)] });
  protected readonly editCtrl = new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(60)] });

  ngOnInit(): void {
    this.refresh();
  }

  private refresh(): void {
    const list$ = this.entity === 'property-types' ? this.lookups.listPropertyTypes('body') : this.lookups.listAreas('body');
    list$.subscribe({
      next: (items) => this.items.set(items),
      error: () => this.items.set([]),
    });
  }

  protected add(): void {
    if (this.labelCtrl.invalid) {
      this.labelCtrl.markAsTouched();
      return;
    }
    this.saving.set(true);
    this.errorMessage.set(null);
    this.fieldError.set(null);
    const label = this.labelCtrl.value.trim();
    const obs = this.entity === 'property-types'
      ? this.lookups.createPropertyType({ lookupWrite: { label } }, 'body')
      : this.lookups.createArea({ lookupWrite: { label } }, 'body');
    obs.subscribe({
      next: () => {
        this.saving.set(false);
        this.labelCtrl.reset('');
        this.refresh();
      },
      error: (err: ApiError) => {
        this.saving.set(false);
        if (err.code === 'conflict') {
          this.fieldError.set(this.msgs.lookupDuplicate);
        } else {
          this.errorMessage.set(err.message || this.msgs.internalError);
        }
      },
    });
  }

  protected startEdit(it: Lookup): void {
    this.editingId.set(it.id);
    this.editCtrl.setValue(it.label);
  }

  protected cancelEdit(): void {
    this.editingId.set(null);
  }

  protected saveEdit(it: Lookup): void {
    if (this.editCtrl.invalid) return;
    const label = this.editCtrl.value.trim();
    const obs = this.entity === 'property-types'
      ? this.lookups.renamePropertyType({ lookupId: it.id, lookupWrite: { label } }, 'body')
      : this.lookups.renameArea({ lookupId: it.id, lookupWrite: { label } }, 'body');
    obs.subscribe({
      next: () => {
        this.editingId.set(null);
        this.refresh();
      },
      error: (err: ApiError) => {
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  protected askRemove(it: Lookup): void {
    this.confirmRemove.set(it);
  }

  protected cancelRemove(): void {
    this.confirmRemove.set(null);
  }

  protected doRemove(it: Lookup): void {
    this.removing.set(true);
    const obs = this.entity === 'property-types'
      ? this.lookups.deletePropertyType({ lookupId: it.id }, 'body')
      : this.lookups.deleteArea({ lookupId: it.id }, 'body');
    obs.subscribe({
      next: () => {
        this.removing.set(false);
        this.confirmRemove.set(null);
        this.refresh();
      },
      error: (err: ApiError) => {
        this.removing.set(false);
        this.confirmRemove.set(null);
        if (err.code === 'in_use') {
          this.errorMessage.set(err.message || this.msgs.lookupInUseSingular);
        } else {
          this.errorMessage.set(err.message || this.msgs.internalError);
        }
      },
    });
  }
}
