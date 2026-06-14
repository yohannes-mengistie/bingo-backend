const { randomUUID } = require("crypto");

function mapGameRow(row) {
  return {
    id: row.id,
    game_type: row.game_type,
    state: row.state,
    bet_amount: Number(row.bet_amount),
    min_players: Number(row.min_players),
    player_count: Number(row.player_count),
    prize_pool: Number(row.prize_pool),
    house_cut: Number(row.house_cut),
    winner_id: row.winner_id || null,
    countdown_ends: row.countdown_ends || null,
    started_at: row.started_at || null,
    finished_at: row.finished_at || null,
    created_at: row.created_at,
    updated_at: row.updated_at,
  };
}

async function create(db, game) {
  const now = new Date();
  const id = game.id || randomUUID();

  const query = `
    INSERT INTO games (id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, created_at, updated_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
  `;

  const values = [
    id,
    game.game_type,
    game.state,
    game.bet_amount,
    game.min_players,
    game.player_count,
    game.prize_pool,
    game.house_cut,
    now,
    now,
  ];

  await db.query(query, values);
  return {
    ...game,
    id,
    created_at: now,
    updated_at: now,
  };
}

async function findById(db, gameId) {
  const query = `
    SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut,
           winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
    FROM games
    WHERE id = $1
  `;

  const result = await db.query(query, [gameId]);
  if (result.rowCount === 0) {
    return null;
  }

  return mapGameRow(result.rows[0]);
}

async function findAvailable(db, gameType, limit) {
  let query = `
    SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut,
           winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
    FROM games
    WHERE state IN ('WAITING', 'COUNTDOWN')
  `;

  const values = [];
  let index = 1;
  if (gameType) {
    query += ` AND game_type = $${index}`;
    values.push(gameType);
    index += 1;
  }

  query += ` ORDER BY created_at ASC LIMIT $${index}`;
  values.push(limit);

  const result = await db.query(query, values);
  return result.rows.map(mapGameRow);
}

async function update(db, game) {
  const query = `
    UPDATE games
    SET state = $2, player_count = $3, prize_pool = $4, house_cut = $5,
        winner_id = $6, countdown_ends = $7, started_at = $8, finished_at = $9, updated_at = $10
    WHERE id = $1
  `;

  const updatedAt = new Date();
  const values = [
    game.id,
    game.state,
    game.player_count,
    game.prize_pool,
    game.house_cut,
    game.winner_id || null,
    game.countdown_ends || null,
    game.started_at || null,
    game.finished_at || null,
    updatedAt,
  ];

  const result = await db.query(query, values);
  if (result.rowCount === 0) {
    const error = new Error("game not found");
    error.code = "NOT_FOUND";
    throw error;
  }

  return {
    ...game,
    updated_at: updatedAt,
  };
}

async function addPlayer(db, player) {
  const joinedAt = new Date();
  const updateQuery = `
    UPDATE game_players
    SET card_id = $1, is_eliminated = $2, joined_at = $3, left_at = NULL
    WHERE game_id = $4 AND user_id = $5 AND left_at IS NOT NULL
  `;

  const updateResult = await db.query(updateQuery, [
    player.card_id,
    player.is_eliminated,
    joinedAt,
    player.game_id,
    player.user_id,
  ]);

  if (updateResult.rowCount === 0) {
    const insertQuery = `
      INSERT INTO game_players (id, game_id, user_id, card_id, is_eliminated, joined_at)
      VALUES ($1, $2, $3, $4, $5, $6)
    `;

    await db.query(insertQuery, [
      player.id || randomUUID(),
      player.game_id,
      player.user_id,
      player.card_id,
      player.is_eliminated,
      joinedAt,
    ]);
  }

  return {
    ...player,
    joined_at: joinedAt,
  };
}

async function removePlayer(db, gameId, userId) {
  const query = `
    UPDATE game_players
    SET left_at = $3
    WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL
  `;

  await db.query(query, [gameId, userId, new Date()]);
}

