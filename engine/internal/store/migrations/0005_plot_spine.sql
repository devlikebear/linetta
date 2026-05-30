-- Plan 24 플롯 스파인: beat에 서술 본문, project에 작가 편집 개요를 추가한다.
-- 둘 다 NOT NULL DEFAULT '' 이므로 기존 행에 안전하게 적용된다.
ALTER TABLE beats ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN outline TEXT NOT NULL DEFAULT '';
