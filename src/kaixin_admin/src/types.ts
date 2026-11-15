export type Role = "Owner" | "Admin" | "Member";
export type UserStatus = "active" | "disabled";
export type VersionStatus = "draft" | "published";

export interface AdminUser {
  id: number;
  account: string;
  displayName: string;
  phoneText: string;
  role: Role;
  status: UserStatus;
  createdAt: string;
  lastLoginAt: string | null;
}

export interface VersionInfo {
  version: string;
  fileName: string;
  fileSize: number;
  scanStatus: string;
  scanDetail: string;
  publishedAt: string;
  releaseNotes: string;
  isCurrent: boolean;
  status: VersionStatus;
}

export interface Skill {
  id: number;
  name: string;
  purpose: string;
  summary: string;
  aiSoftware: string;
  required: boolean;
  wingetId: string;
  notes: string;
  current: VersionInfo | null;
  previous: VersionInfo | null;
}

export interface SkillInput {
  name: string;
  wingetId: string;
  purpose: string;
  summary: string;
  aiSoftware: string;
  notes: string;
  required: boolean;
}

export interface VersionNote {
  version: string;
  title: string;
  detail: string;
  publishedAt: string;
}

export interface Profile {
  id: number;
  account: string;
  displayName: string;
  role: Role;
}
