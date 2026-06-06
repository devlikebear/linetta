-- Persist the writer-facing outline structure preset per project.
ALTER TABLE projects ADD COLUMN outline_preset TEXT NOT NULL DEFAULT 'novel';
