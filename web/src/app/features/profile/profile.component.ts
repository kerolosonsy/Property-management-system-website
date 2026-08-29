// web/src/app/features/profile/profile.component.ts
// The signed-in person's own details, and the place they change their own
// password (FR-011). Available to administrator and manager alike — changing
// your own password is not an administrative act.
//
// The change-password form itself is ChangePasswordComponent, reused here so
// the forced first-sign-in flow and this voluntary one cannot drift apart.

import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { SessionService } from '../../core/session.service';
import { ChangePasswordComponent } from '../sign-in/change-password/change-password.component';
import { ARABIC_MESSAGES } from '../../shared/messages';

@Component({
  selector: 'app-profile',
  standalone: true,
  imports: [CommonModule, ChangePasswordComponent],
  template: `
    <header class="pms-view-head">
      <div>
        <h1>{{ msgs.profile }}</h1>
        <div class="pms-crumb">{{ msgs.breadcrumbRoot }} / {{ msgs.profile }}</div>
      </div>
    </header>

    <div class="pms-page">
      @if (session.user(); as u) {
        <div class="card blueprint elev-sm pms-card">
          <i class="corner tl"></i><i class="corner tr"></i>
          <i class="corner bl"></i><i class="corner br"></i>
          <h2>{{ msgs.profileSubtitle }}</h2>
          <table class="table">
            <tbody>
              <tr><th scope="row">{{ msgs.displayName }}</th><td>{{ u.displayName }}</td></tr>
              <tr><th scope="row">{{ msgs.username }}</th><td>{{ u.username }}</td></tr>
              <tr><th scope="row">{{ msgs.role }}</th><td>{{ roleLabel(u.role) }}</td></tr>
            </tbody>
          </table>
        </div>
      }

      <app-change-password [embedded]="true"></app-change-password>
    </div>
  `,
})
export class ProfileComponent {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly session = inject(SessionService);

  protected roleLabel(role: string): string {
    return role === 'admin' ? this.msgs.roleAdmin : this.msgs.roleManager;
  }
}
