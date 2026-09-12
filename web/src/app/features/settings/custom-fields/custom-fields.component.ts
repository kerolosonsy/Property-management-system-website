// web/src/app/features/settings/custom-fields/custom-fields.component.ts
// US7 — manage custom field definitions: define, rename, add/remove choices,
// delete. Sensitive fields are marked here at definition time and the screen
// states in Arabic that the four identifiers belong in a sensitive field.

import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { CustomFieldsService } from '../../../api/api/custom-fields.service';
import { CustomField } from '../../../api/model/custom-field.model';
import { CustomFieldType } from '../../../api/model/custom-field-type.model';
import { ARABIC_MESSAGES } from '../../../shared/messages';
import { ApiError } from '../../../core/api-error';
import { WesternDigitsDirective } from '../../../shared/western-digits.directive';
import { CloseOnEscapeDirective } from '../../../shared/close-on-escape.directive';

@Component({
  selector: 'app-custom-fields',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, WesternDigitsDirective, CloseOnEscapeDirective],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.customFields }}</h1>
          <div class="pms-crumb">{{ msgs.settings }} / {{ msgs.customFields }}</div>
        </div>
      </header>

      <!-- The editor card and its pinned foot sit in one wrapper so the bar
           releases once the operator scrolls on into the list below. -->
      <div>
        <div class="card blueprint elev-sm">
          <i class="corner tl"></i><i class="corner tr"></i> <i class="corner bl"></i
          ><i class="corner br"></i>
          <p class="pms-note">{{ msgs.customFieldSensitiveNote }}</p>

          <h2>{{ editing() ? msgs.customFieldEdit : msgs.customFieldAdd }}</h2>
          <form id="pms-custom-field-form" [formGroup]="form" (ngSubmit)="onSubmit()">
            <div class="pms-form-grid">
              <div class="field pms-field">
                <label for="label">{{ msgs.customFieldLabel }}</label>
                <input
                  id="label"
                  class="input"
                  type="text"
                  formControlName="label"
                  appWesternDigits
                />
                @if (form.controls.label.touched && form.controls.label.invalid) {
                  <div class="pms-field-error">{{ msgs.customFieldLabelLength }}</div>
                }
                @if (fieldError('label'); as m) {
                  <div class="pms-field-error">{{ m }}</div>
                }
              </div>

              <div class="field pms-field">
                <label for="fieldType">{{ msgs.customFieldType }}</label>
                <select id="fieldType" class="input" formControlName="fieldType">
                  <option [ngValue]="''" disabled>—</option>
                  <option [ngValue]="'text'">{{ msgs.customFieldTypeText }}</option>
                  <option [ngValue]="'dropdown'">{{ msgs.customFieldTypeDropdown }}</option>
                  <option [ngValue]="'multiselect'">{{ msgs.customFieldTypeMultiselect }}</option>
                  <option [ngValue]="'checkbox'">{{ msgs.customFieldTypeCheckbox }}</option>
                </select>
                @if (fieldError('fieldType'); as m) {
                  <div class="pms-field-error">{{ m }}</div>
                }
              </div>

              <div class="field pms-field pms-field-checkbox">
                <input
                  id="isSensitive"
                  type="checkbox"
                  formControlName="isSensitive"
                  [attr.aria-describedby]="sensitiveHintId()"
                  [disabled]="sensitiveDisabled()"
                />
                <label for="isSensitive">{{ msgs.customFieldSensitive }}</label>
                @if (sensitiveHint(); as h) {
                  <div [id]="sensitiveHintId()" class="pms-field-hint">{{ h }}</div>
                }
              </div>

              @if (
                form.controls.fieldType.value === 'dropdown' ||
                form.controls.fieldType.value === 'multiselect'
              ) {
                <div class="field pms-field pms-form-grid-full">
                  <label>{{ msgs.customFieldChoices }}</label>
                  @for (ctrl of choicesControls(); track $index) {
                    <div class="pms-choice-row">
                      <input class="input" type="text" [formControl]="ctrl" appWesternDigits />
                      <button
                        type="button"
                        class="btn btn-secondary"
                        (click)="removeChoice($index)"
                      >
                        {{ msgs.customFieldChoiceRemove }}
                      </button>
                    </div>
                  }
                  <button type="button" class="btn btn-secondary" (click)="addChoice()">
                    {{ msgs.customFieldChoiceAdd }}
                  </button>
                </div>
              }
            </div>

            @if (errorMessage(); as msg) {
              <div class="pms-error-banner">{{ msg }}</div>
            }
          </form>
        </div>

        <footer class="pms-view-foot">
          <button
            type="submit"
            class="btn btn-primary"
            [disabled]="submitting() || form.invalid"
            form="pms-custom-field-form"
          >
            {{ submitting() ? msgs.loading : msgs.save }}
          </button>
          @if (editing()) {
            <button type="button" class="btn btn-secondary" (click)="cancelEdit()">
              {{ msgs.cancel }}
            </button>
          }
        </footer>
      </div>

      <div class="card blueprint elev-sm">
        <i class="corner tl"></i><i class="corner tr"></i> <i class="corner bl"></i
        ><i class="corner br"></i>
        @if (items().length === 0) {
          <p class="pms-empty">{{ msgs.lookupEmpty }}</p>
        } @else {
          <table class="table">
            <thead>
              <tr>
                <th>{{ msgs.customFieldLabel }}</th>
                <th>{{ msgs.customFieldType }}</th>
                <th>{{ msgs.customFieldSensitive }}</th>
                <th>{{ msgs.customFieldChoices }}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              @for (f of items(); track f.id) {
                <tr>
                  <td>{{ f.label }}</td>
                  <td>{{ typeLabel(f.fieldType) }}</td>
                  <td>{{ f.isSensitive ? msgs.yes : msgs.no }}</td>
                  <td>
                    @for (c of f.choices; track c.id) {
                      <span class="pms-pill">{{ c.label }}</span>
                    }
                  </td>
                  <td>
                    <button type="button" class="btn btn-secondary" (click)="startEdit(f)">
                      {{ msgs.edit }}
                    </button>
                    <button type="button" class="btn btn-secondary" (click)="askRemove(f)">
                      {{ msgs.customFieldRemove }}
                    </button>
                  </td>
                </tr>
              }
            </tbody>
          </table>
        }
      </div>
    </section>

    @if (confirmRemove(); as f) {
      <div class="pms-modal-backdrop" (click)="cancelRemove()" [pmsCloseOnEscape]="cancelRemove">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.customFieldRemove }}</h2>
          <p>{{ f.label }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelRemove()">
              {{ msgs.cancel }}
            </button>
            <button
              type="button"
              class="btn btn-primary"
              (click)="doRemove(f)"
              [disabled]="removing()"
            >
              {{ msgs.customFieldRemove }}
            </button>
          </div>
        </div>
      </div>
    }
  `,
  styles: [
    `
      .pms-choice-row {
        display: flex;
        gap: 0.5rem;
        align-items: center;
        margin-block-end: 0.5rem;
      }
    `,
  ],
})
export class CustomFieldsComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;

  private readonly fb = inject(FormBuilder);
  private readonly service = inject(CustomFieldsService);

  protected readonly items = signal<CustomField[]>([]);
  protected readonly editing = signal<CustomField | null>(null);
  protected readonly confirmRemove = signal<CustomField | null>(null);
  protected readonly submitting = signal(false);
  protected readonly removing = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly choicesControls = signal<FormControl<string>[]>([]);
  private readonly fieldErrors = signal<Record<string, string>>({});

  protected readonly form = this.fb.nonNullable.group({
    label: ['', [Validators.required, Validators.maxLength(60)]],
    fieldType: ['' as CustomFieldType | '', [Validators.required]],
    isSensitive: [false],
  });

  protected fieldError(name: string): string | undefined {
    return this.fieldErrors()[name];
  }

  // Sensitivity is text-only and, once any value exists, fixed (FR-027s1, FR-027s5).
  // The checkbox is always rendered so the operator can see and reason about it;
  // when the rules disallow a change it is disabled with an Arabic hint that says
  // why. The hint id is fed into aria-describedby for screen readers.
  protected sensitiveHint(): string | null {
    const type = this.form.controls.fieldType.value;
    const editing = this.editing();
    if (type !== 'text') {
      return this.msgs.customFieldSensitiveTextOnly;
    }
    if (editing && (editing.valuesCount ?? 0) > 0) {
      return this.msgs.customFieldSensitiveFixed;
    }
    return null;
  }

  protected sensitiveHintId(): string {
    return 'pms-isSensitive-hint';
  }

  protected sensitiveDisabled(): boolean {
    const type = this.form.controls.fieldType.value;
    if (type !== 'text') {
      return true;
    }
    const editing = this.editing();
    if (editing && (editing.valuesCount ?? 0) > 0) {
      return true;
    }
    return false;
  }

  ngOnInit(): void {
    this.refresh();
    this.subscribeSensitiveReset();
  }

  protected typeLabel(t: CustomFieldType): string {
    switch (t) {
      case 'text':
        return this.msgs.customFieldTypeText;
      case 'dropdown':
        return this.msgs.customFieldTypeDropdown;
      case 'multiselect':
        return this.msgs.customFieldTypeMultiselect;
      case 'checkbox':
        return this.msgs.customFieldTypeCheckbox;
    }
    return t;
  }

  private refresh(): void {
    this.service.listCustomFields('body').subscribe({
      next: (items) => this.items.set(items),
      error: () => this.items.set([]),
    });
  }

  protected addChoice(): void {
    this.choicesControls.update((arr) => [
      ...arr,
      new FormControl('', {
        nonNullable: true,
        validators: [Validators.required, Validators.maxLength(60)],
      }),
    ]);
  }

  protected removeChoice(i: number): void {
    this.choicesControls.update((arr) => arr.filter((_, idx) => idx !== i));
  }

  protected startEdit(f: CustomField): void {
    this.editing.set(f);
    this.form.patchValue({
      label: f.label,
      fieldType: f.fieldType,
      isSensitive: f.isSensitive,
    });
    this.choicesControls.set(f.choices.map((c) => new FormControl(c.label, { nonNullable: true })));
    this.subscribeSensitiveReset();
  }

  private sensitiveResetSub: { unsubscribe(): void } | null = null;
  private subscribeSensitiveReset(): void {
    this.sensitiveResetSub?.unsubscribe();
    this.sensitiveResetSub = this.form.controls.fieldType.valueChanges.subscribe((t) => {
      // Only text fields may be sensitive. If the operator changed the type
      // away from text, clear isSensitive so the stored value matches what
      // the server will accept (the server refuses sensitive on non-text).
      if (t !== 'text') {
        this.form.controls.isSensitive.setValue(false, { emitEvent: false });
      }
    });
  }

  protected cancelEdit(): void {
    this.editing.set(null);
    this.form.reset({ label: '', fieldType: '', isSensitive: false });
    this.choicesControls.set([]);
  }

  protected onSubmit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.submitting.set(true);
    this.errorMessage.set(null);
    this.fieldErrors.set({});

    const raw = this.form.getRawValue();
    const choices = this.choicesControls()
      .map((c) => c.value)
      .filter((s) => s.trim() !== '');
    const editing = this.editing();

    if (editing) {
      // For an update, the server replaces the choice set when addChoices/removeChoiceIds
      // are sent. The simple path here: send back the full set so the operator sees
      // exactly what they expect. Removing is implicit because the previous choices
      // not in the new list will be pruned by removeChoiceIds.
      const priorIDs = new Set(editing.choices.map((c) => c.id));
      const newLabels = new Set(choices.map((s) => s.trim()));
      const removeChoiceIds = editing.choices
        .filter((c) => !newLabels.has(c.label))
        .map((c) => c.id);
      const addChoices = choices.filter(
        (label) => !priorIDs.has(label.trim() === label.trim() ? '' : ''),
      );
      const _ = addChoices;
      // Build from the new label set directly — easier than diffing.
      this.service
        .updateCustomField(
          {
            fieldId: editing.id,
            customFieldUpdate: {
              label: raw.label.trim(),
              removeChoiceIds,
              addChoices: choices,
            },
          },
          'body',
        )
        .subscribe({
          next: () => {
            this.submitting.set(false);
            this.cancelEdit();
            this.refresh();
          },
          error: (err: ApiError) => this.handleError(err),
        });
    } else {
      this.service
        .createCustomField(
          {
            customFieldCreate: {
              label: raw.label.trim(),
              fieldType: raw.fieldType as CustomFieldType,
              isSensitive: raw.isSensitive,
              choices,
            },
          },
          'body',
        )
        .subscribe({
          next: () => {
            this.submitting.set(false);
            this.cancelEdit();
            this.refresh();
          },
          error: (err: ApiError) => this.handleError(err),
        });
    }
  }

  protected askRemove(f: CustomField): void {
    this.confirmRemove.set(f);
  }
  protected cancelRemove(): void {
    this.confirmRemove.set(null);
  }
  protected doRemove(f: CustomField): void {
    this.removing.set(true);
    this.service.deleteCustomField({ fieldId: f.id }, 'body').subscribe({
      next: () => {
        this.removing.set(false);
        this.confirmRemove.set(null);
        this.refresh();
      },
      error: (err: ApiError) => {
        this.removing.set(false);
        this.confirmRemove.set(null);
        if (err.code === 'in_use') {
          this.errorMessage.set(err.message || this.msgs.customFieldInUsePlural);
        } else {
          this.errorMessage.set(err.message || this.msgs.internalError);
        }
      },
    });
  }

  private handleError(err: ApiError): void {
    this.submitting.set(false);
    if (err.code === 'in_use') {
      this.errorMessage.set(err.message || this.msgs.customFieldInUsePlural);
    } else if (err.code === 'conflict') {
      this.fieldErrors.set({ label: this.msgs.customFieldLabelTaken });
      this.errorMessage.set(err.message);
    } else if (err.fields) {
      this.fieldErrors.set(err.fields);
      this.errorMessage.set(err.message);
    } else {
      this.errorMessage.set(err.message || this.msgs.internalError);
    }
  }
}
