// web/src/app/features/settings/areas/areas.component.ts
import { Component } from '@angular/core';
import { LookupPanelComponent } from '../lookup-panel.component';
import { ARABIC_MESSAGES } from '../../../shared/messages';

@Component({
  selector: 'app-areas',
  standalone: true,
  imports: [LookupPanelComponent],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.areas }}</h1>
          <div class="pms-crumb">{{ msgs.settings }} / {{ msgs.areas }}</div>
        </div>
      </header>
      <app-lookup-panel [entity]="'areas'" [heading]="msgs.areas" />
    </section>
  `,
})
export class AreasComponent {
  protected readonly msgs = ARABIC_MESSAGES;
}
