-- Simplify game tiers to two offerings: REGULAR (10 birr) and VIP (50 birr).
-- The old G1-G7 tiers (5/7/10/20/50/100/200) are removed. Historical game
-- rows are relabeled to the closest new tier so the game_type CHECK constraint
-- can be applied without dropping history. Stored bet_amount values are left
-- untouched, so past games keep their original financials.

BEGIN;

-- Relabel any existing rows to the new tier codes.
-- Lower stakes (5/7/10/20) become REGULAR; higher stakes (50/100/200) become VIP.
UPDATE games SET game_type = 'VIP'     WHERE game_type IN ('G5', 'G6', 'G7');
UPDATE games SET game_type = 'REGULAR' WHERE game_type IN ('G1', 'G2', 'G3', 'G4');

-- Swap the CHECK constraint so only the two supported tiers are accepted.
-- The constraint name comes from migration 004 (games_game_type_check).
ALTER TABLE games DROP CONSTRAINT IF EXISTS games_game_type_check;
ALTER TABLE games ADD CONSTRAINT games_game_type_check
    CHECK (game_type IN ('REGULAR', 'VIP'));

COMMIT;
