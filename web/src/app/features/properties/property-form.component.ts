// web/src/app/features/properties/property-form.component.ts
// US2 / US4 — add or modify a property. The mode is determined by the route:
// /properties/new creates, /properties/:id/edit updates with version concurrency.

import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { PropertiesService } from '../../api/api/properties.service';
import { LookupsService } from '../../api/api/lookups.service';
import { CustomFieldsService } from '../../api/api/custom-fields.service';
import { SessionService } from '../../core/session.service';
import { CustomField } from '../../api/model/custom-field.model';
import { CustomFieldType } from '../../api/model/custom-field-type.model';
import { Lookup } from '../../api/model/lookup.model';
import { ARABIC_MESSAGES } from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { WesternDigitsDirective } from '../../shared/western-digits.directive';

interface CustomFormState {
  field: CustomField;
  text: FormControl<string>;
  checked: FormControl<boolean | null>;
  choice: FormControl<string>;
  choices: FormControl<string[]>;
}

function emptyCustomFormState(field: CustomField): CustomFormState {
  return {
    field,
    text: new FormControl<string>('', { nonNullable: true }),
    checked: new FormControl<boolean | null>(null),
    choice: new FormControl<string>('', { nonNullable: true }),
    choices: new FormControl<string[]>([], { nonNullable: true }),
  };
}

function emptyCustomFormStateList(fields: CustomField[]): CustomFormState[] {
  return fields.map(emptyCustomFormState);
}

// re-export so other files can refer to CustomFieldType from this module
export type { CustomFieldType };

