-- +goose Up
-- How squads held together on the way in, by distance band from the target.
--
-- Game 141 marched the largest army Vimy has fielded at the enemy for 226600
-- ticks and reached `strike` zero times. Everything that could stop a strike
-- AT the base has been measured and eliminated — target scoring, strike range
-- and blind targeting all returned zero across five games — so what remains is
-- that the squad does not arrive together. Spread has sat near 20 cells
-- against a required 8 through four separate fixes.
--
-- JSON rather than twelve integer columns: three bands times four counters,
-- and the shape will change once it says something.
ALTER TABLE games ADD COLUMN transit_spread_json TEXT;

-- +goose Down
ALTER TABLE games DROP COLUMN transit_spread_json;
