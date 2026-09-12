// web/src/app/features/properties/properties-list.component.ts
// US1 — the register. Paged, searchable, filterable, and Arabic-only.

import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormControl, FormsModule, ReactiveFormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { PropertiesService } from '../../api/api/properties.service';
import { LookupsService } from '../../api/api/lookups.service';
import { SearchService } from '../../api/api/search.service';
import { PropertySummary } from '../../api/model/property-summary.model';
import { Lookup } from '../../api/model/lookup.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { AutocompleteInputComponent } from '../../shared/autocomplete-input.component';
import { debounceTime } from 'rxjs/operators';
import { map } from 'rxjs/operators';

const PAGE_SIZES = [10, 25, 50, 100] as const;

@Component({
  selector: 'app-properties-list',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule, RouterLink, AutocompleteInputComponent],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.propertiesList }}</h1>
          <div class="pms-crumb">{{ msgs.propertiesSubtitle }}</div>
        </div>
        <div class="pms-toolbar-spacer"></div>

        <!-- The design joins the search field to the advanced-search button
             into a single control group. The header search spans name and
             code; its suggestions come from the name column. -->
        <div class="pms-searchgroup">
          <div class="field">
            <app-autocomplete-input
              inputId="search"
              [formControl]="searchCtrl"
              [suggest]="suggestName"
              [placeholder]="msgs.searchPlaceholder"
              [ariaLabel]="msgs.search"
            />
          </div>
          <a
            routerLink="/properties/search"
            class="btn btn-secondary"
            [title]="msgs.advancedSearch"
            [attr.aria-label]="msgs.advancedSearch"
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <circle cx="11" cy="11" r="7" />
              <path d="M20 20l-3.5-3.5" />
            </svg>
          </a>
        </div>

        <a routerLink="/properties/new" class="btn btn-primary">
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M12 5v14M5 12h14" />
          </svg>
          {{ msgs.addProperty }}
        </a>
      </header>

      <div class="pms-section-body">
        <div class="pms-section-head">
          <div>
            <h2>{{ msgs.propertiesAll }}</h2>
            <div class="pms-section-count">{{ rangeLabel() }}</div>
          </div>
        </div>

        <div class="card elev-sm pms-card-flush">
          <div class="pms-filters">
            <div class="field pms-field pms-field-checkbox">
              <label for="includeArchived">{{ msgs.includeArchived }}</label>
              <input id="includeArchived" type="checkbox" [formControl]="includeArchivedCtrl" />
            </div>
          </div>

          @if (errorMessage(); as msg) {
            <div class="pms-error-banner">{{ msg }}</div>
          }

          <!-- The table always renders. Its <thead> carries the column filters,
             so collapsing it on an empty result would strip away the very
             controls needed to undo the filter that emptied it. -->
          <table class="table">
            <thead>
              <tr>
                <th>{{ msgs.propertyCode }}</th>
                <th>{{ msgs.propertyName }}</th>
                <th>{{ msgs.propertyType }}</th>
                <th>{{ msgs.propertyArea }}</th>
                <th></th>
              </tr>
              <!-- Per-column filters, as the design source has them. Every
                   filter is applied by the server across the whole register,
                   not to the visible page. Text columns suggest previously
                   stored values; the type and area comboboxes filter their
                   already-loaded lookup lists in the browser. -->
              <tr class="pms-colfilter">
                <th>
                  <app-autocomplete-input
                    inputId="colfilter-code"
                    [formControl]="codeCtrl"
                    [suggest]="suggestCode"
                    [placeholder]="msgs.filterPlaceholder"
                    [ariaLabel]="msgs.propertyCode"
                  />
                </th>
                <th>
                  <app-autocomplete-input
                    inputId="colfilter-name"
                    [formControl]="nameCtrl"
                    [suggest]="suggestName"
                    [placeholder]="msgs.filterPlaceholder"
                    [ariaLabel]="msgs.propertyName"
                  />
                </th>
                <th>
                  <app-autocomplete-input
                    inputId="colfilter-type"
                    [formControl]="typeCtrl"
                    [options]="typeOptions()"
                    [placeholder]="msgs.allItems"
                    [ariaLabel]="msgs.propertyType"
                  />
                </th>
                <th>
                  <app-autocomplete-input
                    inputId="colfilter-area"
                    [formControl]="areaCtrl"
                    [options]="areaOptions()"
                    [placeholder]="msgs.allItems"
                    [ariaLabel]="msgs.propertyArea"
                  />
                </th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              @for (p of items(); track p.id) {
                <tr>
                  <td>
                    <a [routerLink]="['/properties', p.id]">{{ p.code }}</a>
                  </td>
                  <td>
                    <a [routerLink]="['/properties', p.id]">{{ p.name }}</a>
                    @if (p.isArchived) {
                      <span class="pms-pill pms-pill-warn">{{ msgs.propertyArchived }}</span>
                    }
                  </td>
                  <td>{{ p.propertyType.label }}</td>
                  <td>{{ p.area.label }}</td>
                  <td>
                    <a [routerLink]="['/properties', p.id, 'edit']">{{ msgs.edit }}</a>
                  </td>
                </tr>
              }

              @if (items().length === 0) {
                <tr>
                  <td colspan="5" class="pms-empty-cell">
                    @if (loading()) {
                      <span class="text-muted">{{ msgs.loading }}</span>
                    } @else if (totalItems() === 0 && !hasActiveFilters()) {
                      {{ msgs.propertyEmptyRegister }}
                    } @else {
                      {{ msgs.propertyNoMatch }}
                      <button
                        type="button"
                        class="btn btn-secondary pms-clear-inline"
                        (click)="clearFilters()"
                      >
                        {{ msgs.clearAllFilters }}
                      </button>
                    }
                  </td>
                </tr>
              }
            </tbody>
          </table>
        </div>

        <!-- The paging bar pins to the foot so a full page of rows never
             means scrolling to the bottom for the next page and back up. -->
        @if (items().length > 0) {
          <footer class="pms-view-foot pms-paging">
            <span>{{ rangeLabel() }}</span>
            <div class="pms-toolbar-spacer"></div>
            <label>
              <select class="input" [formControl]="pageSizeCtrl">
                @for (n of pageSizes; track n) {
                  <option [ngValue]="n">{{ n }}</option>
                }
              </select>
            </label>
            <button
              type="button"
              class="btn btn-secondary"
              (click)="prev()"
              [disabled]="page() <= 1"
            >
              {{ msgs.pagePrevious }}
            </button>
            <span>{{ format(msgs.pageOf, { page: page() }) }}</span>
            <button
              type="button"
              class="btn btn-secondary"
              (click)="next()"
              [disabled]="page() * pageSize() >= totalItems()"
            >
              {{ msgs.pageNext }}
            </button>
          </footer>
        }
      </div>
    </section>
  `,
})
export class PropertiesListComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;
  protected readonly pageSizes = PAGE_SIZES;

  private readonly propertiesSvc = inject(PropertiesService);
  private readonly lookupsSvc = inject(LookupsService);
  private readonly searchSvc = inject(SearchService);
  private readonly router = inject(Router);

  protected readonly items = signal<PropertySummary[]>([]);
  protected readonly totalItems = signal(0);
  protected readonly page = signal(1);
  protected readonly pageSize = signal(25);
  protected readonly loading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly propertyTypes = signal<Lookup[]>([]);
  protected readonly areas = signal<Lookup[]>([]);

  // The lookup comboboxes filter these in the browser; no request is made for
  // them (the full lists are already loaded for the filter).
  protected readonly typeOptions = computed(() =>
    this.propertyTypes().map((t) => ({ value: t.id, label: t.label })),
  );
  protected readonly areaOptions = computed(() =>
    this.areas().map((a) => ({ value: a.id, label: a.label })),
  );

  // Type-ahead sources: the header search suggests names, the code column
  // suggests codes. Debounce and supersession live inside the shared
  // autocomplete component; these functions only fetch.
  protected readonly suggestName = (q: string) =>
    this.searchSvc
      .getFieldSuggestions({ field: 'name', q: q || undefined }, 'body')
      .pipe(map((resp) => resp.suggestions));
  protected readonly suggestCode = (q: string) =>
    this.searchSvc
      .getFieldSuggestions({ field: 'code', q: q || undefined }, 'body')
      .pipe(map((resp) => resp.suggestions));

  protected readonly searchCtrl = new FormControl('', { nonNullable: true });
  protected readonly typeCtrl = new FormControl('', { nonNullable: true });
  protected readonly areaCtrl = new FormControl('', { nonNullable: true });
  // Per-column filters. Each narrows one column; the header search still spans
  // both name and code.
  protected readonly codeCtrl = new FormControl('', { nonNullable: true });
  protected readonly nameCtrl = new FormControl('', { nonNullable: true });
  protected readonly includeArchivedCtrl = new FormControl(false, { nonNullable: true });
  protected readonly pageSizeCtrl = new FormControl(25, { nonNullable: true });

  /** True when anything is narrowing the list, so an empty result can be told
   *  apart from an empty register. */
  protected hasActiveFilters(): boolean {
    return (
      this.searchCtrl.value !== '' ||
      this.codeCtrl.value !== '' ||
      this.nameCtrl.value !== '' ||
      this.typeCtrl.value !== '' ||
      this.areaCtrl.value !== ''
    );
  }

  protected clearFilters(): void {
    this.searchCtrl.setValue('', { emitEvent: false });
    this.codeCtrl.setValue('', { emitEvent: false });
    this.nameCtrl.setValue('', { emitEvent: false });
    this.typeCtrl.setValue('', { emitEvent: false });
    this.areaCtrl.setValue('', { emitEvent: false });
    this.page.set(1);
    this.refresh();
  }

  protected readonly rangeLabel = computed(() => {
    const total = this.totalItems();
    const from = total === 0 ? 0 : (this.page() - 1) * this.pageSize() + 1;
    const to = Math.min(this.page() * this.pageSize(), total);
    return format(ARABIC_MESSAGES.rangeIndicator, { from, to, total });
  });

  ngOnInit(): void {
    this.refreshLookups();
    // Text inputs debounce; a request per keystroke across four filters would
    // be a request storm and the last response could land out of order.
    for (const ctrl of [this.searchCtrl, this.codeCtrl, this.nameCtrl]) {
      ctrl.valueChanges.pipe(debounceTime(250)).subscribe(() => {
        this.page.set(1);
        this.refresh();
      });
    }
    // The lookup comboboxes now also emit while the user types (the selection
    // is '' until an option is accepted), so they debounce like the text ones
    // rather than refreshing per keystroke.
    for (const ctrl of [this.typeCtrl, this.areaCtrl]) {
      ctrl.valueChanges.pipe(debounceTime(250)).subscribe(() => {
        this.page.set(1);
        this.refresh();
      });
    }
    this.includeArchivedCtrl.valueChanges.subscribe(() => {
      this.page.set(1);
      this.refresh();
    });
    this.pageSizeCtrl.valueChanges.subscribe((n) => {
      this.pageSize.set(n);
      this.page.set(1);
      this.refresh();
    });
    this.refresh();
  }

  protected prev(): void {
    if (this.page() > 1) {
      this.page.set(this.page() - 1);
      this.refresh();
    }
  }

  protected next(): void {
    if (this.page() * this.pageSize() < this.totalItems()) {
      this.page.set(this.page() + 1);
      this.refresh();
    }
  }

  private refreshLookups(): void {
    this.lookupsSvc.listPropertyTypes('body').subscribe({
      next: (items) => this.propertyTypes.set(items),
      error: () => this.propertyTypes.set([]),
    });
    this.lookupsSvc.listAreas('body').subscribe({
      next: (items) => this.areas.set(items),
      error: () => this.areas.set([]),
    });
  }

  private refresh(): void {
    this.loading.set(true);
    this.errorMessage.set(null);
    this.propertiesSvc
      .listProperties(
        {
          page: this.page(),
          pageSize: this.pageSize() as 10 | 25 | 50 | 100,
          q: this.searchCtrl.value || undefined,
          code: this.codeCtrl.value || undefined,
          name: this.nameCtrl.value || undefined,
          propertyTypeId: this.typeCtrl.value || undefined,
          areaId: this.areaCtrl.value || undefined,
          includeArchived: this.includeArchivedCtrl.value,
        },
        'body',
      )
      .subscribe({
        next: (resp) => {
          this.items.set(resp.items);
          this.totalItems.set(resp.totalItems);
          this.loading.set(false);
        },
        error: (err: ApiError) => {
          this.loading.set(false);
          this.errorMessage.set(err.message || this.msgs.internalError);
        },
      });
  }
}
