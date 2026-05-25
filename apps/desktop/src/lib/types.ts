// Mirrors engine/internal/project Project struct (JSON tag names).
export type LengthTarget = "flash" | "short" | "novella" | "novel" | "series";
export type DefaultPOV = "first" | "third_limited" | "omniscient";

export interface Project {
  id: string;
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
  style_notes: string;
  word_count: number;
  last_opened_node_id?: string;
  created_at: number;
  updated_at: number;
  archived_at?: number;
}

export interface NewProjectInput {
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
}

export interface ListProjectsParams {
  include_archived?: boolean;
  limit?: number;
}
