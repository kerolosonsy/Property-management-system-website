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

@Component({
  selector: 'app-custom-fields',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, WesternDigitsDirective],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.customFields }}</h1>
          <div class="pms-crumb">{{ msgs.settings }} / {{ msgs.customFields }}</div>
        </div>
      </header>

      <div class="card blueprint elev-sm">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
        <p class="pms-note">{{ msgs.customFieldSensitiveNote }}</p>

        <h2>{{ editing() ? msgs.customFieldEdit : msgs.customFieldAdd }}</h2>
        <form [formGroup]="form" (ngSubmit)="onSubmit()">
          <div class="field pms-field">
            <label for="label">{{ msgs.customFieldLabel }}</label>
            <input id="label" class="input" type="text" formControlName="label" appWesternDigits />
            @if (form.controls.label.touched && form.controls.label.invalid) {
              <div class="pms-field-error">{{ msgs.customFieldLabelLength }}</div>
            }
            @if (fieldError('label'); as m) { <div class="pms-field-error">{{ m }}</div> }
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
            @if (fieldError('fieldType'); as m) { <div class="pms-field-error">{{ m }}</div> }
          </div>

          @if (form.controls.fieldType.value === 'text') {
            <div class="field pms-field pms-field-checkbox">
              <input id="isSensitive" type="checkbox" formControlName="isSensitive" />
              <label for="isSensitive">{{ msgs.customFieldSensitive }}</label>
            </div>
          }

          @if (form.controls.fieldType.value === 'dropdown' || form.controls.fieldType.value === 'multiselect') {
            <div class="field pms-field">
              <label>{{ msgs.customFieldChoices }}</label>
              @for (ctrl of choicesControls(); track $index) {
                <div class="pms-choice-row">
                  <input class="input" type="text" [formControl]="ctrl" appWesternDigits />
                  <button type="button" class="btn btn-secondary" (click)="removeChoice($index)">{{ msgs.customFieldChoiceRemove }}</button>
                </div>
              }
              <button type="button" class="btn btn-secondary" (click)="addChoice()">{{ msgs.customFieldChoiceAdd }}</button>
            </div>
          }

          @if (errorMessage(); as msg) {
            <div class="pms-error-banner">{{ msg }}</div>
          }

          <div style="display: flex; gap: .75rem;">
            <button type="submit" class="btn btn-primary" [disabled]="submitting() || form.invalid">
              {{ submitting() ? msgs.loading : msgs.save }}
            </button>
            @if (editing()) {
              <button type="button" class="btn btn-secondary" (click)="cancelEdit()">{{ msgs.cancel }}</button>
            }
          </div>
        </form>
      </div>

      <div class="card blueprint elev-sm">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
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
                    <button type="button" class="btn btn-secondary" (click)="startEdit(f)">{{ msgs.edit }}</button>
                    <button type="button" class="btn btn-secondary" (click)="askRemove(f)">{{ msgs.customFieldRemove }}</button>
                  </td>
                </tr>
              }
            </tbody>
          </table>
        }
      </div>
    </section>

    @if (confirmRemove(); as f) {
      <div class="pms-modal-backdrop" (click)="cancelRemove()">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.customFieldRemove }}</h2>
          <p>{{ f.label }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelRemove()">{{ msgs.cancel }}</button>
            <button type="button" class="btn btn-primary" (click)="doRemove(f)" [disabled]="removing()">{{ msgs.customFieldRemove }}</button>
          </div>
        </div>
      </div>
    }
  `,
  styles: [`
    .pms-choice-row { display: flex; gap: .5rem; align-items: center; margin-block-end: .5rem; }
  `],
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

  ngOnInit(): void {
    this.refresh();
  }

  protected typeLabel(t: CustomFieldType): string {
    switch (t) {
      case 'text': return this.msgs.customFieldTypeText;
      case 'dropdown': return this.msgs.customFieldTypeDropdown;
      case 'multiselect': return this.msgs.customFieldTypeMultiselect;
      case 'checkbox': return this.msgs.customFieldTypeCheckbox;
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
    this.choicesControls.update((arr) => [...arr, new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(60)] })]);
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
    const choices = this.choicesControls().map((c) => c.value).filter((s) => s.trim() !== '');
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
      const addChoices = choices.filter((label) => !priorIDs.has(label.trim() === label.trim() ? '' : ''));
      const _ = addChoices;
      // Build from the new label set directly — easier than diffing.
      this.service.updateCustomField({
        fieldId: editing.id,
        customFieldUpdate: {
          label: raw.label.trim(),
          removeChoiceIds,
          addChoices: choices,
        },
      }, 'body').subscribe({
        next: () => {
          this.submitting.set(false);
          this.cancelEdit();
          this.refresh();
        },
        error: (err: ApiError) => this.handleError(err),
      });
    } else {
      this.service.createCustomField({
        customFieldCreate: {
          label: raw.label.trim(),
          fieldType: raw.fieldType as CustomFieldType,
          isSensitive: raw.isSensitive,
          choices,
        },
      }, 'body').subscribe({
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
