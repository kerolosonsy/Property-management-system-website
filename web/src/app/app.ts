import { Component, inject } from '@angular/core';
import { Router, RouterLink, RouterOutlet } from '@angular/router';
import { CommonModule } from '@angular/common';
import { SessionService } from './core/session.service';
import { AuthService } from './api/api/auth.service';
import { ApiError } from './core/api-error';
import { ARABIC_MESSAGES } from './shared/messages';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink],
  template: `
    @if (session.user(); as u) {
      <header class="pms-toolbar">
        <strong>{{ title }}</strong>
        <nav>
          @if (session.isAdmin()) {
            <a routerLink="/users">{{ msgs.usersList }}</a>
            <a routerLink="/records">{{ msgs.recordsList }}</a>
          }
        </nav>
        <div class="pms-toolbar-spacer"></div>
        <div class="pms-toolbar-user">
          <span>{{ u.displayName }}</span>
          <span class="pms-muted">·</span>
          <span class="pms-muted">{{ roleLabel(u.role) }}</span>
        </div>
        <button type="button" class="pms-button pms-button-secondary" (click)="onSignOut()">
          {{ msgs.signOut }}
        </button>
      </header>
    }
    <main>
      <router-outlet></router-outlet>
    </main>
  `,
  styles: [`
    :host { display: block; }
  `],
})
export class App {
  protected readonly title = ARABIC_MESSAGES.appTitle;
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly session = inject(SessionService);
  private readonly router = inject(Router);
  private readonly auth = inject(AuthService);

  // The session is resolved during bootstrap by provideAppInitializer in
  // app.config.ts, before the router runs. Refreshing here as well would race
  // the guards and is what previously showed the sign-in page to a signed-in
  // user.

  protected roleLabel(role: string): string {
    return role === 'admin' ? this.msgs.roleAdmin : this.msgs.roleManager;
  }

  protected onSignOut(): void {
    // Sign out server-side, then clear local state. Either failure must
    // still drop the user back at sign-in.
    this.auth.signOut('response').subscribe({
      next: () => {
        this.session.signOut();
        void this.router.navigate(['/sign-in']);
      },
      error: (_err: ApiError) => {
        this.session.signOut();
        void this.router.navigate(['/sign-in']);
      },
    });
  }
}