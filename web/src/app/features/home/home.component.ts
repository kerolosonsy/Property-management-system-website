// web/src/app/features/home/home.component.ts
// The landing screen for a signed-in user who is not an administrator.
//
// This feature gives managers no screen of their own — accounts and records are
// both administrator-only — so without a destination a signed-in manager has
// nowhere to be and the router bounces them back to sign-in. This is
// deliberately minimal: identity, and nothing invented beyond it.

import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { SessionService } from '../../core/session.service';
import { ARABIC_MESSAGES } from '../../shared/messages';

@Component({
  selector: 'app-home',
  standalone: true,
  imports: [CommonModule],
  template: `
    <section class="pms-card">
      <h1>{{ msgs.home }}</h1>
      <p>{{ msgs.homeWelcome }}</p>
      @if (session.user(); as u) {
        <p>
          <strong>{{ u.displayName }}</strong>
          <span class="pms-muted"> · {{ roleLabel(u.role) }}</span>
        </p>
      }
      @if (!session.isAdmin()) {
        <p class="pms-muted">{{ msgs.homeNoScreens }}</p>
      }
    </section>
  `,
})
export class HomeComponent {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly session = inject(SessionService);

  protected roleLabel(role: string): string {
    return role === 'admin' ? this.msgs.roleAdmin : this.msgs.roleManager;
  }
}
