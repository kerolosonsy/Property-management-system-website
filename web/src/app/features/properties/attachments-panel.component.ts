// web/src/app/features/properties/attachments-panel.component.ts
// US1, US2, US3 — the per-property attachments list, the upload form,
// description edit, sensitivity promotion, removal. The viewer is a
// separate component (attachment-viewer).

import { Component, OnInit, Input, inject, signal } from '@angular/core';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';
import { CommonModule } from '@angular/common';
import { FormsModule, FormControl, ReactiveFormsModule } from '@angular/forms';
import { WesternDigitsDirective } from '../../shared/western-digits.directive';
import { Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { AttachmentsService } from '../../api/api/attachments.service';
import { Attachment } from '../../api/model/attachment.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { apiBasePath } from '../../core/api-base';
import { CloseOnEscapeDirective } from '../../shared/close-on-escape.directive';

@Component({
  selector: 'app-attachments-panel',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule, WesternDigitsDirective, CloseOnEscapeDirective],
  template: `
    <div class="card blueprint elev-sm">
      <i class="corner tl"></i><i class="corner tr"></i>
      <i class="corner bl"></i><i class="corner br"></i>
      <h2>{{ msgs.attachments }}</h2>

      @if (errorMessage(); as msg) {
        <div class="pms-error-banner">{{ msg }}</div>
      }

      <form (ngSubmit)="upload()" class="pms-attachment-form">
        <div class="field pms-field">
          <label for="att-desc">{{ msgs.attachmentDescription }}</label>
          <input id="att-desc" class="input" type="text"
                 [formControl]="descriptionCtrl" appWesternDigits
                 [placeholder]="msgs.attachmentDescriptionHint" />
          @if (descriptionError()) {
            <div class="pms-field-error">{{ descriptionError() }}</div>
          }
        </div>
        <div class="field pms-field">
          <label for="att-file">{{ msgs.attachmentFile }}</label>
          <input id="att-file" type="file" (change)="onFilePicked($event)" />
          @if (pickedFile(); as f) {
            <div class="pms-note">{{ format(msgs.attachmentFileChosen, { name: f.name }) }}</div>
          }
          @if (fileError()) {
            <div class="pms-field-error">{{ fileError() }}</div>
          }
        </div>
        <div class="field pms-field pms-field-checkbox">
          <label>
            <input type="checkbox" [formControl]="sensitiveCtrl" />
            {{ msgs.attachmentSensitive }}
          </label>
          <div class="pms-note">{{ msgs.attachmentSensitiveHint }}</div>
        </div>
        <div class="pms-attachment-form-actions">
          <button type="submit" class="btn btn-primary" [disabled]="uploading()">
            {{ msgs.attachmentUpload }}
          </button>
        </div>
      </form>

      @if (loading()) {
        <p class="pms-empty">{{ msgs.loading }}</p>
      } @else if (attachments().length === 0) {
        <p class="pms-empty">{{ msgs.attachmentsEmpty }}</p>
      } @else {
        <ul class="pms-attachment-list">
          @for (a of attachments(); track a.id) {
            <li class="pms-attachment-item">
              <div class="pms-attachment-meta">
                <div class="pms-attachment-desc">
                  @if (editingDescription()?.id === a.id) {
                    <form (ngSubmit)="saveDescription(a)" class="pms-inline-form">
                      <input class="input" type="text" [formControl]="editDescriptionCtrl" appWesternDigits />
                      <button class="btn btn-secondary" type="submit" [disabled]="editingSaving()">
                        {{ msgs.attachmentSaveDescription }}
                      </button>
                      <button type="button" class="btn btn-secondary" (click)="cancelEditDescription()">
                        {{ msgs.cancel }}
                      </button>
                    </form>
                  } @else {
                    {{ a.description }}
                  }
                </div>
                <div class="pms-attachment-info">
                  <span>{{ a.originalFilename }}</span>
                  <span class="pms-attachment-size">{{ formatBytes(a.byteSize) }}</span>
                  <span>{{ formatDate(a.createdAt) }} — {{ a.createdBy }}</span>
                  @if (a.isSensitive) {
                    <span class="pms-pill pms-pill-warn">{{ msgs.attachmentSensitive }}</span>
                  }
                  <span class="pms-pill" [class]="statePillClass(a.extractState)">
                    {{ extractLabel(a.extractState) }}
                  </span>
                  @if (a.extractError) {
                    <span class="pms-note">{{ extractReasonLabel(a.extractError) }}</span>
                  }
                </div>
              </div>
              <div class="pms-attachment-actions">
                @if (canViewInline(a.contentType)) {
                  <button type="button" class="btn btn-secondary" (click)="viewInline(a)">
                    {{ msgs.attachmentView }}
                  </button>
                }
                <button type="button" class="btn btn-secondary" (click)="download(a)">
                  {{ msgs.attachmentDownload }}
                </button>
                <button type="button" class="btn btn-secondary" (click)="askEditDescription(a)">
                  {{ msgs.attachmentEditDescription }}
                </button>
                @if (canOpenText(a)) {
                  <button type="button" class="btn btn-secondary" (click)="openText(a)">
                    {{ msgs.attachmentOpenText }}
                  </button>
                }
                @if (!a.isSensitive) {
                  <button type="button" class="btn btn-secondary" (click)="askPromote(a)">
                    {{ msgs.attachmentPromoteSensitive }}
                  </button>
                }
                <button type="button" class="btn btn-secondary" (click)="askRemove(a)">
                  {{ msgs.attachmentRemove }}
                </button>
              </div>
            </li>
          }
        </ul>
      }
    </div>

    @if (confirmRemove(); as a) {
      <div class="pms-modal-backdrop" (click)="cancelRemove()" [pmsCloseOnEscape]="cancelRemove">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.attachmentRemoveTitle }}</h2>
          <p>{{ format(msgs.attachmentRemoveConfirm, { name: a.description }) }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelRemove()">{{ msgs.cancel }}</button>
            <button type="button" class="btn btn-primary" (click)="doRemove()" [disabled]="removing()">
              {{ msgs.attachmentRemove }}
            </button>
          </div>
        </div>
      </div>
    }

    @if (confirmPromote(); as a) {
      <div class="pms-modal-backdrop" (click)="cancelPromote()" [pmsCloseOnEscape]="cancelPromote">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.attachmentPromoteTitle }}</h2>
          <p>{{ msgs.attachmentPromoteWarning }}</p>
          <p>{{ format(msgs.attachmentPromoteConfirm, { name: a.description }) }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelPromote()">{{ msgs.cancel }}</button>
            <button type="button" class="btn btn-primary" (click)="doPromote()" [disabled]="promoting()">
              {{ msgs.attachmentPromoteAction }}
            </button>
          </div>
        </div>
      </div>
    }

    @if (viewerState(); as vs) {
      <div class="pms-modal-backdrop" (click)="closeViewer()" [pmsCloseOnEscape]="closeViewer">
        <div class="pms-modal pms-modal-wide" (click)="$event.stopPropagation()">
          <button type="button" class="btn btn-secondary" (click)="closeViewer()">{{ msgs.cancel }}</button>
          @switch (vs.kind) {
            @case ('image') {
              <img [src]="vs.src" alt="" class="pms-viewer-image" />
            }
            @case ('pdf') {
              <iframe [src]="vs.safeSrc" class="pms-viewer-pdf"></iframe>
            }
          }
        </div>
      </div>
    }
  `,
  styles: [`
    .pms-attachment-form { display: grid; gap: .75rem; margin-block-end: 1rem; }
    .pms-attachment-form-actions { display: flex; gap: .5rem; justify-content: flex-end; }
    .pms-attachment-list { list-style: none; padding: 0; margin: 0; display: grid; gap: .5rem; }
    .pms-attachment-item {
      display: flex; gap: 1rem; justify-content: space-between; align-items: center;
      padding: .75rem 1rem; border: 1px solid var(--pms-border); border-radius: .5rem;
      background: var(--pms-surface);
    }
    .pms-attachment-meta { display: grid; gap: .25rem; flex: 1; }
    .pms-attachment-desc { font-weight: 600; }
    .pms-attachment-info { display: flex; gap: .5rem; flex-wrap: wrap; align-items: center; color: var(--pms-muted); font-size: .85rem; }
    .pms-attachment-actions { display: flex; gap: .25rem; flex-wrap: wrap; }
    .pms-attachment-size { font-variant-numeric: tabular-nums; }
    .pms-field-error { color: var(--pms-error); font-size: .85rem; }
    .pms-viewer-image { max-width: 100%; max-height: 70vh; object-fit: contain; margin: auto; }
    .pms-viewer-pdf { width: 100%; height: 70vh; border: 0; }
    .pms-pill-pending { background: var(--pms-info); color: white; padding: 0 .5rem; border-radius: 999px; }
    .pms-pill-done { background: var(--pms-success); color: white; padding: 0 .5rem; border-radius: 999px; }
    .pms-pill-empty { background: var(--pms-muted); color: white; padding: 0 .5rem; border-radius: 999px; }
    .pms-pill-not_eligible { background: var(--pms-warn); color: white; padding: 0 .5rem; border-radius: 999px; }
    .pms-pill-too_large { background: var(--pms-error); color: white; padding: 0 .5rem; border-radius: 999px; }
    .pms-pill-failed { background: var(--pms-error); color: white; padding: 0 .5rem; border-radius: 999px; }
  `],
})
export class AttachmentsPanelComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;

  @Input() propertyId!: string;

  private readonly svc = inject(AttachmentsService);
  private readonly router = inject(Router);
  private readonly sanitizer = inject(DomSanitizer);

  protected readonly attachments = signal<Attachment[]>([]);
  protected readonly loading = signal(true);
  protected readonly errorMessage = signal<string | null>(null);

  protected readonly descriptionCtrl = new FormControl('', { nonNullable: true });
  protected readonly sensitiveCtrl = new FormControl(false, { nonNullable: true });
  protected readonly pickedFile = signal<File | null>(null);
  protected readonly descriptionError = signal<string | null>(null);
  protected readonly fileError = signal<string | null>(null);
  protected readonly uploading = signal(false);

  protected readonly editingDescription = signal<Attachment | null>(null);
  protected readonly editDescriptionCtrl = new FormControl('', { nonNullable: true });
  protected readonly editingSaving = signal(false);

  protected readonly confirmRemove = signal<Attachment | null>(null);
  protected readonly removing = signal(false);

  protected readonly confirmPromote = signal<Attachment | null>(null);
  protected readonly promoting = signal(false);

  protected readonly viewerState = signal<{ kind: 'image' | 'pdf'; src: string; safeSrc?: SafeResourceUrl } | null>(null);

  ngOnInit(): void {
    this.refresh();
  }

  protected refresh(): void {
    this.loading.set(true);
    this.svc.listAttachments({ propertyId: this.propertyId }, 'body').subscribe({
      next: (items) => {
        this.attachments.set(items);
        this.loading.set(false);
      },
      error: (err: ApiError) => {
        this.errorMessage.set(err.message || this.msgs.internalError);
        this.loading.set(false);
      },
    });
  }

  protected onFilePicked(ev: Event): void {
    const input = ev.target as HTMLInputElement;
    this.pickedFile.set(input.files && input.files.length > 0 ? input.files[0] : null);
    this.fileError.set(null);
  }

  protected async upload(): Promise<void> {
    // Everything runs inside the try. A throw before the request — a null
    // control, a bad reference — would otherwise reject the ngSubmit handler
    // and leave the user staring at a form that does nothing, with no request
    // and no visible reason.
    try {
      await this.doUpload();
    } catch (err) {
      console.error('[attachments] upload failed before the request', err);
      this.errorMessage.set(
        err instanceof Error ? `${err.name}: ${err.message}` : String(err),
      );
      this.uploading.set(false);
    }
  }

  private async doUpload(): Promise<void> {
    const file = this.pickedFile();
    const desc = this.descriptionCtrl.value.trim();
    if (desc.length < 2 || desc.length > 200) {
      this.descriptionError.set(this.msgs.attachmentDescriptionLength);
      return;
    }
    if (!file) {
      this.fileError.set(this.msgs.attachmentMissingDescription);
      return;
    }
    this.uploading.set(true);
    this.errorMessage.set(null);
    try {
      await firstValueFrom(this.svc.uploadAttachment(
        { propertyId: this.propertyId, file, description: desc, isSensitive: this.sensitiveCtrl.value },
        'body',
      ));
      this.descriptionCtrl.setValue('');
      this.sensitiveCtrl.setValue(false);
      this.pickedFile.set(null);
      this.refresh();
    } catch (err) {
      const apiErr = err as ApiError;
      this.errorMessage.set(apiErr?.message || this.msgs.internalError);
    } finally {
      this.uploading.set(false);
    }
  }

  protected viewInline(a: Attachment): void {
    const url = `${apiBasePath()}/attachments/${a.id}/content?disposition=inline`;
    if (a.contentType === 'application/pdf') {
      // An iframe's src is a RESOURCE URL context. Angular refuses a plain
      // string there and the frame stays blank, so the URL this component
      // built itself is explicitly marked trusted.
      this.viewerState.set({
        kind: 'pdf',
        src: url,
        safeSrc: this.sanitizer.bypassSecurityTrustResourceUrl(url),
      });
    } else if (a.contentType.startsWith('image/')) {
      this.viewerState.set({ kind: 'image', src: url });
    }
  }

  protected closeViewer(): void {
    this.viewerState.set(null);
  }

  protected download(a: Attachment): void {
    window.location.href = `${apiBasePath()}/attachments/${a.id}/content?disposition=attachment`;
  }

  protected askEditDescription(a: Attachment): void {
    this.editingDescription.set(a);
    this.editDescriptionCtrl.setValue(a.description);
  }

  protected cancelEditDescription(): void {
    this.editingDescription.set(null);
  }

  protected saveDescription(a: Attachment): void {
    const d = this.editDescriptionCtrl.value.trim();
    if (d.length < 2 || d.length > 200) {
      this.errorMessage.set(this.msgs.attachmentDescriptionLength);
      return;
    }
    this.editingSaving.set(true);
    this.svc.updateAttachment({ attachmentId: a.id, updateAttachmentRequest: { description: d } }, 'body').subscribe({
      next: (updated) => {
        this.editingSaving.set(false);
        this.editingDescription.set(null);
        this.replace(updated);
      },
      error: (err: ApiError) => {
        this.editingSaving.set(false);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  protected askRemove(a: Attachment): void {
    this.confirmRemove.set(a);
  }
  protected cancelRemove(): void {
    this.confirmRemove.set(null);
  }
  protected doRemove(): void {
    const a = this.confirmRemove();
    if (!a) return;
    this.removing.set(true);
    this.svc.deleteAttachment({ attachmentId: a.id }, 'body').subscribe({
      next: () => {
        this.removing.set(false);
        this.confirmRemove.set(null);
        this.attachments.update((items) => items.filter((i) => i.id !== a.id));
      },
      error: (err: ApiError) => {
        this.removing.set(false);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  protected askPromote(a: Attachment): void {
    this.confirmPromote.set(a);
  }
  protected cancelPromote(): void {
    this.confirmPromote.set(null);
  }
  protected doPromote(): void {
    const a = this.confirmPromote();
    if (!a) return;
    this.promoting.set(true);
    this.svc.updateAttachment({ attachmentId: a.id, updateAttachmentRequest: { isSensitive: true } }, 'body').subscribe({
      next: (updated) => {
        this.promoting.set(false);
        this.confirmPromote.set(null);
        this.replace(updated);
      },
      error: (err: ApiError) => {
        this.promoting.set(false);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  protected openText(a: Attachment): void {
    this.router.navigate(['/properties', this.propertyId, 'attachments', a.id, 'text']);
  }

  protected canViewInline(contentType: string): boolean {
    return contentType === 'application/pdf' || contentType.startsWith('image/');
  }
  protected canOpenText(a: Attachment): boolean {
    return a.extractState === 'done' || a.extractState === 'empty' || a.extractState === 'failed' || a.extractState === 'not_eligible';
  }

  protected extractLabel(state: string): string {
    switch (state) {
      case 'pending': return this.msgs.extractStatePending;
      case 'done': return this.msgs.extractStateDone;
      case 'empty': return this.msgs.extractStateEmpty;
      case 'not_eligible': return this.msgs.extractStateNotEligible;
      case 'too_large': return this.msgs.extractStateTooLarge;
      case 'failed': return this.msgs.extractStateFailed;
      default: return state;
    }
  }

  protected extractReasonLabel(reason: string | null | undefined): string {
    if (!reason) return '';
    switch (reason) {
      case 'tesseract-missing': return this.msgs.extractReasonTesseractMissing;
      case 'tesseract-no-arabic': return this.msgs.extractReasonTesseractNoArabic;
      case 'poppler-missing': return this.msgs.extractReasonPopplerMissing;
      case 'legacy-office-format': return this.msgs.extractReasonLegacyOffice;
      case 'unsupported-type': return this.msgs.extractReasonUnsupportedType;
      case 'text-too-large': return this.msgs.extractReasonTextTooLarge;
      default: return reason;
    }
  }

  protected statePillClass(state: string): string {
    return `pms-pill-${state}`;
  }

  protected formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / 1024 / 1024).toFixed(1)} MB`;
  }

  protected formatDate(iso: string): string {
    try {
      return new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso));
    } catch {
      return iso;
    }
  }

  private replace(updated: Attachment): void {
    this.attachments.update((items) => items.map((i) => (i.id === updated.id ? updated : i)));
  }
}
