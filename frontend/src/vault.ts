import { call } from "./api";

export interface Folder {
  id: string;
  name: string;
}

export interface DocumentSummary {
  id: string;
  revision: string;
  title: string;
  folder_id: string | null;
  updated_at: string;
}

export interface VaultDocument extends DocumentSummary {
  markdown: string;
  merged: boolean;
}

export interface DocumentInput {
  title: string;
  folder_id: string | null;
  markdown: string;
}

export const vault = {
  list: (folder: string) => call<{ folders: Folder[]; documents: DocumentSummary[] }>(
    "GET", folder ? `/vault?folder=${encodeURIComponent(folder)}` : "/vault",
  ),
  createFolder: (name: string) => call<Folder>("POST", "/vault/folders", { name }),
  create: (body: DocumentInput) => call<VaultDocument>("POST", "/vault/documents", body),
  read: (id: string) => call<VaultDocument>("GET", `/vault/documents/${id}`),
  save: (id: string, revision: string, body: DocumentInput) =>
    call<VaultDocument>("PUT", `/vault/documents/${id}`, { ...body, revision }),
};
