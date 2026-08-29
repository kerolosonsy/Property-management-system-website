export * from './auth.service';
import { AuthService } from './auth.service';
export * from './records.service';
import { RecordsService } from './records.service';
export * from './system.service';
import { SystemService } from './system.service';
export * from './users.service';
import { UsersService } from './users.service';
export const APIS = [AuthService, RecordsService, SystemService, UsersService];
