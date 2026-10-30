-- The earlier gallery pictures a requested drawing is sent with, after the
-- characters' reference images, so a face drawn once is drawn the same way
-- again. Chosen by the agent, or suggested when the request is left
-- (illustration.Repo.SuggestPrior); the writer can change them on the card.
ALTER TABLE illustration_requests ADD COLUMN prior_illustration_ids_json TEXT NOT NULL DEFAULT '[]';
