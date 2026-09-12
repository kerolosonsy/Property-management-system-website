// web/src/app/features/home/home.component.ts
import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { SessionService } from '../../core/session.service';
import { SystemService } from '../../api/api/system.service';
import { DashboardCounts } from '../../api/model/dashboard-counts.model';
import { ARABIC_MESSAGES } from '../../shared/messages';

@Component({
  selector: 'app-home',
  standalone: true,
  imports: [CommonModule, RouterLink],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.home }}</h1>
          <div class="pms-crumb">{{ msgs.homeWelcome }}</div>
        </div>
      </header>

      @if (loading()) {
        <p class="text-muted">{{ msgs.loading }}</p>
      } @else if (errorMessage(); as message) {
        <div class="pms-error-banner">{{ message }}</div>
      } @else if (counts(); as data) {
        <div class="dashboard-groups">
          <section class="card dashboard-group">
            <h2>{{ msgs.dashboardProperties }}</h2>
            <div class="dashboard-values">
              <a routerLink="/properties"
                ><strong>{{ data.propertiesTotal }}</strong
                ><span>{{ msgs.dashboardPropertiesTotal }}</span></a
              >
              <a routerLink="/properties"
                ><strong>{{ data.propertiesActive }}</strong
                ><span>{{ msgs.dashboardPropertiesActive }}</span></a
              >
              <a routerLink="/properties"
                ><strong>{{ data.propertiesArchived }}</strong
                ><span>{{ msgs.dashboardPropertiesArchived }}</span></a
              >
            </div>
          </section>

          <section class="card dashboard-group">
            <h2>{{ msgs.dashboardAttachments }}</h2>
            <div class="dashboard-values">
              <div>
                <strong>{{ data.attachmentsTotal }}</strong
                ><span>{{ msgs.dashboardAttachmentsTotal }}</span>
              </div>
              <div>
                <strong>{{ data.attachmentsPending }}</strong
                ><span>{{ msgs.dashboardAttachmentsPending }}</span>
              </div>
            </div>
          </section>

          <section class="card dashboard-group">
            <h2>{{ msgs.dashboardConfiguration }}</h2>
            <div class="dashboard-values">
              @if (session.isAdmin()) {
                <a routerLink="/settings/custom-fields"
                  ><strong>{{ data.customFieldsTotal }}</strong
                  ><span>{{ msgs.dashboardCustomFields }}</span></a
                >
                <a routerLink="/settings/property-types"
                  ><strong>{{ data.propertyTypesTotal }}</strong
                  ><span>{{ msgs.dashboardPropertyTypes }}</span></a
                >
                <a routerLink="/settings/areas"
                  ><strong>{{ data.areasTotal }}</strong
                  ><span>{{ msgs.dashboardAreas }}</span></a
                >
              } @else {
                <div>
                  <strong>{{ data.customFieldsTotal }}</strong
                  ><span>{{ msgs.dashboardCustomFields }}</span>
                </div>
                <div>
                  <strong>{{ data.propertyTypesTotal }}</strong
                  ><span>{{ msgs.dashboardPropertyTypes }}</span>
                </div>
                <div>
                  <strong>{{ data.areasTotal }}</strong
                  ><span>{{ msgs.dashboardAreas }}</span>
                </div>
              }
            </div>
          </section>
        </div>
      }
    </section>
  `,
  styles: [
    `
      .dashboard-groups {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
        gap: var(--space-4);
      }
      .dashboard-group h2 {
        margin-block-start: 0;
        font-size: 1rem;
      }
      .dashboard-values {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(5rem, 1fr));
        gap: var(--space-3);
      }
      .dashboard-values a,
      .dashboard-values div {
        display: flex;
        flex-direction: column;
        gap: var(--space-1);
        color: inherit;
        text-align: start;
        text-decoration: none;
      }
      .dashboard-values a:hover span {
        text-decoration: underline;
      }
      .dashboard-values strong {
        font-size: 1.75rem;
        font-variant-numeric: tabular-nums;
      }
      .dashboard-values span {
        color: var(--color-neutral-600);
        font-size: 0.8rem;
      }
    `,
  ],
})
export class HomeComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly session = inject(SessionService);
  private readonly system = inject(SystemService);

  protected readonly counts = signal<DashboardCounts | null>(null);
  protected readonly loading = signal(true);
  protected readonly errorMessage = signal<string | null>(null);

  ngOnInit(): void {
    this.system.getDashboard('body').subscribe({
      next: (counts) => {
        this.counts.set(counts);
        this.loading.set(false);
      },
      error: () => {
        this.errorMessage.set(this.msgs.dashboardLoadFailed);
        this.loading.set(false);
      },
    });
  }
}
