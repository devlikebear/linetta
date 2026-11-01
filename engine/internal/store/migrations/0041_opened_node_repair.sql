-- 0041_opened_node_repair.sql — a pulled work could arrive with no opened node.
--
-- The opened node stays on its device, so v1.20.0 left it empty on the device that
-- pulled the work, and the app could not open it. Apply now places one. This puts
-- the first scene on works already pulled, without touching updated_at.
WITH RECURSIVE walk(project_id, id, kind, path) AS (
  SELECT project_id, id, kind, printf('%010d', ordinal) FROM nodes WHERE parent_id IS NULL
  UNION ALL
  SELECT n.project_id, n.id, n.kind, w.path || '/' || printf('%010d', n.ordinal)
    FROM nodes n JOIN walk w ON n.parent_id = w.id
)
UPDATE projects
   SET last_opened_node_id = (
         SELECT id FROM walk
          WHERE walk.project_id = projects.id AND kind = 'leaf'
          ORDER BY path, id LIMIT 1)
 WHERE last_opened_node_id IS NULL
    OR NOT EXISTS (
         SELECT 1 FROM nodes
          WHERE nodes.id = projects.last_opened_node_id AND nodes.project_id = projects.id);
