const { getPaginationParams } = require("../utils/pagination");
const { GameType } = require("../constants/game");

const uuidRegex =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function isUuid(value) {
  return typeof value === "string" && uuidRegex.test(value);
}

function isValidGameType(gameType) {
  return Object.values(GameType).includes(gameType);
}

function getGameUsecase(req) {
  return req.app.locals.gameUsecase;
}

async function getGames(req, res) {
  const gameType = req.query.type;
  if (gameType && !isValidGameType(gameType)) {
    return res.status(400).json({
      error: "Invalid query parameters",
      details: `Invalid game type '${gameType}'. Must be one of: ${Object.values(GameType).join(", ")}`,
    });
  }

  try {
    const games = await getGameUsecase(req).getAvailableGames(
      gameType || null
    );
    if (gameType && games.length === 0) {
      const game = await getGameUsecase(req).createOrGetGame(gameType);
      return res.status(200).json({ games: [game] });
    }

    return res.status(200).json({ games });
  } catch (err) {
    return res.status(500).json({
      error: "Failed to get available games",
      details: err.message,
    });
  }
}

async function joinGame(req, res) {
  const gameId = req.params.gameId;
  if (!isUuid(gameId)) {
    return res.status(400).json({ error: "Invalid game ID" });
  }

  if (!req.body || !isUuid(req.body.user_id) || typeof req.body.card_id !== "number") {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const player = await getGameUsecase(req).joinGame(gameId, req.body);
    return res.status(200).json({ player });
  } catch (err) {
    if (
      err.message === "game is not accepting new players" ||
      err.message === "user is already in this game" ||
      err.message === "card is already taken" ||
      err.message === "insufficient balance"
    ) {
      return res.status(400).json({ error: err.message });
    }

    return res.status(500).json({ error: err.message });
  }
}

async function leaveGame(req, res) {
  const gameId = req.params.gameId;
  if (!isUuid(gameId)) {
    return res.status(400).json({ error: "Invalid game ID" });
  }

  if (!req.body || !isUuid(req.body.user_id)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    await getGameUsecase(req).leaveGame(gameId, req.body);
    return res.status(200).json({ message: "Successfully left the game" });
  } catch (err) {
    if (
      err.message === "user is not in this game" ||
      err.message === "game is no longer active"
    ) {
      return res.status(400).json({ error: err.message });
    }

    return res.status(500).json({ error: err.message });
  }
}

async function claimBingo(req, res) {
  const gameId = req.params.gameId;
  if (!isUuid(gameId)) {
    return res.status(400).json({ error: "Invalid game ID" });
  }

  if (!req.body || !isUuid(req.body.user_id) || !Array.isArray(req.body.marked_numbers)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const isWinner = await getGameUsecase(req).claimBingo(gameId, req.body);
    if (isWinner) {
      return res.status(200).json({
        winner: true,
        message: "Congratulations! You won!",
      });
    }

    return res.status(200).json({
      winner: false,
      message: "Invalid bingo claim. You have been eliminated.",
    });
  } catch (err) {
    if (
      err.message === "game is not in drawing phase" ||
      err.message === "user is not in this game" ||
      err.message === "player is already eliminated"
    ) {
      return res.status(400).json({ error: err.message });
    }

    return res.status(500).json({ error: err.message });
  }
}

async function getGameState(req, res) {
  const gameId = req.params.gameId;
  if (!isUuid(gameId)) {
    return res.status(400).json({ error: "Invalid game ID" });
  }

  try {
    const result = await getGameUsecase(req).getGameState(gameId);
    return res.status(200).json({
      game: result.game,
      drawnNumbers: result.drawnNumbers,
      takenCards: result.takenCards,
    });
  } catch (err) {
    return res.status(500).json({ error: err.message });
  }
}

async function getPlayerInGame(req, res) {
  const gameId = req.params.gameId;
  const userId = req.params.userId;
  if (!isUuid(gameId)) {
    return res.status(400).json({ error: "Invalid game ID" });
  }
  if (!isUuid(userId)) {
    return res.status(400).json({ error: "Invalid user ID" });
  }

  try {
    const player = await getGameUsecase(req).getPlayerInGame(gameId, userId);
    if (!player) {
      return res.status(404).json({ player: null });
    }
    return res.status(200).json({ player });
  } catch (err) {
    if (err.message === "player not found") {
      return res.status(404).json({ player: null });
    }
    return res.status(500).json({ error: err.message });
  }
}

async function getCardData(req, res) {
  const cardId = Number.parseInt(req.params.cardId, 10);
  if (!Number.isFinite(cardId)) {
    return res.status(400).json({ error: "Invalid card ID" });
  }

  try {
    const card = await getGameUsecase(req).getCardData(cardId);
    return res.status(200).json({ card });
  } catch (err) {
    const defaultMessage = `card ID must be between 1 and 200`;
    const statusCode = err.message === defaultMessage ? 400 : 500;
    return res.status(statusCode).json({ error: err.message });
  }
}

async function getGameHistory(req, res) {
  const userId = req.params.user_id;
  if (!isUuid(userId)) {
    return res.status(400).json({ error: "Invalid user ID" });
  }

  const { limit, offset } = getPaginationParams(req.query);
  try {
    const games = await getGameUsecase(req).getGameHistory(userId, limit, offset);
    return res.status(200).json({
      games,
      count: games.length,
      limit,
      offset,
    });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch game history" });
  }
}

module.exports = {
  getGames,
  joinGame,
  leaveGame,
  claimBingo,
  getGameState,
  getPlayerInGame,
  getCardData,
  getGameHistory,
};
