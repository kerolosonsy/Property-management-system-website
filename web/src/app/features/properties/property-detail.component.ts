// web/src/app/features/properties/property-detail.component.ts
// US3 / US4 / US5 — one property, every stored field, the record section, and
// the actions that change state.

import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { PropertiesService } from '../../api/api/properties.service';
import { CustomFieldsService } from '../../api/api/custom-fields.service';
import { SessionService } from '../../core/session.service';
import { PropertyDetail } from '../../api/model/property-detail.model';
import { CustomField } from '../../api/model/custom-field.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { formatCairoDate, renderCustomValues } from './custom-values';

@Component({
  selector: 'app-property-detail',
  standalone: true,
  imports: [CommonModule, RouterLink, ReactiveFormsModule],
  template: `
    <section class="pms-page">
      @if (errorMessage(); as msg) {
        <div class="pms-error-banner">{{ msg }}</div>
      }

      @if (property(); as p) {
        <header class="pms-view-head">
          <div>
            <h1>{{ p.name }}</h1>
            <div class="pms-crumb">{{ msgs.properties }} / {{ p.code }}</div>
          </div>
          <div class="pms-toolbar-spacer"></div>
          <a [routerLink]="['/properties']" class="btn btn-secondary">{{ msgs.backToList }}</a>
          <a [routerLink]="['/properties', p.id, 'edit']" class="btn btn-primary">{{ msgs.editProperty }}</a>
          @if (!p.isArchived) {
            <button type="button" class="btn btn-secondary" (click)="askArchive()">{{ msgs.archive }}</button>
          } @else {
            <button type="button" class="btn btn-primary" (click)="askRestore()">{{ msgs.restore }}</button>
          }
        </header>

        <div class="card blueprint elev-sm">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
          <h2>{{ msgs.propertyDetail }}</h2>
          <dl class="pms-detail-grid">
            <dt>{{ msgs.propertyCode }}</dt>
            <dd class="pms-detail-code">
              <span>{{ p.code }}</span>
              @if (canEditCode()) {
                <form (ngSubmit)="saveCode()" class="pms-inline-form">
                  <input class="input" type="text" [formControl]="codeCtrl" name="code" />
                  <button class="btn btn-secondary" type="submit" [disabled]="codeSaving()">
                    {{ msgs.save }}
                  </button>
                </form>
              }
            </dd>
            <dt>{{ msgs.propertyName }}</dt><dd>{{ p.name }}</dd>
            <dt>{{ msgs.propertyType }}</dt><dd>{{ p.propertyType.label }}</dd>
            <dt>{{ msgs.propertyArea }}</dt><dd>{{ p.area.label }}</dd>
            @if (p.isArchived) {
              <dt>{{ msgs.propertyArchivedAt }}</dt><dd>{{ formatDate(p.archivedAt) }}</dd>
              <dt>{{ msgs.propertyArchivedBy_ }}</dt><dd>{{ p.archivedBy || '—' }}</dd>
            }
          </dl>

          @if (renderedValues().length > 0) {
            <h3>{{ msgs.customFields }}</h3>
            <dl class="pms-detail-grid">
              @for (rv of renderedValues(); track rv.field.id) {
                <dt>
                  {{ rv.field.label }}
                  @if (rv.isSensitive) {
                    <span class="pms-pill pms-pill-warn">{{ msgs.customFieldSensitiveHint }}</span>
                  }
                </dt>
                <dd>{{ rv.display }}</dd>
              }
            </dl>
          }
        </div>

        <div class="card blueprint elev-sm">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
          <h3>{{ msgs.propertyRecord }}</h3>
          <dl class="pms-detail-grid">
            <dt>{{ msgs.propertyCreated }}</dt>
            <dd>{{ formatDate(p.createdAt) }} — {{ p.createdBy }}</dd>
            <dt>{{ msgs.propertyLastModified }}</dt>
            <dd>{{ formatDate(p.updatedAt) }} — {{ p.updatedBy }}</dd>
          </dl>
        </div>
      } @else if (!loading()) {
        <p class="pms-empty">{{ msgs.propertyNotFound }}</p>
      }
    </section>

    @if (confirmArchive(); as p) {
      <div class="pms-modal-backdrop" (click)="cancelArchive()">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.propertyArchivedTitle }}</h2>
          <p>{{ format(msgs.propertyArchivedQ, { name: p.name }) }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelArchive()">{{ msgs.cancel }}</button>
            <button type="button" class="btn btn-primary" (click)="doArchive()" [disabled]="archiveSaving()">
              {{ msgs.archive }}
            </button>
          </div>
        </div>
      </div>
    }

    @if (confirmRestore(); as p) {
      <div class="pms-modal-backdrop" (click)="cancelRestore()">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.propertyRestoreTitle }}</h2>
          <p>{{ format(msgs.propertyRestoreQ, { name: p.name }) }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelRestore()">{{ msgs.cancel }}</button>
            <button type="button" class="btn btn-primary" (click)="doRestore()" [disabled]="restoreSaving()">
              {{ msgs.restore }}
            </button>
          </div>
        </div>
      </div>
    }
  `,
  styles: [`
    .pms-inline-form { display: inline-flex; gap: .5rem; align-items: center; margin-inline-start: .75rem; }
  `],
})
export class PropertyDetailComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;
  protected readonly formatDate = formatCairoDate;

  private readonly route = inject(ActivatedRoute);
  private readonly propertiesSvc = inject(PropertiesService);
  private readonly customFieldsSvc = inject(CustomFieldsService);
  private readonly session = inject(SessionService);

  protected readonly property = signal<PropertyDetail | null>(null);
  protected readonly fields = signal<CustomField[]>([]);
  protected readonly loading = signal(true);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly confirmArchive = signal<PropertyDetail | null>(null);
  protected readonly confirmRestore = signal<PropertyDetail | null>(null);
  protected readonly archiveSaving = signal(false);
  protected readonly restoreSaving = signal(false);
  protected readonly codeSaving = signal(false);

  protected readonly codeCtrl = new FormControl('', { nonNullable: true });

  protected readonly renderedValues = computed(() =>
    renderCustomValues(this.fields(), this.property()?.customValues ?? []),
  );

  protected readonly canEditCode = computed(() => this.session.isAdmin());

  ngOnInit(): void {
    const id = this.route.snapshot.paramMap.get('propertyId');
    if (!id) {
      this.loading.set(false);
      return;
    }
    this.refreshFields();
    this.refresh(id);
  }

  private refreshFields(): void {
    this.customFieldsSvc.listCustomFields('body').subscribe({
      next: (items) => this.fields.set(items),
      error: () => this.fields.set([]),
    });
  }

  private refresh(id: string): void {
    this.loading.set(true);
    this.propertiesSvc.getProperty({ propertyId: id }, 'body').subscribe({
      next: (p) => {
        this.property.set(p);
        this.codeCtrl.setValue(p.code);
        this.loading.set(false);
      },
      error: (err: ApiError) => {
        this.loading.set(false);
        this.errorMessage.set(err.message || this.msgs.propertyNotFound);
      },
    });
  }

  protected askArchive(): void {
    const p = this.property();
    if (p) this.confirmArchive.set(p);
  }
  protected cancelArchive(): void {
    this.confirmArchive.set(null);
  }
  protected doArchive(): void {
    const p = this.confirmArchive();
    if (!p) return;
    this.archiveSaving.set(true);
    this.propertiesSvc
      .archiveProperty({ propertyId: p.id, archivePropertyRequest: { version: p.version } }, 'body')
      .subscribe({
        next: (updated) => {
          this.archiveSaving.set(false);
          this.confirmArchive.set(null);
          this.property.set(updated);
          this.codeCtrl.setValue(updated.code);
        },
        error: (err: ApiError) => {
          this.archiveSaving.set(false);
          this.errorMessage.set(err.message || this.msgs.internalError);
        },
      });
  }

  protected askRestore(): void {
    const p = this.property();
    if (p) this.confirmRestore.set(p);
  }
  protected cancelRestore(): void {
    this.confirmRestore.set(null);
  }
  protected doRestore(): void {
    const p = this.confirmRestore();
    if (!p) return;
    this.restoreSaving.set(true);
    this.propertiesSvc
      .restoreProperty({ propertyId: p.id, archivePropertyRequest: { version: p.version } }, 'body')
      .subscribe({
        next: (updated) => {
          this.restoreSaving.set(false);
          this.confirmRestore.set(null);
          this.property.set(updated);
          this.codeCtrl.setValue(updated.code);
        },
        error: (err: ApiError) => {
          this.restoreSaving.set(false);
          this.errorMessage.set(err.message || this.msgs.internalError);
        },
      });
  }

  protected saveCode(): void {
    const p = this.property();
    if (!p) return;
    this.codeSaving.set(true);
    this.propertiesSvc
      .changePropertyCode({
        propertyId: p.id,
        changePropertyCodeRequest: { code: this.codeCtrl.value, version: p.version },
      }, 'body')
      .subscribe({
        next: (updated) => {
          this.codeSaving.set(false);
          this.property.set(updated);
          this.codeCtrl.setValue(updated.code);
        },
        error: (err: ApiError) => {
          this.codeSaving.set(false);
          this.errorMessage.set(err.message || this.msgs.internalError);
        },
      });
  }
}
