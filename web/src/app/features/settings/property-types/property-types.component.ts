// web/src/app/features/settings/property-types/property-types.component.ts
import { Component } from '@angular/core';
import { LookupPanelComponent } from '../lookup-panel.component';
import { ARABIC_MESSAGES } from '../../../shared/messages';

@Component({
  selector: 'app-property-types',
  standalone: true,
  imports: [LookupPanelComponent],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.propertyTypes }}</h1>
          <div class="pms-crumb">{{ msgs.settings }} / {{ msgs.propertyTypes }}</div>
        </div>
      </header>
      <app-lookup-panel [entity]="'property-types'" [heading]="msgs.propertyTypes" />
    </section>
  `,
})
export class PropertyTypesComponent {
  protected readonly msgs = ARABIC_MESSAGES;
}
