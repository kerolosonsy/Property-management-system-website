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
import { AutocompleteInputComponent } from '../../shared/autocomplete-input.component';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

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
  imports: [
    FormsModule,
    CommonModule,
    ReactiveFormsModule,
    RouterLink,
    WesternDigitsDirective,
    AutocompleteInputComponent,
  ],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.advancedSearch }}</h1>
          <div class="pms-crumb">{{ msgs.properties }} / {{ msgs.advancedSearch }}</div>
        </div>
        <div class="pms-toolbar-spacer"></div>
        <!-- Navigation stays in the head; the filter actions (search, clear)
             live together in the pinned foot rather than being split across
             both bars. -->
        <a routerLink="/properties" class="btn btn-secondary">{{ msgs.backToList }}</a>
      </header>

      <div class="card">
        <p class="pms-note">{{ msgs.customFieldSensitiveSearch }}</p>

        <!-- Collapsed with [hidden], not @if, so the filter controls stay in
             the DOM and keep their values while only the results show. -->
        <button
          type="button"
          class="btn btn-secondary"
          (click)="toggleFilters()"
          [attr.aria-expanded]="filtersOpen()"
        >
          {{ filtersOpen() ? msgs.filtersHide : msgs.filtersShow }}
        </button>

        <form id="pms-advanced-search-form" [formGroup]="form" (ngSubmit)="onSubmit()" novalidate>
          <div [hidden]="!filtersOpen()">
            <div class="pms-form-grid">
              <div class="field pms-field">
                <label for="q">{{ msgs.search }}</label>
                <app-autocomplete-input
                  inputId="q"
                  formControlName="q"
                  [suggest]="suggestName"
                  [placeholder]="msgs.searchPlaceholder"
                />
              </div>
              <div class="field pms-field">
                <label for="propertyTypeId">{{ msgs.propertyType }}</label>
                <app-autocomplete-input
                  inputId="propertyTypeId"
                  formControlName="propertyTypeId"
                  [options]="typeOptions()"
                  [placeholder]="msgs.allItems"
                />
              </div>
              <div class="field pms-field">
                <label for="areaId">{{ msgs.propertyArea }}</label>
                <app-autocomplete-input
                  inputId="areaId"
                  formControlName="areaId"
                  [options]="areaOptions()"
                  [placeholder]="msgs.allItems"
                />
              </div>
              <div class="field pms-field">
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
              <div class="field pms-field">
                <label for="attachmentName">{{ msgs.attachmentNameSearch }}</label>
                <app-autocomplete-input
                  inputId="attachmentName"
                  formControlName="attachmentName"
                  [suggest]="suggestAttachmentName"
                  [placeholder]="msgs.attachmentNameSearch"
                />
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
              <div class="pms-form-grid">
                @for (fs of filterStates(); track fs.field.id) {
                  <div
                    class="field pms-field"
                    [class.pms-form-grid-full]="fs.field.fieldType === 'multiselect'"
                  >
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
                      @case ('autocomplete') {
                        <app-autocomplete-input
                          [inputId]="'adv-' + fs.field.id"
                          [formControl]="fs.text"
                          [suggest]="suggestCustomField(fs.field.id)"
                          [ariaLabel]="fs.field.label"
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
              </div>
            }
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

      <!-- The search actions pin to the foot so a search can be re-run or
           cleared from anywhere in the results. The submit reaches the form
           by its id — the form itself may be collapsed out of sight, never
           out of the DOM. -->
      <footer class="pms-view-foot">
        <button
          type="submit"
          class="btn btn-primary"
          [disabled]="searching()"
          form="pms-advanced-search-form"
        >
          {{ msgs.search }}
        </button>
        <button type="button" class="btn btn-secondary" (click)="clearAll()">
          {{ msgs.clearAllFilters }}
        </button>
      </footer>
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

  // Type/area filter comboboxes narrow these already-loaded lists in the
  // browser; they never call the suggestion endpoint.
  protected readonly typeOptions = computed(() =>
    this.propertyTypes().map((t) => ({ value: t.id, label: t.label })),
  );
  protected readonly areaOptions = computed(() =>
    this.areas().map((a) => ({ value: a.id, label: a.label })),
  );

  // Type-ahead sources. Sensitive fields never reach filterStates at all
  // (filtered out below), so no suggestion about them is ever requested.
  protected readonly suggestName = (q: string) =>
    this.searchSvc
      .getFieldSuggestions({ field: 'name', q: q || undefined }, 'body')
      .pipe(map((resp) => resp.suggestions));
  protected readonly suggestAttachmentName = (q: string) =>
    this.searchSvc
      .getFieldSuggestions({ field: 'attachmentName', q: q || undefined }, 'body')
      .pipe(map((resp) => resp.suggestions));

  private readonly customSuggesters = new Map<string, (q: string) => Observable<string[]>>();

  /** One cached suggester per custom field id, so template re-renders reuse it. */
  protected suggestCustomField(fieldId: string): (q: string) => Observable<string[]> {
    let fn = this.customSuggesters.get(fieldId);
    if (!fn) {
      fn = (q: string) =>
        this.searchSvc
          .getFieldSuggestions({ fieldId, q: q || undefined }, 'body')
          .pipe(map((resp) => resp.suggestions));
      this.customSuggesters.set(fieldId, fn);
    }
    return fn;
  }

  // The filter block collapses so the results are reachable without
  // scrolling past a wall of inputs. Default open; nothing is remembered —
  // collapsing is a view convenience, not state.
  protected readonly filtersOpen = signal(true);

  protected toggleFilters(): void {
    this.filtersOpen.update((open) => !open);
  }

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
        case 'autocomplete':
          // autocomplete filters exactly like text: the contains operator on
          // the stored value, which is the same column for both types.
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
