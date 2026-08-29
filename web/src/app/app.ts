import { Component, computed, inject, signal } from '@angular/core';
import { Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { CommonModule } from '@angular/common';
import { SessionService } from './core/session.service';
import { AuthService } from './api/api/auth.service';
import { ApiError } from './core/api-error';
import { ARABIC_MESSAGES } from './shared/messages';

const NAV_COLLAPSED_KEY = 'pms.nav.collapsed';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink, RouterLinkActive],
  // The shell follows the design's layout: a fixed-width aside carrying the
  // brand and the navigation, with the routed view filling the rest. Under
  // dir="rtl" the aside sits on the right; `border-inline-end` keeps its rule
  // on the correct edge without a direction-specific override.
  //
  // Collapsing narrows the aside to an icon rail rather than removing it, so
  // every destination stays one click away and the toggle is never orphaned.
  // The aside is hidden entirely while signed out and while a forced password
  // change is outstanding, so those screens render as the design's centred
  // blueprint card on an otherwise empty page.
  template: `
    @if (showShell()) {
      <div class="pms-shell" [class.is-collapsed]="collapsed()">
        <aside class="pms-aside">
          <div class="pms-brand">
            <span class="pms-auth-mark" style="inline-size:32px;block-size:32px;">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M3 21h18"></path><path d="M6 21V9l6-5 6 5v12"></path>
                <path d="M10 21v-6h4v6"></path>
              </svg>
            </span>
            <div class="pms-collapsible">
              <div class="pms-brand-name">{{ msgs.appTitle }}</div>
              <div class="pms-brand-sub">{{ msgs.breadcrumbRoot }}</div>
            </div>
          </div>

          <button type="button"
                  class="pms-nav-toggle"
                  (click)="toggleNav()"
                  [attr.aria-expanded]="!collapsed()"
                  [attr.aria-label]="collapsed() ? msgs.expandNav : msgs.collapseNav"
                  [title]="collapsed() ? msgs.expandNav : msgs.collapseNav">
            <!-- The chevron points the way the panel will move. Under RTL the
                 aside is on the right, so collapsing moves it toward the
                 inline-start edge — the icon is mirrored by the stylesheet
                 rather than by swapping paths here. -->
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M15 18l-6-6 6-6"></path>
            </svg>
          </button>

          <nav class="pms-nav">
            <a routerLink="/home" routerLinkActive="is-current" [title]="msgs.home">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M3 9.5 12 3l9 6.5"></path><path d="M5 9.5V21h14V9.5"></path>
                <path d="M9 21v-6h6v6"></path>
              </svg>
              <span class="pms-collapsible">{{ msgs.home }}</span>
            </a>

            <!-- Accounts and the records view both live under Settings, and
                 Settings is administrator-only. Hiding it is presentation only;
                 the server refuses a manager on every one of those endpoints
                 regardless of what the navigation shows (Constitution I). -->
            @if (session.isAdmin()) {
              <a routerLink="/settings" routerLinkActive="is-current" [title]="msgs.settings">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <circle cx="12" cy="12" r="3"></circle>
                  <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                </svg>
                <span class="pms-collapsible">{{ msgs.settings }}</span>
              </a>
            }
          </nav>

          <div class="pms-aside-foot">
            <!-- The signed-in person's name is the way into their own profile
                 (FR-011 lives there). Collapsed, the avatar mark remains and
                 still carries the link, so the profile never becomes
                 unreachable. -->
            @if (session.user(); as u) {
              <a class="pms-aside-user"
                 routerLink="/profile"
                 routerLinkActive="is-current"
                 [title]="u.displayName + ' — ' + msgs.profile">
                <span class="pms-avatar" aria-hidden="true">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                       stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="8" r="4"></circle>
                    <path d="M4 21c0-4 3.6-7 8-7s8 3 8 7"></path>
                  </svg>
                </span>
                <span class="pms-collapsible">
                  <span class="pms-aside-user-name">{{ u.displayName }}</span>
                  <span class="text-muted">{{ roleLabel(u.role) }}</span>
                </span>
              </a>
            }
            <button type="button"
                    class="btn btn-secondary btn-block pms-signout"
                    (click)="onSignOut()"
                    [title]="msgs.signOut">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path>
                <path d="M16 17l5-5-5-5"></path><path d="M21 12H9"></path>
              </svg>
              <span class="pms-collapsible">{{ msgs.signOut }}</span>
            </button>
          </div>
        </aside>

        <main class="pms-main">
          <router-outlet></router-outlet>
        </main>
      </div>
    } @else {
      <router-outlet></router-outlet>
    }
  `,
  styles: [`:host { display: block; }`],
})
export class App {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly session = inject(SessionService);
  private readonly router = inject(Router);
  private readonly auth = inject(AuthService);

  protected readonly collapsed = signal(readCollapsed());

  protected readonly showShell = computed(
    // The navigation is offered only to a fully signed-in user. Someone owing a
    // password change gets the bare screen, matching the routing rule that keeps
    // them there until it is done (FR-007).
    () => this.session.user() !== null && !this.session.mustChangePassword(),
  );

  protected toggleNav(): void {
    const next = !this.collapsed();
    this.collapsed.set(next);
    // A remembered preference is a convenience, not state the app depends on:
    // storage can be unavailable or throw outright in a private window, and the
    // shell must render either way.
    try {
      localStorage.setItem(NAV_COLLAPSED_KEY, next ? '1' : '0');
    } catch {
      /* preference simply is not remembered */
    }
  }

  protected roleLabel(role: string): string {
    return role === 'admin' ? this.msgs.roleAdmin : this.msgs.roleManager;
  }

  protected onSignOut(): void {
    this.auth.signOut('response').subscribe({
      next: () => this.afterSignOut(),
      error: (_err: ApiError) => this.afterSignOut(),
    });
  }

  private afterSignOut(): void {
    this.session.signOut();
    void this.router.navigate(['/sign-in']);
  }
}

function readCollapsed(): boolean {
  try {
    return localStorage.getItem(NAV_COLLAPSED_KEY) === '1';
  } catch {
    return false;
  }
}
