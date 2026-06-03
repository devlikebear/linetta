-- Project synopsis is distinct from outline: outline is the writer's plan,
-- synopsis is the story-as-currently-understood summary used for AI context.
ALTER TABLE projects ADD COLUMN synopsis TEXT NOT NULL DEFAULT '';
