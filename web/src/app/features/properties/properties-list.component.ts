// web/src/app/features/properties/properties-list.component.ts
// US1 — the register. Paged, searchable, filterable, and Arabic-only.

import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormControl, FormsModule, ReactiveFormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { PropertiesService } from '../../api/api/properties.service';
import { LookupsService } from '../../api/api/lookups.service';
import { PropertySummary } from '../../api/model/property-summary.model';
import { Lookup } from '../../api/model/lookup.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { debounceTime } from 'rxjs/operators';

const PAGE_SIZES = [10, 25, 50, 100] as const;

@Component({
  selector: 'app-properties-list',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule, RouterLink],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.propertiesList }}</h1>
          <div class="pms-crumb">{{ msgs.propertiesSubtitle }}</div>
        </div>
        <div class="pms-toolbar-spacer"></div>

        <!-- The design joins the search field to the advanced-search button
             into a single control group. -->
        <div class="pms-searchgroup">
          <div class="field">
            <input id="search" class="input" type="search" [formControl]="searchCtrl"
                   [attr.aria-label]="msgs.search" [placeholder]="msgs.searchPlaceholder" />
          </div>
          <a routerLink="/properties/search" class="btn btn-secondary"
             [title]="msgs.advancedSearch" [attr.aria-label]="msgs.advancedSearch">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" />
            </svg>
          </a>
        </div>

        <a routerLink="/properties/new" class="btn btn-primary">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
               stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
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

          <!-- Type filter as the design's segmented control, not a dropdown. -->
          <div class="seg" role="group" [attr.aria-label]="msgs.propertyType">
            <label class="seg-opt">
              <input type="radio" name="typeFilter" [checked]="typeCtrl.value === ''"
                     (change)="setType('')" />
              <span>{{ msgs.allItems }}</span>
            </label>
            @for (t of propertyTypes(); track t.id) {
              <label class="seg-opt">
                <input type="radio" name="typeFilter" [checked]="typeCtrl.value === t.id"
                       (change)="setType(t.id)" />
                <span>{{ t.label }}</span>
              </label>
            }
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
                   not to the visible page. -->
              <tr class="pms-colfilter">
                <th>
                  <input class="input" type="search" [formControl]="codeCtrl"
                         [attr.aria-label]="msgs.propertyCode" [placeholder]="msgs.filterPlaceholder" />
                </th>
                <th>
                  <input class="input" type="search" [formControl]="nameCtrl"
                         [attr.aria-label]="msgs.propertyName" [placeholder]="msgs.filterPlaceholder" />
                </th>
                <th>
                  <select class="input" [formControl]="typeCtrl" [attr.aria-label]="msgs.propertyType">
                    <option [ngValue]="''">{{ msgs.allItems }}</option>
                    @for (t of propertyTypes(); track t.id) {
                      <option [ngValue]="t.id">{{ t.label }}</option>
                    }
                  </select>
                </th>
                <th>
                  <select class="input" [formControl]="areaCtrl" [attr.aria-label]="msgs.propertyArea">
                    <option [ngValue]="''">{{ msgs.allItems }}</option>
                    @for (a of areas(); track a.id) {
                      <option [ngValue]="a.id">{{ a.label }}</option>
                    }
                  </select>
                </th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              @for (p of items(); track p.id) {
                <tr>
                  <td><a [routerLink]="['/properties', p.id]">{{ p.code }}</a></td>
                  <td>
                    <a [routerLink]="['/properties', p.id]">{{ p.name }}</a>
                    @if (p.isArchived) {
                      <span class="pms-pill pms-pill-warn">{{ msgs.propertyArchived }}</span>
                    }
                  </td>
                  <td>{{ p.propertyType.label }}</td>
                  <td>{{ p.area.label }}</td>
                  <td>
                    <a [routerLink]="['/properties', p.id]">{{ msgs.edit }}</a>
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
                      <button type="button" class="btn btn-secondary pms-clear-inline"
                              (click)="clearFilters()">{{ msgs.clearAllFilters }}</button>
                    }
                  </td>
                </tr>
              }
            </tbody>
          </table>

          @if (items().length > 0) {
          <div class="pms-paging">
            <span>{{ rangeLabel() }}</span>
            <div class="pms-toolbar-spacer"></div>
            <label>
              <select class="input" [formControl]="pageSizeCtrl">
                @for (n of pageSizes; track n) {
                  <option [ngValue]="n">{{ n }}</option>
                }
              </select>
            </label>
            <button type="button" class="btn btn-secondary" (click)="prev()" [disabled]="page() <= 1">
              {{ msgs.pagePrevious }}
            </button>
            <span>{{ format(msgs.pageOf, { page: page() }) }}</span>
            <button type="button" class="btn btn-secondary" (click)="next()" [disabled]="page() * pageSize() >= totalItems()">
              {{ msgs.pageNext }}
            </button>
          </div>
          }
      </div>
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
  private readonly router = inject(Router);

  protected readonly items = signal<PropertySummary[]>([]);
  protected readonly totalItems = signal(0);
  protected readonly page = signal(1);
  protected readonly pageSize = signal(25);
  protected readonly loading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly propertyTypes = signal<Lookup[]>([]);
  protected readonly areas = signal<Lookup[]>([]);

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

  protected setType(id: string): void {
    this.typeCtrl.setValue(id);
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
    this.typeCtrl.valueChanges.subscribe(() => {
      this.page.set(1);
      this.refresh();
    });
    this.areaCtrl.valueChanges.subscribe(() => {
      this.page.set(1);
      this.refresh();
    });
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
