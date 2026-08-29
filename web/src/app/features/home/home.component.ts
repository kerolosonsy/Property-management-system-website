// web/src/app/features/home/home.component.ts
// Landing screen for a signed-in user who is not an administrator.
//
// Every other screen in feature 001 is administrator-only: accounts and the
// records view both require the admin role. Without a destination a signed-in
// manager has nowhere to be and the router sends them back to sign-in.
//
// It is deliberately small. Property, unit, lease, tenant and payment work
// belongs to later specifications and is not invented here — the only action a
// manager actually has in this feature is changing their own password
// (FR-011).

import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { SessionService } from '../../core/session.service';
import { ARABIC_MESSAGES } from '../../shared/messages';

@Component({
  selector: 'app-home',
  standalone: true,
  imports: [CommonModule, RouterLink],
  template: `
    <section class="pms-page">
      <div class="card" style="max-inline-size: 32rem; margin-inline: auto;">
        <h1>{{ msgs.home }}</h1>
        <p>{{ msgs.homeWelcome }}</p>

        @if (session.user(); as u) {
          <table class="table">
            <tbody>
              <tr>
                <th scope="row">{{ msgs.displayName }}</th>
                <td>{{ u.displayName }}</td>
              </tr>
              <tr>
                <th scope="row">{{ msgs.username }}</th>
                <td>{{ u.username }}</td>
              </tr>
              <tr>
                <th scope="row">{{ msgs.role }}</th>
                <td>{{ roleLabel(u.role) }}</td>
              </tr>
            </tbody>
          </table>
        }

        <p>
          <a routerLink="/profile" class="btn btn-secondary">
            {{ msgs.profile }}
          </a>
        </p>

        @if (!session.isAdmin()) {
          <p class="text-muted">{{ msgs.homeNoScreens }}</p>
        }
      </div>
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
