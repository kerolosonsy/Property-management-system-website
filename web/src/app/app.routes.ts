import { Routes } from '@angular/router';
import {
  landingGuard,
  requireSignedInGuard,
  requireSignedOutGuard,
  requireAdminGuard,
  requirePasswordChangedGuard,
} from './core/auth.guard';

// Guard order matters: signed-in first, then the outstanding-password-change
// check, then role. Each returns a UrlTree rather than false, so a refused
// visitor is always sent somewhere rather than left on a blank route.
//
// Accounts and the records view are sections of Settings, which is
// administrator-only. Nesting them means the role check is declared once on the
// parent and inherited. That is presentation only — the server refuses a
// manager on each of those endpoints independently (Constitution I).
export const routes: Routes = [
  {
    path: 'sign-in',
    canActivate: [requireSignedOutGuard],
    loadComponent: () => import('./features/sign-in/sign-in.component').then(m => m.SignInComponent),
  },
  {
    // The forced change after first sign-in or a reset. Voluntary changes live
    // on the profile screen and reuse the same component.
    path: 'change-password',
    canActivate: [requireSignedInGuard],
    loadComponent: () => import('./features/sign-in/change-password/change-password.component').then(m => m.ChangePasswordComponent),
  },
  {
    path: 'home',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard],
    loadComponent: () => import('./features/home/home.component').then(m => m.HomeComponent),
  },
  {
    path: 'profile',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard],
    loadComponent: () => import('./features/profile/profile.component').then(m => m.ProfileComponent),
  },
  {
    path: 'settings',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard, requireAdminGuard],
    loadComponent: () => import('./features/settings/settings.component').then(m => m.SettingsComponent),
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'users' },
      {
        path: 'users',
        loadComponent: () => import('./features/users/users-list.component').then(m => m.UsersListComponent),
      },
      {
        path: 'users/new',
        loadComponent: () => import('./features/users/create/user-create.component').then(m => m.UserCreateComponent),
      },
      {
        path: 'users/:id',
        loadComponent: () => import('./features/users/edit/user-edit.component').then(m => m.UserEditComponent),
      },
      {
        path: 'records',
        loadComponent: () => import('./features/records/records.component').then(m => m.RecordsComponent),
      },
      {
        path: 'property-types',
        loadComponent: () => import('./features/settings/property-types/property-types.component').then(m => m.PropertyTypesComponent),
      },
      {
        path: 'areas',
        loadComponent: () => import('./features/settings/areas/areas.component').then(m => m.AreasComponent),
      },
      {
        path: 'custom-fields',
        loadComponent: () => import('./features/settings/custom-fields/custom-fields.component').then(m => m.CustomFieldsComponent),
      },
    ],
  },

  // Properties — visible to all signed-in users (manager and admin).
  // The /search and /:id branches below must register before the wildcard
  // sibling route so the static path is matched first (Constitution II /
  // research.md D-006).
  {
    path: 'properties',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard],
    loadComponent: () => import('./features/properties/properties-list.component').then(m => m.PropertiesListComponent),
  },
  {
    path: 'properties/search',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard],
    loadComponent: () => import('./features/properties/advanced-search.component').then(m => m.AdvancedSearchComponent),
  },
  {
    path: 'properties/new',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard],
    loadComponent: () => import('./features/properties/property-form.component').then(m => m.PropertyFormComponent),
  },
  {
    path: 'properties/:propertyId/edit',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard],
    loadComponent: () => import('./features/properties/property-form.component').then(m => m.PropertyFormComponent),
  },
  {
    path: 'properties/:propertyId',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard],
    loadComponent: () => import('./features/properties/property-detail.component').then(m => m.PropertyDetailComponent),
  },

  // Old top-level paths kept as redirects so existing links and bookmarks do
  // not break now that both live under Settings.
  { path: 'users', pathMatch: 'full', redirectTo: 'settings/users' },
  { path: 'users/new', redirectTo: 'settings/users/new' },
  { path: 'users/:id', redirectTo: 'settings/users/:id' },
  { path: 'records', pathMatch: 'full', redirectTo: 'settings/records' },

  // '' and anything unmatched resolve through landingGuard, which sends a
  // signed-out visitor to /sign-in and a signed-in one to their own start page.
  { path: '', pathMatch: 'full', canActivate: [landingGuard], children: [] },
  { path: '**', canActivate: [landingGuard], children: [] },
];