@Component({
  selector: 'app-property-form',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink, WesternDigitsDirective],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ isEdit() ? msgs.editProperty : msgs.addProperty }}</h1>
          @if (isEdit()) {
            <div class="pms-crumb">{{ msgs.properties }} / {{ initialName() }}</div>
          }
        </div>
        <div class="pms-toolbar-spacer"></div>
        <a [routerLink]="backLink()" class="btn btn-secondary">{{ msgs.cancel }}</a>
      </header>

      <div class="card blueprint elev-sm" style="max-inline-size: 48rem; margin-inline: auto;">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
        @if (errorMessage(); as msg) {
          <div class="pms-error-banner">{{ msg }}</div>
        }
        @if (versionMismatch(); as current) {
          <div class="pms-warning-banner">
            {{ msgs.propertyVersionNeeded }}
            <ul>
              <li>{{ msgs.propertyName }}: {{ current.name }}</li>
              <li>{{ msgs.propertyType }}: {{ current.propertyType.label }}</li>
              <li>{{ msgs.propertyArea }}: {{ current.area.label }}</li>
            </ul>
          </div>
        }

        <form [formGroup]="form" (ngSubmit)="onSubmit()" novalidate>
          <div class="field pms-field">
            <label for="name">{{ msgs.propertyName }}</label>
            <input id="name" class="input" type="text" formControlName="name" appWesternDigits />
            @if (form.controls.name.touched && form.controls.name.invalid) {
              <div class="pms-field-error">
                @if (form.controls.name.hasError('required')) { {{ msgs.requiredField }} }
                @else { {{ msgs.propertyNameLength }} }
              </div>
            }
            @if (fieldError('name'); as m) { <div class="pms-field-error">{{ m }}</div> }
          </div>

          <div class="field pms-field">
            <label for="propertyTypeId">{{ msgs.propertyType }}</label>
            <select id="propertyTypeId" class="input" formControlName="propertyTypeId">
              <option [ngValue]="''" disabled>{{ msgs.requiredField }}</option>
              @for (t of propertyTypes(); track t.id) {
                <option [ngValue]="t.id">{{ t.label }}</option>
              }
            </select>
            @if (fieldError('propertyTypeId'); as m) { <div class="pms-field-error">{{ m }}</div> }
          </div>

          <div class="field pms-field">
            <label for="areaId">{{ msgs.propertyArea }}</label>
            <select id="areaId" class="input" formControlName="areaId">
              <option [ngValue]="''" disabled>{{ msgs.requiredField }}</option>
              @for (a of areas(); track a.id) {
                <option [ngValue]="a.id">{{ a.label }}</option>
              }
            </select>
            @if (fieldError('areaId'); as m) { <div class="pms-field-error">{{ m }}</div> }
          </div>

          @if (isAdmin()) {
            <div class="field pms-field">
              <label for="code">{{ msgs.propertyCode }}</label>
              <input id="code" class="input" type="text" formControlName="code" appWesternDigits
                     [placeholder]="msgs.propertyCode" />
              @if (fieldError('code'); as m) { <div class="pms-field-error">{{ m }}</div> }
            </div>
          }

          @if (customStates().length > 0) {
            <h3>{{ msgs.customFields }}</h3>
            @for (cs of customStates(); track cs.field.id) {
              <div class="field pms-field">
                <label [attr.for]="'custom-' + cs.field.id">
                  {{ cs.field.label }}
                  @if (cs.field.isSensitive) {
                    <span class="pms-pill pms-pill-warn">{{ msgs.customFieldSensitiveHint }}</span>
                  }
                </label>
                @switch (cs.field.fieldType) {
                  @case ('text') {
                    <input class="input" [attr.id]="'custom-' + cs.field.id" type="text"
                           [formControl]="cs.text" appWesternDigits />
                  }
                  @case ('checkbox') {
                    <div class="pms-checkbox-row">
                      <input [attr.id]="'custom-' + cs.field.id" type="checkbox"
                             [formControl]="cs.checked" />
                      <label [attr.for]="'custom-' + cs.field.id">{{ msgs.yes }}</label>
                    </div>
                  }
                  @case ('dropdown') {
                    <select class="input" [attr.id]="'custom-' + cs.field.id" [formControl]="cs.choice">
                      <option [ngValue]="''">—</option>
                      @for (ch of cs.field.choices; track ch.id) {
                        <option [ngValue]="ch.id">{{ ch.label }}</option>
                      }
                    </select>
                  }
                  @case ('multiselect') {
                    <div class="pms-multi-row">
                      @for (ch of cs.field.choices; track ch.id) {
                        <label class="pms-multi-option">
                          <input type="checkbox"
                                 [checked]="cs.choices.value.includes(ch.id)"
                                 (change)="toggleMulti(cs, ch.id, $event)" />
                          {{ ch.label }}
                        </label>
                      }
                    </div>
                  }
                }
              </div>
            }
          }

          <div style="display: flex; gap: 0.75rem; margin-block-start: 1rem;">
            <button type="submit" class="btn btn-primary" [disabled]="submitting() || form.invalid || versionMismatch() !== null">
              {{ submitting() ? msgs.loading : (isEdit() ? msgs.save : msgs.create) }}
            </button>
            <a [routerLink]="backLink()" class="btn btn-secondary">{{ msgs.cancel }}</a>
          </div>
        </form>
      </div>
    </section>
  `,
})
export class PropertyFormComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;

  private readonly fb = inject(FormBuilder);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly propertiesSvc = inject(PropertiesService);
  private readonly lookupsSvc = inject(LookupsService);
  private readonly customFieldsSvc = inject(CustomFieldsService);
  private readonly session = inject(SessionService);

  protected readonly isEdit = signal(false);
  protected readonly propertyId = signal<string | null>(null);
  protected readonly version = signal<number>(0);
  protected readonly initialName = signal<string>('');
  protected readonly propertyTypes = signal<Lookup[]>([]);
  protected readonly areas = signal<Lookup[]>([]);
  protected readonly customStates = signal<CustomFormState[]>([]);
  protected readonly submitting = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly versionMismatch = signal<{ name: string; propertyType: Lookup; area: Lookup } | null>(null);
  private readonly fieldErrors = signal<Record<string, string>>({});

  protected readonly form = this.fb.nonNullable.group({
    name: ['', [Validators.required, Validators.minLength(2), Validators.maxLength(120)]],
    propertyTypeId: ['' as string, [Validators.required]],
    areaId: ['' as string, [Validators.required]],
    code: [''],
  });

  protected readonly isAdmin = () => this.session.isAdmin();

  protected fieldError(name: string): string | undefined {
    return this.fieldErrors()[name];
  }

  protected toggleMulti(cs: CustomFormState, id: string, ev: Event): void {
    const checked = (ev.target as HTMLInputElement).checked;
    const current = new Set(cs.choices.value);
    if (checked) current.add(id); else current.delete(id);
    cs.choices.setValue([...current]);
  }

  protected backLink(): unknown[] {
    const id = this.propertyId();
    return id ? ['/properties', id] : ['/properties'];
  }

  ngOnInit(): void {
    this.lookupsSvc.listPropertyTypes('body').subscribe({
      next: (items) => this.propertyTypes.set(items),
      error: () => this.propertyTypes.set([]),
    });
    this.lookupsSvc.listAreas('body').subscribe({
      next: (items) => this.areas.set(items),
      error: () => this.areas.set([]),
    });
    this.customFieldsSvc.listCustomFields('body').subscribe({
      next: (fields) => this.buildCustomStates(fields),
      error: () => this.buildCustomStates([]),
    });

    const id = this.route.snapshot.paramMap.get('propertyId');
    if (id) {
      this.isEdit.set(true);
      this.propertyId.set(id);
      this.propertiesSvc.getProperty({ propertyId: id }, 'body').subscribe({
        next: (p) => {
          this.version.set(p.version);
          this.initialName.set(p.name);
          this.form.patchValue({
            name: p.name,
            propertyTypeId: p.propertyType.id,
            areaId: p.area.id,
            code: p.code,
          });
          // Hydrate custom values from server response.
          this.customStates.update((states) => states.map((cs) => {
            const v = p.customValues.find((cv) => cv.fieldId === cs.field.id);
            if (!v) return cs;
            if (cs.field.fieldType === 'text') {
              cs.text.setValue(v.text ?? '');
            } else if (cs.field.fieldType === 'checkbox') {
              cs.checked.setValue(v.checked ?? null);
            } else if (cs.field.fieldType === 'dropdown') {
              cs.choice.setValue(v.choiceId ?? '');
            } else if (cs.field.fieldType === 'multiselect') {
              cs.choices.setValue(v.choiceIds ?? []);
            }
            return cs;
          }));
        },
        error: (err: ApiError) => {
          this.errorMessage.set(err.message || this.msgs.propertyNotFound);
        },
      });
    }
  }

  private buildCustomStates(fields: CustomField[]): void {
    const states: CustomFormState[] = fields.map((f) => ({
      field: f,
      text: new FormControl<string>('', { nonNullable: true }),
      checked: new FormControl<boolean | null>(null),
      choice: new FormControl<string>('', { nonNullable: true }),
      choices: new FormControl<string[]>([], { nonNullable: true }),
    }));
    this.customStates.set(states);
  }

  protected onSubmit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.submitting.set(true);
    this.errorMessage.set(null);
    this.fieldErrors.set({});
    this.versionMismatch.set(null);

    const raw = this.form.getRawValue();
    const states = this.customStates();
    const customValues = states
      .map((cs): { fieldId: string; text?: string | null; checked?: boolean | null; choiceId?: string | null; choiceIds?: string[] } | null => {
        const out: { fieldId: string; text?: string | null; checked?: boolean | null; choiceId?: string | null; choiceIds?: string[] } = { fieldId: cs.field.id };
        if (cs.field.fieldType === 'text') {
          out.text = cs.text.value || null;
        } else if (cs.field.fieldType === 'checkbox') {
          out.checked = cs.checked.value;
        } else if (cs.field.fieldType === 'dropdown') {
          out.choiceId = cs.choice.value || null;
        } else if (cs.field.fieldType === 'multiselect') {
          out.choiceIds = cs.choices.value;
        }
        return out;
      })
      .filter((x): x is NonNullable<typeof x> => x !== null);

    if (this.isEdit()) {
      this.propertiesSvc
        .updateProperty({
          propertyId: this.propertyId()!,
          propertyUpdate: {
            name: raw.name,
            propertyTypeId: raw.propertyTypeId,
            areaId: raw.areaId,
            version: this.version(),
            customValues,
          },
        }, 'body')
        .subscribe({
          next: () => {
            this.submitting.set(false);
            void this.router.navigate(['/properties', this.propertyId()]);
          },
          error: (err: ApiError) => this.handleSubmitError(err),
        });
    } else {
      const body: { name: string; propertyTypeId: string; areaId: string; code?: string; customValues?: typeof customValues } = {
        name: raw.name,
        propertyTypeId: raw.propertyTypeId,
        areaId: raw.areaId,
        customValues,
      };
      if (this.isAdmin() && raw.code) body.code = raw.code;
      this.propertiesSvc.createProperty({ propertyCreate: body }, 'body').subscribe({
        next: (p) => {
          this.submitting.set(false);
          void this.router.navigate(['/properties', p.id]);
        },
        error: (err: ApiError) => this.handleSubmitError(err),
      });
    }
  }

  private handleSubmitError(err: ApiError): void {
    this.submitting.set(false);
    if (err.code === 'version_conflict') {
      // Reload the property to show the current values.
      const id = this.propertyId();
      if (id) {
        this.propertiesSvc.getProperty({ propertyId: id }, 'body').subscribe({
          next: (p) => {
            this.versionMismatch.set({
              name: p.name,
              propertyType: p.propertyType,
              area: p.area,
            });
            this.version.set(p.version);
            this.form.patchValue({
              name: p.name,
              propertyTypeId: p.propertyType.id,
              areaId: p.area.id,
            });
          },
        });
      }
      this.errorMessage.set(this.msgs.propertyVersionNeeded);
      return;
    }
    if (err.fields) {
      this.fieldErrors.set(err.fields);
    }
    this.errorMessage.set(err.message || this.msgs.internalError);
  }
}
