// web/src/app/shared/autocomplete-input.component.ts
// One reusable type-ahead combobox for every property search filter and for
// `autocomplete` custom fields (Constitution II — RTL-native, logical CSS only,
// Arabic-first). Two modes:
//
//   • free-text mode — pass `suggest`: a function the component calls with the
//     typed text (debounced ~250ms, superseded calls cancelled through
//     switchMap). The model is whatever the user typed; a value that is not in
//     the suggestions is always allowed and is what gets submitted.
//   • options mode — pass `options`: a fixed {value, label} list the component
//     filters in the browser with the same Arabic normalisation the server
//     applies. The model is the accepted option's value, or '' while the user
//     is still typing (the filter constrains nothing until a choice is made).
//
// Sensitive custom fields never render this component at all: their callers
// keep the plain input, so nothing about them is ever suggested or requested.

import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  OnDestroy,
  effect,
  forwardRef,
  inject,
  input,
  signal,
} from '@angular/core';
import { ControlValueAccessor, NG_VALUE_ACCESSOR } from '@angular/forms';
import { Subject, Observable, of } from 'rxjs';
import {
  catchError,
  debounceTime,
  distinctUntilChanged,
  switchMap,
  takeUntil,
} from 'rxjs/operators';

/** A pickable option in options mode (an id + its Arabic label). */
export interface AutocompleteOption {
  value: string;
  label: string;
}

/** What free-text mode calls to obtain suggestions for the typed text. */
export type AutocompleteSuggestFn = (q: string) => Observable<string[]>;

// The same Eastern→Western digit mapping the western-digits directive applies
// to every other text input, so suggestions typed with Eastern digits still
// match (research §8: converted client-side on entry).
const EASTERN_DIGITS: Record<string, string> = {
  '٠': '0',
  '١': '1',
  '٢': '2',
  '٣': '3',
  '٤': '4',
  '٥': '5',
  '٦': '6',
  '٧': '7',
  '٨': '8',
  '٩': '9',
};

function westernDigits(s: string): string {
  return s.replace(/[٠-٩]/g, (c) => EASTERN_DIGITS[c] ?? c);
}

// A light client-side mirror of the server's identity.Canonical: digits, case,
// diacritics, tatweel, and the letter variants the normaliser unifies. Used
// only to narrow the already-loaded lookup lists in the browser — the server
// remains the authority wherever a query is actually answered server-side.
function canonicalText(s: string): string {
  return westernDigits(s)
    .toLowerCase()
    .replace(/[\u064B-\u065F\u0670\u0640]/g, '')
    .replace(/[\u0622\u0623\u0625\u0671]/g, '\u0627')
    .replace(/\u0649/g, '\u064A')
    .replace(/\u0629/g, '\u0647')
    .trim();
}

