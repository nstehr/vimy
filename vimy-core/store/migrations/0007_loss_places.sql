-- +goose Up
-- +goose StatementBegin
-- Where units were standing when we lost them.
--
-- Armour peak held between 1 and 6 across games whose vehicle production ranged
-- from 6 orders to 1167. The counters could say how many died and never where,
-- so every account of it was a story. Forward and at home imply opposite fixes:
-- one says armour is fed into defences in mixed waves, the other says base
-- defence is eating the army meant for attacking.
--
-- Only the forward half is stored. The totals are already columns, so home is
-- the subtraction, and a second pair would let the two disagree.
--
-- Nullable: games archived before this ran cannot answer.
ALTER TABLE games ADD COLUMN infantry_lost_forward INTEGER;
ALTER TABLE games ADD COLUMN vehicles_lost_forward INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE games DROP COLUMN infantry_lost_forward;
ALTER TABLE games DROP COLUMN vehicles_lost_forward;
-- +goose StatementEnd