async function findPlayer(db, gameId, userId) {
  const query = `
    SELECT id, game_id, user_id, card_id, is_eliminated, joined_at, left_at
    FROM game_players
    WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL
  `;

  const result = await db.query(query, [gameId, userId]);
  if (result.rowCount === 0) {
    return null;
  }

  const row = result.rows[0];
  return {
    id: row.id,
    game_id: row.game_id,
    user_id: row.user_id,
    card_id: Number(row.card_id),
    is_eliminated: row.is_eliminated,
    joined_at: row.joined_at,
    left_at: row.left_at,
  };
}

async function getPlayers(db, gameId) {
  const query = `
    SELECT id, game_id, user_id, card_id, is_eliminated, joined_at, left_at
    FROM game_players
    WHERE game_id = $1 AND left_at IS NULL
  `;

  const result = await db.query(query, [gameId]);
  return result.rows.map((row) => ({
    id: row.id,
    game_id: row.game_id,
    user_id: row.user_id,
    card_id: Number(row.card_id),
    is_eliminated: row.is_eliminated,
    joined_at: row.joined_at,
    left_at: row.left_at,
  }));
}

async function eliminatePlayer(db, gameId, userId) {
  const query = `
    UPDATE game_players
    SET is_eliminated = TRUE
    WHERE game_id = $1 AND user_id = $2
  `;

  await db.query(query, [gameId, userId]);
}

async function getTakenCards(db, gameId) {
  const query = `
    SELECT card_id
    FROM game_players
    WHERE game_id = $1 AND left_at IS NULL
  `;

  const result = await db.query(query, [gameId]);
  return result.rows.map((row) => Number(row.card_id));
}

async function saveDrawnNumber(db, gameId, letter, number) {
  const query = `
    INSERT INTO drawn_numbers (game_id, letter, number, drawn_at)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (game_id, letter, number) DO NOTHING
  `;

  await db.query(query, [gameId, letter, number, new Date()]);
}

async function findGamesByUserId(db, userId, limit, offset) {
  const query = `
    SELECT
      g.id, g.game_type, g.state, g.bet_amount, g.min_players, g.player_count,
      g.prize_pool, g.house_cut, g.winner_id, g.countdown_ends, g.started_at,
      g.finished_at, g.created_at, g.updated_at,
      gp.card_id, gp.is_eliminated, gp.joined_at, gp.left_at
    FROM game_players gp
    INNER JOIN games g ON gp.game_id = g.id
    WHERE gp.user_id = $1
    ORDER BY gp.joined_at DESC
    LIMIT $2 OFFSET $3
  `;

  const result = await db.query(query, [userId, limit, offset]);
  return result.rows.map((row) => {
    const game = mapGameRow(row);
    const entry = {
      game,
      card_id: Number(row.card_id),
      is_eliminated: row.is_eliminated,
      joined_at: row.joined_at,
      left_at: row.left_at,
      is_winner: false,
    };

    if (row.winner_id) {
      entry.is_winner = row.winner_id === userId;
    }

    return entry;
  });
}

async function countGamesByType(db) {
  const query = `
    SELECT game_type, COUNT(*) AS count
    FROM games
    WHERE state IN ('FINISHED', 'CLOSED')
    GROUP BY game_type
  `;

  const result = await db.query(query);
  const counts = {};
  for (const row of result.rows) {
    counts[row.game_type] = Number(row.count);
  }

  return counts;
}

async function getTotalHouseCut(db) {
  const query = `
    SELECT COALESCE(SUM(prize_pool * house_cut / (1 - house_cut)), 0) AS total_house_cut
    FROM games
    WHERE state IN ('FINISHED', 'CLOSED')
  `;

  const result = await db.query(query);
  return Number(result.rows[0].total_house_cut || 0);
}

module.exports = {
  create,
  findById,
  findAvailable,
  update,
  addPlayer,
  removePlayer,
  findPlayer,
  getPlayers,
  eliminatePlayer,
  getTakenCards,
  saveDrawnNumber,
  findGamesByUserId,
  countGamesByType,
  getTotalHouseCut,
};
