// web/src/app/features/properties/advanced-search.component.ts
// US8 — advanced search across built-in and custom fields. Sensitive fields
// are absent from the filter list (Constitution VII); a client that bypasses
// the UI is refused by the server.

import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule, FormControl, FormGroup, ReactiveFormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { SearchService } from '../../api/api/search.service';
import { LookupsService } from '../../api/api/lookups.service';
import { CustomFieldsService } from '../../api/api/custom-fields.service';
import { CustomField } from '../../api/model/custom-field.model';
import { Lookup } from '../../api/model/lookup.model';
import { PropertySummary } from '../../api/model/property-summary.model';
import { SearchOperator } from '../../api/model/search-operator.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { AdvancedSearch } from '../../api/model/advanced-search.model';
import { ApiError } from '../../core/api-error';
import { WesternDigitsDirective } from '../../shared/western-digits.directive';

interface CustomFilterState {
  field: CustomField;
  text: FormControl<string>;
  choice: FormControl<string>;
  choices: FormControl<string[]>;
  checked: FormControl<boolean | null>;
}

@Component({
  selector: 'app-advanced-search',
  standalone: true,
  imports: [FormsModule, CommonModule, ReactiveFormsModule, RouterLink, WesternDigitsDirective],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.advancedSearch }}</h1>
          <div class="pms-crumb">{{ msgs.properties }} / {{ msgs.advancedSearch }}</div>
        </div>
        <div class="pms-toolbar-spacer"></div>
        <a routerLink="/properties" class="btn btn-secondary">{{ msgs.backToList }}</a>
        <button type="button" class="btn btn-secondary" (click)="clearAll()">
          {{ msgs.clearAllFilters }}
        </button>
      </header>

      <div class="card">
        <p class="pms-note">{{ msgs.customFieldSensitiveSearch }}</p>

        <form [formGroup]="form" (ngSubmit)="onSubmit()" novalidate>
          <div class="pms-filters">
            <div class="field pms-field pms-field-grow">
              <label for="q">{{ msgs.search }}</label>
              <input
                id="q"
                class="input"
                type="search"
                formControlName="q"
                appWesternDigits
                [placeholder]="msgs.searchPlaceholder"
              />
            </div>
            <div class="field pms-field">
              <label for="propertyTypeId">{{ msgs.propertyType }}</label>
              <select id="propertyTypeId" class="input" formControlName="propertyTypeId">
                <option [ngValue]="''">{{ msgs.allItems }}</option>
                @for (t of propertyTypes(); track t.id) {
                  <option [ngValue]="t.id">{{ t.label }}</option>
                }
              </select>
            </div>
            <div class="field pms-field">
              <label for="areaId">{{ msgs.propertyArea }}</label>
              <select id="areaId" class="input" formControlName="areaId">
                <option [ngValue]="''">{{ msgs.allItems }}</option>
                @for (a of areas(); track a.id) {
                  <option [ngValue]="a.id">{{ a.label }}</option>
                }
              </select>
            </div>
            <div class="field pms-field pms-field-grow">
              <label for="documentText">{{ msgs.documentSearch }}</label>
              <input
                id="documentText"
                class="input"
                type="search"
                formControlName="documentText"
                appWesternDigits
                [placeholder]="msgs.documentSearchPlaceholder"
              />
              <div class="pms-note">{{ msgs.documentSearchSensitiveExcluded }}</div>
            </div>
            <div class="field pms-field pms-field-grow">
              <label for="attachmentName">{{ msgs.attachmentNameSearch }}</label>
              <input id="attachmentName" class="input" type="search" formControlName="attachmentName"
                     appWesternDigits [placeholder]="msgs.attachmentNameSearch" />
              <div class="pms-note">{{ msgs.attachmentNameSearchHint }}</div>
            </div>
            <div class="field pms-field">
              <label for="hasAttachments">{{ msgs.hasAttachmentsFilter }}</label>
              <select id="hasAttachments" class="input" formControlName="hasAttachments">
                <option [ngValue]="'any'">{{ msgs.hasAttachmentsAny }}</option>
                <option [ngValue]="'yes'">{{ msgs.hasAttachmentsYes }}</option>
                <option [ngValue]="'no'">{{ msgs.hasAttachmentsNo }}</option>
              </select>
            </div>
            <div class="field pms-field pms-field-checkbox">
              <label for="includeArchived">{{ msgs.includeArchived }}</label>
              <input id="includeArchived" type="checkbox" formControlName="includeArchived" />
            </div>
          </div>

          @if (filterStates().length > 0) {
            <h3>{{ msgs.customFields }}</h3>
            @for (fs of filterStates(); track fs.field.id) {
              <div class="field pms-field">
                <label [attr.for]="'adv-' + fs.field.id">{{ fs.field.label }}</label>
                @switch (fs.field.fieldType) {
                  @case ('text') {
                    <input
                      class="input"
                      [attr.id]="'adv-' + fs.field.id"
                      type="text"
                      [formControl]="fs.text"
                      appWesternDigits
                    />
                  }
                  @case ('dropdown') {
                    <select
                      class="input"
                      [attr.id]="'adv-' + fs.field.id"
                      [formControl]="fs.choice"
                    >
                      <option [ngValue]="''">—</option>
                      @for (ch of fs.field.choices; track ch.id) {
                        <option [ngValue]="ch.id">{{ ch.label }}</option>
                      }
                    </select>
                  }
                  @case ('multiselect') {
                    <div class="pms-multi-row">
                      @for (ch of fs.field.choices; track ch.id) {
                        <label class="pms-multi-option">
                          <input
                            type="checkbox"
                            [checked]="fs.choices.value.includes(ch.id)"
                            (change)="toggleMulti(fs, ch.id, $event)"
                          />
                          {{ ch.label }}
                        </label>
                      }
                    </div>
                  }
                  @case ('checkbox') {
                    <select
                      class="input"
                      [attr.id]="'adv-' + fs.field.id"
                      [formControl]="fs.checked"
                    >
                      <option [ngValue]="null">—</option>
                      <option [ngValue]="true">{{ msgs.yes }}</option>
                      <option [ngValue]="false">{{ msgs.no }}</option>
                    </select>
                  }
                }
              </div>
            }
          }

          <div style="display: flex; gap: 0.75rem; margin-block-start: 1rem;">
            <button type="submit" class="btn btn-primary" [disabled]="searching()">
              {{ msgs.search }}
            </button>
            <button type="button" class="btn btn-secondary" (click)="clearAll()">
              {{ msgs.clearAllFilters }}
            </button>
          </div>
        </form>

        @if (errorMessage(); as msg) {
          <div class="pms-error-banner">{{ msg }}</div>
        }

        @if (results().length > 0) {
          <h3>{{ msgs.advancedSearchResults }} ({{ totalItems() }})</h3>
          <table class="table">
            <thead>
              <tr>
                <th>{{ msgs.propertyCode }}</th>
                <th>{{ msgs.propertyName }}</th>
                <th>{{ msgs.propertyType }}</th>
                <th>{{ msgs.propertyArea }}</th>
              </tr>
            </thead>
            <tbody>
              @for (p of results(); track p.id) {
                <tr>
                  <td>
                    <a [routerLink]="['/properties', p.id]">{{ p.code }}</a>
                  </td>
                  <td>
                    <a [routerLink]="['/properties', p.id]">{{ p.name }}</a>
                    @if (p.isArchived) {
                      <span class="pms-pill pms-pill-warn">{{ msgs.propertyArchived }}</span>
                    }
                    @if (p.matchedAttachments?.length) {
                      <div class="pms-note">
                        {{ msgs.documentSearchMatches }}:
                        @for (attachment of p.matchedAttachments; track attachment.id) {
                          <a
                            [routerLink]="[
                              '/properties',
                              p.id,
                              'attachments',
                              attachment.id,
                              'text',
                            ]"
                          >
                            {{ attachment.description }}
                          </a>
                        }
                      </div>
                    }
                  </td>
                  <td>{{ p.propertyType.label }}</td>
                  <td>{{ p.area.label }}</td>
                </tr>
              }
            </tbody>
          </table>
        } @else if (searched()) {
          <p class="pms-empty">{{ msgs.propertyNoMatch }}</p>
        }
      </div>
    </section>
  `,
})
export class AdvancedSearchComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;

  private readonly searchSvc = inject(SearchService);
  private readonly lookupsSvc = inject(LookupsService);
  private readonly customFieldsSvc = inject(CustomFieldsService);
  private readonly router = inject(Router);

  protected readonly propertyTypes = signal<Lookup[]>([]);
  protected readonly areas = signal<Lookup[]>([]);
  protected readonly filterStates = signal<CustomFilterState[]>([]);
  protected readonly results = signal<PropertySummary[]>([]);
  protected readonly totalItems = signal(0);
  protected readonly searched = signal(false);
  protected readonly searching = signal(false);
  protected readonly errorMessage = signal<string | null>(null);

  protected readonly form = new FormGroup({
    q: new FormControl('', { nonNullable: true }),
    propertyTypeId: new FormControl('', { nonNullable: true }),
    areaId: new FormControl('', { nonNullable: true }),
    documentText: new FormControl('', { nonNullable: true }),
    attachmentName: new FormControl('', { nonNullable: true }),
    hasAttachments: new FormControl('any', { nonNullable: true }),
    includeArchived: new FormControl(false, { nonNullable: true }),
    page: new FormControl(1, { nonNullable: true }),
    pageSize: new FormControl(25, { nonNullable: true }),
  });

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
      next: (fields) => {
        // FR-027s4: sensitive fields are excluded from the search.
        const searchable = fields.filter((f) => !f.isSensitive);
        this.filterStates.set(
          searchable.map((f) => ({
            field: f,
            text: new FormControl<string>('', { nonNullable: true }),
            choice: new FormControl<string>('', { nonNullable: true }),
            choices: new FormControl<string[]>([], { nonNullable: true }),
            checked: new FormControl<boolean | null>(null),
          })),
        );
      },
      error: () => this.filterStates.set([]),
    });
  }

  protected toggleMulti(fs: CustomFilterState, id: string, ev: Event): void {
    const checked = (ev.target as HTMLInputElement).checked;
    const current = new Set(fs.choices.value);
    if (checked) current.add(id);
    else current.delete(id);
    fs.choices.setValue([...current]);
  }

  protected clearAll(): void {
    this.form.reset({
      q: '',
      documentText: '',
      propertyTypeId: '',
      areaId: '',
      includeArchived: false,
      page: 1,
      pageSize: 25,
    });
    for (const fs of this.filterStates()) {
      fs.text.setValue('');
      fs.choice.setValue('');
      fs.choices.setValue([]);
      fs.checked.setValue(null);
    }
    this.results.set([]);
    this.totalItems.set(0);
    this.searched.set(false);
    this.errorMessage.set(null);
  }

  protected onSubmit(): void {
    this.searching.set(true);
    this.errorMessage.set(null);
    const raw = this.form.getRawValue();

    const customFilters: {
      fieldId: string;
      operator: SearchOperator;
      text?: string | null;
      choiceId?: string | null;
      choiceIds?: string[];
    }[] = [];
    for (const fs of this.filterStates()) {
      switch (fs.field.fieldType) {
        case 'text':
          if (fs.text.value)
            customFilters.push({
              fieldId: fs.field.id,
              operator: SearchOperator.Contains,
              text: fs.text.value,
            });
          break;
        case 'dropdown':
          if (fs.choice.value)
            customFilters.push({
              fieldId: fs.field.id,
              operator: SearchOperator.Equals,
              choiceId: fs.choice.value,
            });
          break;
        case 'multiselect':
          if (fs.choices.value.length > 0)
            customFilters.push({
              fieldId: fs.field.id,
              operator: SearchOperator.IncludesAll,
              choiceIds: fs.choices.value,
            });
          break;
        case 'checkbox':
          if (fs.checked.value === true)
            customFilters.push({ fieldId: fs.field.id, operator: SearchOperator.IsTrue });
          else if (fs.checked.value === false)
            customFilters.push({ fieldId: fs.field.id, operator: SearchOperator.IsFalse });
          break;
      }
    }

    this.searchSvc
      .searchProperties(
        {
          advancedSearch: {
            q: raw.q || undefined,
            documentText: raw.documentText || undefined,
        attachmentName: raw.attachmentName || undefined,
        hasAttachments: (raw.hasAttachments || 'any') as AdvancedSearch.HasAttachmentsEnum,
            propertyTypeId: raw.propertyTypeId || undefined,
            areaId: raw.areaId || undefined,
            includeArchived: raw.includeArchived,
            page: raw.page,
            pageSize: raw.pageSize as 10 | 25 | 50 | 100,
            customFilters: customFilters.length > 0 ? customFilters : undefined,
          },
        },
        'body',
      )
      .subscribe({
        next: (resp) => {
          this.results.set(resp.items);
          this.totalItems.set(resp.totalItems);
          this.searched.set(true);
          this.searching.set(false);
        },
        error: (err: ApiError) => {
          this.searching.set(false);
          this.errorMessage.set(err.message || this.msgs.internalError);
        },
      });
  }
}