@Component({
  selector: 'app-autocomplete-input',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  providers: [
    {
      provide: NG_VALUE_ACCESSOR,
      useExisting: forwardRef(() => AutocompleteInputComponent),
      multi: true,
    },
  ],
  host: {
    '(document:click)': 'onDocumentClick($event)',
  },
  template: `
    <span class="pms-ac">
      <input
        class="input"
        type="text"
        [id]="inputId()"
        [attr.placeholder]="placeholder() || null"
        [attr.aria-label]="ariaLabel() || null"
        role="combobox"
        [attr.aria-autocomplete]="'list'"
        [attr.aria-expanded]="listOpen()"
        [attr.aria-controls]="listboxId"
        [attr.aria-activedescendant]="activeDescendant()"
        [attr.disabled]="disabled() ? '' : null"
        autocomplete="off"
        [value]="text()"
        (input)="onInput($event)"
        (keydown)="onKeydown($event)"
        (blur)="onBlur()"
      />
      @if (listOpen()) {
        <ul class="pms-ac-list" [id]="listboxId" role="listbox">
          @for (item of visible(); track item) {
            <li
              role="option"
              [id]="listboxId + '-opt-' + $index"
              [class.pms-ac-active]="$index === activeIndex()"
              (mousedown)="$event.preventDefault(); pick($index)"
            >
              {{ labelOf(item) }}
            </li>
          }
        </ul>
      }
    </span>
  `,
  styles: [
    `
      .pms-ac {
        position: relative;
        display: inline-block;
        inline-size: 100%;
      }

      .pms-ac-list {
        position: absolute;
        inset-inline-start: 0;
        inset-inline-end: 0;
        inset-block-start: calc(100% + var(--space-1));
        z-index: 30;
        margin: 0;
        padding: 0;
        list-style: none;
        background: var(--color-bg);
        border: 1px solid var(--color-divider);
        border-radius: var(--radius-md);
        box-shadow: var(--shadow-md);
        max-block-size: 240px;
        overflow-y: auto;
      }

      .pms-ac-list li {
        padding: var(--space-2) var(--space-3);
        font-size: 13px;
        cursor: pointer;
      }

      .pms-ac-list li.pms-ac-active {
        background: var(--color-accent-100);
      }
    `,
  ],
})
export class AutocompleteInputComponent implements ControlValueAccessor, OnDestroy {
  /** Id of the input; labels bind to it with `for`. */
  readonly inputId = input.required<string>();
  readonly placeholder = input('');
  readonly ariaLabel = input('');

  /** Free-text mode: fetch suggestions for the typed text. */
  readonly suggest = input<AutocompleteSuggestFn | null>(null);
  /** Options mode: the full pickable list, filtered in the browser. */
  readonly options = input<AutocompleteOption[] | null>(null);

  readonly disabled = signal(false);
  protected readonly text = signal('');
  protected readonly suggestions = signal<string[]>([]);
  protected readonly open = signal(false);
  protected readonly activeIndex = signal(-1);

  /** Model value in options mode (the accepted id, '' while typing). */
  private model = '';
  private readonly host = inject(ElementRef<HTMLElement>);
  private readonly query$ = new Subject<string>();
  private readonly destroy$ = new Subject<void>();

  constructor() {
    // Debounce so a request is not fired per keystroke; switchMap cancels the
    // superseded request when typing continues. The suggestion stream is
    // driven only by real typing — accepting a suggestion never refetches.
    this.query$
      .pipe(
        debounceTime(250),
        distinctUntilChanged(),
        switchMap((q) => {
          const fetcher = this.suggest();
          if (!fetcher) return of<string[]>([]);
          return fetcher(q).pipe(catchError(() => of<string[]>([])));
        }),
        takeUntil(this.destroy$),
      )
      .subscribe((items) => {
        this.suggestions.set(items);
        this.activeIndex.set(-1);
        this.open.set(items.length > 0);
      });

    // Options mode: a saved id can be written into the form before the lookup
    // list arrives (the detail loads and the lookups load in parallel). When
    // the list lands, resolve the label for the already-set model.
    effect(() => {
      const opts = this.options();
      if (!opts || this.model === '') return;
      const label = this.displayForModel(this.model);
      if (label !== '' && label !== this.text()) this.text.set(label);
    });
  }

  ngOnDestroy(): void {
    this.destroy$.next();
    this.destroy$.complete();
  }

  // ── ControlValueAccessor ────────────────────────────────────────────────

  writeValue(value: unknown): void {
    const v = value == null ? '' : String(value);
    this.model = v;
    this.text.set(this.displayForModel(v));
    this.open.set(false);
    this.activeIndex.set(-1);
  }

