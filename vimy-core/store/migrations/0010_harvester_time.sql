-- +goose Up
-- Where harvester time went, in harvester-samples: one harvester in one state.
--
-- Income sat near 1.7 credits a tick across every game measured, through a
-- flee radius cut by two thirds, harvester counts from 9 to 11 and an
-- economy-first doctrine. Three economy changes were made on inference about
-- why, because nothing recorded could tell working from idle from queueing.
ALTER TABLE games ADD COLUMN harvester_idle INTEGER;
ALTER TABLE games ADD COLUMN harvester_mining INTEGER;
ALTER TABLE games ADD COLUMN harvester_travelling INTEGER;
ALTER TABLE games ADD COLUMN harvester_at_refinery INTEGER;
-- Mean distance from the nearest refinery while travelling. A rising number is
-- ore running out near the base.
ALTER TABLE games ADD COLUMN harvester_haul_distance REAL;

-- +goose Down
ALTER TABLE games DROP COLUMN harvester_idle;
ALTER TABLE games DROP COLUMN harvester_mining;
ALTER TABLE games DROP COLUMN harvester_travelling;
ALTER TABLE games DROP COLUMN harvester_at_refinery;
ALTER TABLE games DROP COLUMN harvester_haul_distance;
