// web/src/app/features/properties/attachment-text.component.ts
// US6 — view extracted text, correct it, request re-extraction.

import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { WesternDigitsDirective } from '../../shared/western-digits.directive';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { AttachmentTextService } from '../../api/api/attachment-text.service';
import { AttachmentText } from '../../api/model/attachment-text.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';

@Component({
  selector: 'app-attachment-text',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink, WesternDigitsDirective],
  template: `
    <section class="pms-page">
      @if (errorMessage(); as msg) {
        <div class="pms-error-banner">{{ msg }}</div>
      }

      @if (data(); as d) {
        <header class="pms-view-head">
          <div>
            <h1>{{ msgs.attachmentTextTitle }}</h1>
            <div class="pms-crumb">{{ msgs.attachments }}</div>
          </div>
          <div class="pms-toolbar-spacer"></div>
          <a [routerLink]="['/properties', propertyId]" class="btn btn-secondary">{{ msgs.backToList }}</a>
        </header>

        <div class="card blueprint elev-sm">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
          @if (d.isCorrected) {
            <p class="pms-note">{{ format(msgs.attachmentTextIsCorrected, { when: formatDate(d.correctedAt) }) }}</p>
          }
          @if (d.truncated) {
            <p class="pms-note">{{ msgs.extractTruncated }}</p>
          }

          @if (editing()) {
            <textarea class="input" rows="12" [formControl]="editCtrl" appWesternDigits></textarea>
            <div class="pms-modal-actions">
              <button type="button" class="btn btn-secondary" (click)="cancelEdit()">{{ msgs.cancel }}</button>
              <button type="button" class="btn btn-primary" (click)="saveEdit()" [disabled]="saving()">
                {{ msgs.attachmentTextSaveCorrection }}
              </button>
            </div>
          } @else {
            <pre class="pms-text-block">{{ d.body ?? msgs.attachmentTextEmpty }}</pre>
            <div class="pms-modal-actions">
              <button type="button" class="btn btn-secondary" (click)="startEdit()">
                {{ msgs.attachmentTextCorrect }}
              </button>
              <button type="button" class="btn btn-secondary" (click)="askReextract()" [disabled]="reextracting()">
                {{ msgs.attachmentTextReextract }}
              </button>
            </div>
          }
        </div>
      }
    </section>
  `,
  styles: [`
    .pms-text-block {
      background: var(--pms-surface);
      padding: 1rem;
      border-radius: .5rem;
      white-space: pre-wrap;
      word-break: break-word;
      font-family: inherit;
      border: 1px solid var(--pms-border);
    }
  `],
})
export class AttachmentTextComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;

  private readonly route = inject(ActivatedRoute);
  private readonly svc = inject(AttachmentTextService);

  protected readonly propertyId = this.route.snapshot.paramMap.get('propertyId') ?? '';
  protected readonly attachmentId = this.route.snapshot.paramMap.get('attachmentId') ?? '';

  protected readonly data = signal<AttachmentText | null>(null);
  protected readonly loading = signal(true);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly editing = signal(false);
  protected readonly saving = signal(false);
  protected readonly reextracting = signal(false);

  protected readonly editCtrl = new FormControl('', { nonNullable: true });

  ngOnInit(): void {
    this.refresh();
  }

  private refresh(): void {
    this.loading.set(true);
    this.svc.getAttachmentText({ attachmentId: this.attachmentId }, 'body').subscribe({
      next: (t) => {
        this.data.set(t);
        this.editCtrl.setValue(t.body ?? '');
        this.loading.set(false);
      },
      error: (err: ApiError) => {
        this.errorMessage.set(err.message || this.msgs.internalError);
        this.loading.set(false);
      },
    });
  }

  protected startEdit(): void {
    this.editing.set(true);
  }
  protected cancelEdit(): void {
    this.editing.set(false);
  }
  protected saveEdit(): void {
    this.saving.set(true);
    this.svc.correctAttachmentText(
      { attachmentId: this.attachmentId, correctAttachmentTextRequest: { body: this.editCtrl.value } },
      'body',
    ).subscribe({
      next: () => {
        this.saving.set(false);
        this.editing.set(false);
        this.refresh();
      },
      error: (err: ApiError) => {
        this.saving.set(false);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  protected askReextract(): void {
    this.reextracting.set(true);
    this.svc.reextractAttachmentText({ attachmentId: this.attachmentId }, 'body').subscribe({
      next: () => {
        this.reextracting.set(false);
        this.refresh();
      },
      error: (err: ApiError) => {
        this.reextracting.set(false);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  protected formatDate(iso: string | null | undefined): string {
    if (!iso) return '';
    try {
      return new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso));
    } catch {
      return iso;
    }
  }
}
