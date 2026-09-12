// web/src/app/shared/close-on-escape.directive.ts
// Closes the hosting modal when the user presses Escape. Applied to every
// .pms-modal-backdrop so all five modal users (archive/restore confirm,
// attachment remove/promote/viewer, custom-field remove, lookup remove) gain
// keyboard parity with the existing backdrop-click-to-close behaviour
// (Constitution II — keyboard equivalence).

import { Directive, HostListener, Input } from '@angular/core';

@Directive({
  selector: '[pmsCloseOnEscape]',
  standalone: true,
})
export class CloseOnEscapeDirective {
  @Input('pmsCloseOnEscape') closeFn!: () => void;

  @HostListener('document:keydown.escape')
  onEscape(): void {
    if (this.closeFn) {
      this.closeFn();
    }
  }
}