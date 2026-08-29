import { Routes } from '@angular/router';
import {
  landingGuard,
  requireSignedInGuard,
  requireSignedOutGuard,
  requireAdminGuard,
  requirePasswordChangedGuard,
} from './core/auth.guard';

// Guard order matters: signed-in first, then the outstanding-password-change
// check, then role. Each returns a UrlTree rather than false so the user is
// always sent somewhere rather than left on a blank route.
export const routes: Routes = [
  {
    path: 'sign-in',
    canActivate: [requireSignedOutGuard],
    loadComponent: () => import('./features/sign-in/sign-in.component').then(m => m.SignInComponent),
  },
  {
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
    path: 'users',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard, requireAdminGuard],
    loadComponent: () => import('./features/users/users-list.component').then(m => m.UsersListComponent),
  },
  {
    path: 'users/new',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard, requireAdminGuard],
    loadComponent: () => import('./features/users/create/user-create.component').then(m => m.UserCreateComponent),
  },
  {
    path: 'users/:id',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard, requireAdminGuard],
    loadComponent: () => import('./features/users/edit/user-edit.component').then(m => m.UserEditComponent),
  },
  {
    path: 'records',
    canActivate: [requireSignedInGuard, requirePasswordChangedGuard, requireAdminGuard],
    loadComponent: () => import('./features/records/records.component').then(m => m.RecordsComponent),
  },
  // '' and anything unmatched resolve through landingGuard, which sends a
  // signed-out visitor to /sign-in and a signed-in one to their own start page.
  { path: '', pathMatch: 'full', canActivate: [landingGuard], children: [] },
  { path: '**', canActivate: [landingGuard], children: [] },
];
