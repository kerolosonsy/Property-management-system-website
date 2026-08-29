// web/src/app/shared/western-digits.directive.ts
// Eastern-to-Western digit directive (FR-033).
//
// As the user types or pastes, any Eastern Arabic digit (٠-٩) in the value is
// replaced with the equivalent Western digit. Non-digit code points are left
// untouched, so surrounding Arabic letters stay as-is (research §8).

import { Directive, ElementRef, HostListener, inject, OnInit } from '@angular/core';

const EASTERN_DIGITS: Record<string, string> = {
  '٠': '0', '١': '1', '٢': '2', '٣': '3', '٤': '4',
  '٥': '5', '٦': '6', '٧': '7', '٨': '8', '٩': '9',
};

function convert(value: string): string {
  let out = '';
  let changed = false;
  for (const ch of value) {
    const mapped = EASTERN_DIGITS[ch];
    if (mapped !== undefined) {
      out += mapped;
      changed = true;
    } else {
      out += ch;
    }
  }
  return changed ? out : value;
}

@Directive({
  selector: '[appWesternDigits]',
  standalone: true,
})
export class WesternDigitsDirective implements OnInit {
  private readonly host = inject(ElementRef<HTMLInputElement | HTMLTextAreaElement>);

  ngOnInit(): void {
    const el = this.host.nativeElement;
    const initial = el.value;
    if (initial) {
      const mapped = convert(initial);
      if (mapped !== initial) {
        el.value = mapped;
      }
    }
  }

  @HostListener('input', ['$event'])
  onInput(event: Event): void {
    const el = event.target as HTMLInputElement | HTMLTextAreaElement;
    const original = el.value;
    const mapped = convert(original);
    if (mapped !== original) {
      // Preserve the caret position relative to the start.
      const start = el.selectionStart ?? original.length;
      const end = el.selectionEnd ?? original.length;
      el.value = mapped;
      try {
        el.setSelectionRange(start, end);
      } catch {
        // Some input types (number, email) reject setSelectionRange.
      }
    }
  }
}