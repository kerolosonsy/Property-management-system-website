// web/src/app/features/settings/settings.component.ts
// Settings is the administrator's area. Accounts (US2), the records view
// (US4), and the three configuration screens of feature 002 (property types,
// areas, custom fields) are its sections; each keeps its own URL so a link or
// a refresh lands where it should.
//
// The section list and the administrator-only nav entry are presentation.
// Every endpoint behind them is refused for a manager by the server regardless
// of what the interface offers (Constitution I).

import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { ARABIC_MESSAGES } from '../../shared/messages';

@Component({
  selector: 'app-settings',
  standalone: true,
  imports: [CommonModule, RouterLink, RouterLinkActive, RouterOutlet],
  template: `
    <header class="pms-view-head">
      <div>
        <h1>{{ msgs.settings }}</h1>
        <div class="pms-crumb">{{ msgs.breadcrumbRoot }} / {{ msgs.settings }}</div>
      </div>
    </header>

    <div class="pms-subnav-layout">
      <nav class="pms-subnav">
        <a routerLink="users" routerLinkActive="is-current">{{ msgs.usersList }}</a>
        <a routerLink="records" routerLinkActive="is-current">{{ msgs.recordsList }}</a>
        <a routerLink="property-types" routerLinkActive="is-current">{{ msgs.propertyTypes }}</a>
        <a routerLink="areas" routerLinkActive="is-current">{{ msgs.areas }}</a>
        <a routerLink="custom-fields" routerLinkActive="is-current">{{ msgs.customFields }}</a>
      </nav>
      <div class="pms-subnav-body">
        <router-outlet></router-outlet>
      </div>
    </div>
  `,
})
export class SettingsComponent {
  protected readonly msgs = ARABIC_MESSAGES;
}