  registerOnChange(fn: (v: string) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(isDisabled: boolean): void {
    this.disabled.set(isDisabled);
  }

  private onChange: (v: string) => void = () => {};
  private onTouched: () => void = () => {};

  // ── Interaction ─────────────────────────────────────────────────────────

  protected get listboxId(): string {
    return this.inputId() + '-listbox';
  }

  /** Options mode resolves the model back to its label for display. */
  private displayForModel(v: string): string {
    if (v === '') return '';
    const opts = this.options();
    if (!opts) return v;
    return opts.find((o) => o.value === v)?.label ?? '';
  }

  protected visible(): string[] {
    const opts = this.options();
    if (!opts) return this.suggestions();
    // Options mode narrows the loaded list in the browser by the same Arabic
    // normalisation the server applies — never a request.
    const needle = canonicalText(this.text());
    return opts
      .filter((o) => needle === '' || canonicalText(o.label).includes(needle))
      .map((o) => o.value)
      .slice(0, 10);
  }

  protected labelOf(item: string): string {
    const opts = this.options();
    if (!opts) return item;
    return opts.find((o) => o.value === item)?.label ?? item;
  }

  protected listOpen(): boolean {
    return this.open() && this.visible().length > 0 && !this.disabled();
  }

  protected activeDescendant(): string | null {
    if (this.activeIndex() < 0 || !this.listOpen()) return null;
    return `${this.listboxId}-opt-${this.activeIndex()}`;
  }

  protected onInput(ev: Event): void {
    if (this.disabled()) return;
    const raw = (ev.target as HTMLInputElement).value;
    const v = westernDigits(raw);
    if (v !== raw) (ev.target as HTMLInputElement).value = v;
    this.text.set(v);
    this.activeIndex.set(-1);
    if (this.options()) {
      // Options mode: no choice has been made yet, so the filter constrains
      // nothing — the model is '' until an option is accepted. Free-text mode
      // submits exactly what is typed, suggestion or no suggestion.
      this.model = '';
      this.onChange('');
      this.open.set(this.visible().length > 0);
    } else {
      this.model = v;
      this.onChange(v);
      this.query$.next(v);
    }
  }

  protected pick(index: number): void {
    const item = this.visible()[index];
    if (item === undefined) return;
    const opts = this.options();
    if (opts) {
      const chosen = opts.find((o) => o.value === item);
      if (!chosen) return;
      this.model = chosen.value;
      this.text.set(chosen.label);
      this.onChange(chosen.value);
    } else {
      this.model = item;
      this.text.set(item);
      this.onChange(item);
    }
    this.open.set(false);
    this.activeIndex.set(-1);
  }

  protected onKeydown(ev: KeyboardEvent): void {
    const items = this.visible();
    if (ev.key === 'Escape') {
      if (this.open()) {
        // Handled here on the input rather than through the shared
        // close-on-escape directive: that one listens on the document for
        // modal backdrops, where a combobox Escape must not also fire it.
        ev.preventDefault();
        this.open.set(false);
        this.activeIndex.set(-1);
      }
      return;
    }
    if (!this.open() || items.length === 0) {
      if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
        ev.preventDefault();
        this.open.set(this.visible().length > 0);
      }
      return;
    }
    switch (ev.key) {
      case 'ArrowDown':
        ev.preventDefault();
        this.activeIndex.set((this.activeIndex() + 1) % items.length);
        break;
      case 'ArrowUp':
        ev.preventDefault();
        this.activeIndex.set((this.activeIndex() - 1 + items.length) % items.length);
        break;
      case 'Enter':
        if (this.activeIndex() >= 0) {
          ev.preventDefault();
          this.pick(this.activeIndex());
        } else {
          // Nothing highlighted: the typed value stands (free-text mode) or
          // the empty selection stands (options mode); close the list.
          this.open.set(false);
        }
        break;
      case 'Tab':
        this.open.set(false);
        this.activeIndex.set(-1);
        break;
    }
  }

  protected onBlur(): void {
    this.onTouched();
    this.open.set(false);
    this.activeIndex.set(-1);
  }

  protected onDocumentClick(ev: MouseEvent): void {
    if (!this.host.nativeElement.contains(ev.target as Node)) {
      this.open.set(false);
      this.activeIndex.set(-1);
    }
  }
}
